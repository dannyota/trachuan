# PDF text layer

This note owns when `extract` reads a PDF's own text layer instead of sending
the file to OCR. [Text extraction](text-extraction.md) owns the order of text
sources and the parsing that follows; [OCR store](ocr-store.md) and
[OCR cross-check](ocr-check.md) own the OCR path.

Status: decided, not implemented. The owner decided on 2026-11-11 to prefer the
PDF's own text and use OCR only when that text fails, as banhmi did. It is
scheduled after the OCR path, because every document it would serve gets text
through OCR in the meantime. The `pdf_text` value joins the corpus schema as
version 5, together with the next schema change, with the matching reader,
OpenAPI and client changes; version 4 keeps its four values.

## What qualifies

`pdf_text` is a text source between `html_transcription` and `ocr`. A PDF
qualifies only when all three hold:

1. Every page is classified, and every page is `text` or `empty`. One `scan` or
   `failed_text` page sends the whole document to `ocr`; nothing is dropped and
   no document mixes sources. A 12-page probe may reject early but never
   accepts.
2. The file has an approval (below).
3. The extracted text passes the 300-character minimum and the parsing limits.

Page classes, gate version `pdf-gate/1`, computed on the page's text after NFC:

- `scan`: an image is drawn over at least 80% of the page area, whatever text
  layer the page carries. Area is the image's rendered size (pixel size divided
  by its resolution on the page, as `pdfimages -list` reports) against the page
  size from `pdfinfo`, never the pixel size alone. An embedded layer on a scan
  has unknown provenance and is never read.
- `empty`: no letters and no image above 5% of the page.
- `text`: at least 50 letters; replacement character (U+FFFD) and C0 controls
  other than tab, line feed and carriage return at most 1% of the characters;
  Private Use Area (U+E000 to U+F8FF) at most 0.5% of the characters; Cyrillic
  letters at most 1% of the letters; the mojibake signatures `Ã`, `Â`, `á»`,
  `áº`, `Ä‘`, `Æ°` at most 1% of the letters; and at least 5% of the letters
  from the Vietnamese set: `ă â đ ê ô ơ ư`, their capitals, and every letter
  carrying one of the five tone marks.
- `failed_text`: anything else (a broken font, a legacy encoding).

A file that poppler cannot read is the gap `file_unreadable` and continues to
`ocr`.

## Approval

Character statistics show that a layer is eligible; they cannot show that it
decodes correctly, covers the page or reads in order (two paragraphs swapped
between articles keep every statistic and can keep the article sequence). So an
eligible PDF is used only with an approval: an entry in a reviewed list, keyed
by the PDF's SHA-256 and the SHA-256 of the extracted text, written after a
person compared the extracted text with the rendered pages on every page:
coverage, decoding and reading order. The approval must reject text that
originates from a scan, including tiled or partial scans that pass the per-image
threshold above. Changed extracted text needs a new approval. Without one the
document goes to `ocr`. The candidate set is small enough for this to be the
whole control.

`pdf_text` documents are `text_complete` false by default: plain extracted text
carries no table, formula or image signals, so a flattened table cannot be
detected. The approval may state that the file has none; that permits
`text_complete` true only when the shared extraction and build rules report no
incompleteness gap. An approval never overrides those rules.

## Measured

Run `vn/20261110_6`, 323 distinct PDF files of the 314 PDF-only documents,
poppler 26.11.0, every page classified (11,439 pages in the 321 readable files):

| Pages  | Count  |     | Files                              | Count |
| ------ | ------ | --- | ---------------------------------- | ----- |
| scan   | 10,523 |     | all scan                           | 295   |
| text   | 700    |     | all text                           | 14    |
| failed | 206    |     | scan and failed                    | 1     |
| empty  | 10     |     | mixed, with at least one text page | 11    |
|        |        |     | unreadable                         | 2     |

The 14 all-text files are the candidates; one document has several of them and
is ambiguous under the selection rule, so the source serves at most 13 documents
here. Of the 11 mixed files, 7 are signed scans with one text page, 3 are
consolidated texts with 25 to 37 text pages and one scanned page, and 1 has 82
text pages and 4 failed ones; under rule 1 all go to OCR. Vietnamese letter
share per page with at least 50 letters: readable layers median 27.9%, garbage
layers median 0% and at most 2.3%, lowest page classed `text` 12.1%. A per-page
source (text pages from the layer, the rest from OCR) would save 184 OCR pages
in 4 documents; it is in the backlog.

## Extractor and its boundary

`pdfinfo`, `pdfimages` and `pdftotext` from poppler, run as subprocesses with
arguments passed directly, never through a shell. Text mode is poppler's default
reading order (no `-layout`, no `-raw`), UTF-8; pages split on form feeds,
paragraphs on blank lines; locators are `page:<n>`. Compared: MuPDF through
go-fitz (banhmi's choice) links a C library into the worker and its AGPL licence
would bind the worker binary; pdfcpu has no reliable text extraction; poppler
stays a separate program, and cross-check version 2 renders pages with its
`pdftoppm`.

The PDFs are untrusted and this is the one place the worker hands them to a
parser, so limits are not enough: each subprocess runs in an enforced sandbox
with no network, no credentials in its environment, read access to the verified
PDF and the tool's runtime files only, the numeric limits in
[budgets](budgets.md#pipeline) (memory, scratch space, stdout, stderr,
concurrent subprocesses, time per call and per document), and cancellation that
kills and reaps the process tree. The worker refuses a configuration that cannot
enforce them. A missing tool, an unsupported version or an unavailable sandbox
is a worker configuration error that stops the stage; it is never a reason to
select OCR for a document. A timeout or nonzero exit on one PDF is
`file_unreadable` for that PDF.

Where it runs: `extract`, on the write path only. Nothing on the serving host
reads a PDF or needs poppler. A worker container installs a pinned poppler
package; the deploy lane keeps poppler independently executable, ships its
licence notices and provides the corresponding source or a written offer as the
GPL requires for distributed binaries.

## Freezing and recording

Planning freezes the selection as it does for OCR: an immutable record per
document holding the candidate PDF hashes, the selected PDF or the failure
reason, the per-page classes with their Vietnamese letter share and image
coverage, the extracted text hash, the approval used, and the OCR selection it
falls back to. Planning stores the extracted text and the page record immutably
and binds their hashes, the selection outcome and the approval to the frozen
input. Processing verifies those bytes, fails the item when they are absent or
differ, and never selects or extracts again. The extract recipe binds the
poppler version, the arguments, the gate version and the parser version; a
change invalidates every selection that consulted the gate, failures included,
and a runtime version that differs from the recipe refuses to process.

Candidates are deduplicated by SHA-256 and counted only after full extraction,
the 300-character minimum and the parsing limits. Exactly one usable candidate
is selected; several are `text_source_ambiguous`; none hands the document to the
OCR selector. A heading-only tree is replaced by `pdf_text` under the same rule
as by any later source.

`text.json` records `text_source` `pdf_text`, the PDF's name and SHA-256, the
tool version, the per-page record above and the paragraphs with locators.
Validity rules treat `pdf_text` like every source other than the source tree:
review evidence only. When this is implemented, the safety sections of
[text extraction](text-extraction.md#limits-and-safety) and
[OCR cross-check](ocr-check.md#limits) change with it to name the sandboxed
extraction in the worker.
