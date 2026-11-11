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
| PDF with a Unicode Vietnamese text layer                  | 24        |
| PDF whose text layer is garbage from a non-Vietnamese OCR | 203       |
| Scanned PDF, no text layer                                | 96        |
| No attachment, or only `.doc` or `.rar`                   | 60        |

Of the 188 transcriptions, 73 have two or more `Điều` lines, 80 a numbered
outline, 35 plain text; 46 have more than two tables, 9 have images. Where
`_content.html` and `_content_origin.html` both hold text, 98 are identical and
49 differ; none has text only in the origin. So this work recovers 212 documents
with no model, and 299 (about 10,500 pages) wait for OCR.

## Text source per document

Exactly one source supplies a document's text, chosen in this order:

1. `source_tree`: the source's tree, as today.
2. `html_transcription`: `<id>_content.html` of the bundle when its text has at
   least 300 characters. `_content_origin`, `_body_content` and dated backups
   are never read: the inventory shows they add no document, and where the
   origin differs we take the published version.
3. `pdf_text`: the text layer of one PDF, only when every condition holds: the
   bundle has exactly one PDF, or exactly one marked `main`; every page has
   text; at least 2% of its letters carry Vietnamese diacritics (the garbage
   layers measured 0); and the document has no usable transcription. Anything
   else is `needs_ocr`.
4. `none`: no usable text; the document stays `no_tree` with a gap naming why
   (`needs_ocr`, `no_text_source`, `file_format_unsupported` for `.doc` and
   archives, `text_source_ambiguous` for several candidate PDFs).

DOCX is not read in this work: the inventory found no document it would recover.
Reason for the order: the transcription is the publisher's own text; a PDF layer
is only trusted when it is whole and plainly Vietnamese; a partial or doubtful
source is worse than none, because the document would look complete.

## Paragraphs

Every text source becomes an ordered list of paragraphs, each NFC with
whitespace collapsed and a locator: the HTML block's ordinal, or the PDF page
and line range. Table cells become paragraphs in reading order, marked `table`;
images and formulas leave a marker paragraph `[hình]`, never invented text.

## From paragraphs to provisions

Provision kinds stay those the schema has (`phan`, `chuong`, `muc`, `dieu`,
`khoan`, `diem`, `other`). Generated `node_id` values are `tx:<ordinal>` (the
first paragraph's ordinal), a namespace no source id uses, so a source relation
endpoint can never resolve to one by accident.

A provision's `text` holds its heading line and every paragraph under it up to
the next heading of the same or a higher level, exactly once, the same shape the
source tree gives; so an article's `text` is the whole article.

Grammar, applied to paragraph starts only, outside quotation marks and footnote
blocks:

- `Phần <roman|word>`, `Chương <roman|number>`, `Mục <number>`: structure
  headings; the title is the rest of the line or the next line in capitals.
- `Điều <n>.` or `Điều <n>` followed by a title: an article.
- Under an article: `<n>.` a clause, `<letter>)` a point under a clause.

Instrument scopes. A document often carries a second instrument (a regulation
"ban hành kèm theo"). A new scope starts at a paragraph that is a title line
(`QUY CHẾ`, `QUY ĐỊNH`, `ĐIỀU LỆ`, `PHỤ LỤC` and their variants) followed, with
no article in between, by an article numbered 1. Anchors and citations in the
second and later scopes carry the scope: anchor `kem-theo-<k>/dieu-<n>`, label
"Điều n (văn bản kèm theo k)". A restart at `Điều 1` without such a title is
ambiguity, below.

Fail closed. The article-level reading is accepted for a scope only when its
article numbers run 1, 2, 3, ... with no gap, repeat or step back, every clause
sequence under an article starts at 1 and runs without a gap, and no quotation
mark or footnote block is left open at a heading. If any check fails, that scope
asserts no article, clause or point: its paragraphs are grouped into neutral
provisions of kind `other`, anchor `doan-<n>` (a new one at each line that looks
like a heading, else every 20 paragraphs), and the document gets the gap
`structure_uncertain` with the failed check. Structure headings (`chuong`,
`muc`) are kept in either case. Reason: a missed heading attaches text to the
wrong article silently; a strict sequence check catches it, and a neutral anchor
is a true citation where "Điều 5" might not be.

A document with no article at all uses neutral provisions too, grouped by its
numbered outline (`I.`, `1.`) when one exists. Text under 300 characters is
`no_text`.

## What is recorded

- The extract artifact of a document without a source tree gains `text.json`:
  `text_source`, the file name and SHA-256 read, the paragraphs with locators,
  the scopes with their checks, and counts. The artifact of a source-tree
  document is unchanged in every file and in its manifest, so its chunk and
  embed artifacts are reused; a compatibility test pins the fetch, extract,
  chunk and embed hashes of recorded documents.
- Corpus: `document.text_source` (`source_tree`, `html_transcription`,
  `pdf_text`, `ocr`, `none`) and `document.text_complete boolean`, false when
  any gap of `structure_uncertain`, `table_text` (more than a tenth of the
  characters in tables), `image_content` or `needs_ocr` applies; build derives
  `source_tree` for artifacts without `text.json`. Search and read results
  return both on the document entry, with each provision's locator. This is
  corpus schema version 4.
- The identity map treats `tx:` node ids and `doan-`, `kem-theo-` anchors as
  parser output: a parser recipe change that alters any anchor of a document
  marks that document's map entries for re-adjudication.

## Validity rules

Rules A, B and C of [validity corrections](validity-status.md) read an effect
article only from a document whose text source is `source_tree`, or
`html_transcription` with `text_complete` true and the article in an accepted
scope. For every other document the effect article is review evidence only: it
appears in `validity-review.json` with a proposed manual entry and changes
nothing. Reason: a partial text can show a commencement sentence and hide its
exception.

## Limits and safety

Attachments are untrusted. PDF text is read in a child process with a memory
limit, a wall-clock limit and no network, killed on overrun; the limits (file
size, pages, memory, seconds, output bytes, one document at a time per worker)
live in [budgets](budgets.md#pipeline). The PDF reader is a pure-Go library
chosen in the plan's first task by running candidates over the 24 documents with
a text layer and the 299 without; a reader that crashes or exceeds limits on any
of them is rejected. HTML is parsed with the tokenizer already used, with a
paragraph and nesting cap. Any overrun is a gap for that document, never a
failed run. Text from a signed PDF says nothing about its signature.

## Acceptance

The plan's first task commits the inventory note and recorded bundles as
fixtures: one per stratum and structure (articles, outline, plain, tables,
images, a second instrument, quoted amendments with footnotes, a good and a
garbage PDF layer). Each has an expected provision list written by hand from the
file, with exact anchors and text boundaries, and adverse fixtures that must
fall back to neutral anchors. Before a run is accepted: counts of documents by
text source and by gap; every accepted article-level scope checked against its
article count in the file; the validity review queue and identity-map changes
reviewed; and a real-output check on new evaluation cases that cite recovered
documents.
