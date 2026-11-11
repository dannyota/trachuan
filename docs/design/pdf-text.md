# PDF text layer

This note owns when `extract` reads a PDF's own text layer instead of sending
the file to OCR. [Text extraction](text-extraction.md) owns the order of text
sources and the parsing that follows; [OCR store](ocr-store.md) and
[OCR cross-check](ocr-check.md) own the OCR path.

## Decision

The owner decided on 2026-11-11 to prefer the PDF's own text and use OCR only
when that text fails, as banhmi did. `pdf_text` is a text source between
`html_transcription` and `ocr`. It needs no OCR and no model cross-check: the
text is the publisher's, not a machine reading of an image.

## Gate

The rule follows banhmi's shipped gate (read for the rule; no code is reused)
and is stricter on one point. The pipeline probes the first 12 pages of the PDF
(all pages of a shorter file) and accepts the text layer only when every
condition holds:

1. Not a scan with a text layer: fewer than 80% of the probed pages carry a
   page-sized image (1,000 pixels or more on each side). A scan with an embedded
   OCR layer goes to OCR even when that layer reads fluently, because it is a
   machine reading of unknown quality.
2. Enough text: at least 200 letters in the probe after NFC.
3. Clean encoding: replacement and control characters at most 1% of the
   characters, Private Use Area at most 0.5% (legacy Vietnamese fonts), no UTF-8
   mojibake sequences or Cyrillic above 1% of the letters.
4. Vietnamese: at least 15% of the letters carry a Vietnamese diacritic or are
   one of `ă â đ ê ô ơ ư`. banhmi treated low diacritic density as a penalty;
   here it is a hard condition, because every garbage layer measured has almost
   none.

A PDF that fails any condition is not read as text; the document continues to
`ocr`. The gate result and its measured values go into `text.json`.

Measured on the 314 PDF-only documents of run `vn/20261110_6` (323 distinct
files) with the extractor below:

| Earlier label of the text layer   | Files | Pass | Fail, by first failed condition                      |
| --------------------------------- | ----- | ---- | ---------------------------------------------------- |
| Readable Vietnamese               | 35    | 18   | 16 scan with a text layer, 1 no diacritics           |
| Garbage from a non-Vietnamese OCR | 196   | 0    | 196 scan with a text layer (and no diacritics)       |
| No text layer                     | 92    | 0    | 79 scan, 11 too few letters, 2 unreadable by poppler |

The 18 passing files belong to 16 documents; those are the documents this source
recovers. No garbage layer and no scan passes. Passing files have a diacritic
share of 26% to 31%; the garbage layers stay under 1%. Ten of the 16 readable
scans have good diacritics and still go to OCR under condition 1.

## Extractor

`pdftotext` and `pdfimages` from poppler, run as subprocesses. Compared:

- MuPDF through go-fitz (banhmi's choice): fast and accurate, but it links a C
  library into the worker (cgo, a larger attack surface inside our process) and
  its AGPL licence would bind the worker binary.
- pdfcpu (already used to count pages): no reliable text extraction with layout
  and encodings.
- poppler: a separate program, so the parser runs outside our process and its
  GPL licence does not reach our code; the cross-check already needs its
  `pdftoppm` to render pages. One tool for text, images and rendering.

Each subprocess runs with a 60-second limit, a 1 GiB memory limit where the
platform can enforce one, no network need, the PDF as its only input and its
output read under the 50 MB text limit. A timeout, a nonzero exit or oversized
output is a failed gate, never a failed item: the document continues to `ocr`.
The PDF in the bundle is never modified.

Where it runs. `extract` runs on the write path only: the MacBook today. The
gate result is part of the extract artifact, so nothing on the serving host
reads a PDF or needs poppler. If the worker moves into a container, its image
installs a pinned poppler version from the distribution's package, records the
version in the extract recipe, and states the GPL licence in the image's
notices; the deploy lane owns that image and this note is its contract. The
recipe carries the extractor name and version, so a new poppler version reruns
extract for `pdf_text` documents.

## What is recorded

`text_source` is `pdf_text`; `text.json` holds the PDF's name and SHA-256, the
extractor version, the gate values and the paragraphs with `page:<n>` locators,
since the layer has pages. The text goes through the same paragraph and article
parser as every other source, so the same fail-closed checks and gaps apply. A
document whose PDF passes the gate but whose text ends under 300 characters is
`no_text` and continues to `ocr`. Validity rules treat `pdf_text` like every
source other than the source tree: review evidence only.

Selection among several PDFs follows the OCR rule: one distinct PDF by SHA-256,
or exactly one that passes the gate; several passing candidates are
`text_source_ambiguous`.
