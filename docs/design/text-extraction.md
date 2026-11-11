# Text extraction for documents without a source tree

This note owns how `extract` builds provisions when the source supplies no tree.
[Pipeline](pipeline.md) owns the stage; [chunk](chunk.md) owns splitting;
[fetch bundles](fetch-bundles.md) owns the artifacts read here;
[corpus schema](corpus-schema.md) owns the columns. OCR is a separate later
note; this one reserves its names only.

## Evidence

Run `vn/20261110_5` fetched 3,832 documents; 562 have no tree (553 VBPL, 9
secondary) and so cannot be found by search; 361 of the VBPL ones are
consolidated texts. An inventory of their fetch artifacts
(`docs/research/vn-text-inventory-2026-11.md`) gives the first usable text
source per document:

| Text source                                               | Documents |
| --------------------------------------------------------- | --------- |
| HTML transcription `<id>_content.html` with text          | 188       |
| PDF with a Unicode Vietnamese text layer                  | 23        |
| PDF whose text layer is garbage from a non-Vietnamese OCR | 203       |
| Scanned PDF, no text layer                                | 88        |
| No attachment, or only `.doc` or `.rar`                   | 60        |

Of the 188 transcriptions, 73 have two or more `Điều` lines, 80 a numbered
outline, 35 plain text; 46 have more than two tables, 9 have images. Where
`_content.html` and `_content_origin.html` both hold text, 98 are identical and
49 differ; none has text only in the origin. So 188 documents need no OCR and
314 have only a PDF (about 10,500 pages); the counts include the nine secondary
documents and sum to 562.

## Text source per document

Exactly one source supplies a document's text, chosen in this order:

1. `source_tree`: the source's tree, as today.
2. `html_transcription`: `<id>_content.html` of the bundle when its text has at
   least 300 characters. `_content_origin`, `_body_content` and dated backups
   are never read: the inventory shows they add no document, and where the
   origin differs we take the published version.
3. `ocr`: OCR text of the document's PDF under
   [ADR 0014](../adr/0014-ocr-on-the-write-path.md). The PDF is the bundle's
   only distinct PDF by SHA-256 (261 of the measured documents list one file
   under several names), or the only one with OCR text; several distinct
   candidates are `text_source_ambiguous`. OCR text comes from the OCR store,
   keyed by the PDF's SHA-256: first banhmi's Google Vision output reused as
   data (266 documents matched by hash), else a fresh Google Cloud Vision
   request (48 documents, 1,342 pages measured).
4. `none`: the document stays `no_tree` with a gap naming why (`needs_ocr` until
   the OCR store holds its text with a completed cross-check, `no_text_source`,
   `file_format_unsupported` for `.doc` and archives, `text_source_ambiguous`).

A PDF's own text layer is never read: 203 measured layers are garbage from an
earlier non-Vietnamese OCR pass, and the 23 good ones go through OCR like the
scans, so there is one PDF path and no PDF parser in the pipeline. DOCX is not
read: it would recover no document. Reason for the order: the transcription is
the publisher's own text; OCR is a machine reading and says so.

## Paragraphs

Every text source becomes an ordered list of paragraphs, each NFC with
whitespace collapsed and a locator: the HTML block's ordinal, or the PDF page
and line range. Table cells become paragraphs in reading order, marked `table`;
images and formulas leave a marker paragraph `[hình]`, never invented text.

## From paragraphs to provisions

Provision kinds stay those the schema has (`phan`, `chuong`, `muc`, `dieu`,
`khoan`, `diem`, `other`). Generated `node_id` values use the `tx:` namespace
defined under "What is recorded", which no source id uses, so a source relation
endpoint can never resolve to one by accident.

A provision's `text` holds its heading line and every paragraph under it up to
the next heading of the same or a higher level, exactly once, the same shape the
source tree gives; so an article's `text` is the whole article.

Grammar, applied to paragraph starts only:

- `Phần <roman|word>`, `Chương <roman|number>`, `Mục <number>`: structure
  headings; the title is the rest of the line or the next line in capitals.
- `Điều <n>.` or `Điều <n>` followed by a title: an article.

Parsed and OCR text asserts articles only. Clauses and points stay inside their
article's text and get no provision, anchor or citation of their own in this
work: a missed clause heading would attach text to the wrong clause with no
check able to catch it, while an article boundary is verified by the strict
sequence below. The loss of granularity is recorded in the backlog, with the
measurement that can lift it (clause and point parsing compared against
documents that have both a source tree and a transcription).

Instrument scopes. A document often carries a second instrument (a regulation
"ban hành kèm theo"). A new scope starts at a paragraph that is a title line
(`QUY CHẾ`, `QUY ĐỊNH`, `ĐIỀU LỆ`, `PHỤ LỤC` and their variants) followed, with
no article in between, by an article numbered 1. Anchors and citations in the
second and later scopes carry the scope: anchor `kem-theo-<k>/dieu-<n>`, label
"Điều n (văn bản kèm theo k)". A restart at `Điều 1` without such a title is
ambiguity, below.

Fail closed. The article-level reading is accepted for a scope only when its
article numbers run 1, 2, 3, ... with no gap, repeat or step back, and the last
article is followed only by a signature or recipient block or by a new scope. If
any check fails, that scope asserts no article, clause or point: its paragraphs
are grouped into neutral provisions of kind `other`, anchor `doan-<n>` (a new
one at each line that looks like a heading, else every 20 paragraphs), and the
document gets the gap `structure_uncertain` with the failed check. Structure
headings (`chuong`, `muc`) are kept in either case. Reason: a missed heading
attaches text to the wrong article silently; a strict sequence check catches it,
and a neutral anchor is a true citation where "Điều 5" might not be.

A document with no article at all uses neutral provisions too, grouped by its
numbered outline (`I.`, `1.`) when one exists. Text under 300 characters is
`no_text`.

## What is recorded

- The extract artifact of a document without a source tree gains `text.json`:
  `text_source`, the file name and SHA-256 read, for OCR the engine, the store
  entry's hash and its cross-check result, the paragraphs with locators, the
  scopes with their checks, and counts. The artifact of a source-tree document
  is unchanged in every file and in its manifest, so its chunk and embed
  artifacts are reused; a compatibility test pins the fetch, extract, chunk and
  embed hashes of recorded documents.
- Generated node ids are `tx:` plus the first 16 hex characters of the SHA-256
  of the scope index, the anchor and the provision text joined by newlines. The
  id therefore names one text at one anchor in one scope: if text moves to
  another anchor, or an anchor comes to hold other text, the old id no longer
  exists, and the identity map's node check forces re-adjudication even when
  anchor strings such as `doan-3` survive a refetch or a parser change.
- Corpus: `document.text_source` (`source_tree`, `html_transcription`, `ocr`,
  `none`) and `document.text_complete boolean`, false when any gap of
  `structure_uncertain`, `table_text` (more than a tenth of the characters in
  tables), `image_content`, `ocr_flagged` or `needs_ocr` applies; build derives
  `source_tree` for artifacts without `text.json`. Search and read results
  return both on the document entry, with each provision's locator. This is
  corpus schema version 4.

## Inputs and reuse

A source-tree document's extract input stays its fetch artifact hash. For any
other document, planning selects the text source and freezes it into the input
hash: the fetch hash, the source name, and for OCR the store entry's hash and
its cross-check record's hash, or the literal `absent` when the store has no
checked entry. Adding OCR text or a cross-check to the store therefore changes
the input and reruns extract on the next run; a frozen run never changes. The
store keeps one entry per PDF hash and engine id and one check record per entry
and checker id; both are immutable, and the engine and checker ids are part of
the extract recipe.

## Validity rules

Rules A, B and C of [validity corrections](validity-status.md) read effect
articles only from documents whose text source is `source_tree`. For every other
document the effect article is review evidence: it appears in
`validity-review.json` with a proposed manual entry and changes nothing. Reason:
nothing proves a transcription or an OCR reading is complete, and a partial text
can show a commencement sentence and hide its exception; a reviewed manual entry
is the safe way to use it.

## Limits and safety

Attachments are untrusted, and this work parses none of them: HTML is read with
the tokenizer already used, under the paragraph and nesting caps in
[budgets](budgets.md#pipeline); PDFs are only hashed and, for fresh OCR, sent to
the OCR engine. OCR text from the store is plain text and goes through the same
paragraph parser. Any overrun is a gap for that document, never a failed run.
OCR text of a signed PDF says nothing about its signature.

## Acceptance

The plan's first task commits recorded bundles as fixtures: one per stratum and
structure (articles, outline, plain, tables, images, a second instrument, quoted
amendments with footnotes, an OCR text with page headers and broken lines). Each
has an expected provision list written by hand from the file, with exact anchors
and article boundaries, and adverse fixtures that must fall back to neutral
anchors. Before a run is accepted: counts of documents by text source and by
gap; every accepted article-level scope checked against the article count in its
file; the validity review queue and identity-map changes reviewed; and a
real-output check on evaluation cases that cite recovered documents
(142/2026/NĐ-CP and the 35/VBHN-NHNN alternates among them).
