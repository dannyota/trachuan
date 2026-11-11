# Text extraction for documents without a source tree

This note owns how `extract` builds a provision tree when the source supplies
none: from the source's own HTML transcription, from attached files, and later
from OCR. [Pipeline](pipeline.md) owns the stage; [chunk](chunk.md) owns
splitting; [fetch bundles](fetch-bundles.md) owns the artifacts read here;
[corpus schema](corpus-schema.md) owns the columns.

## Problem

Run `vn/20261110_4` fetched 3,833 documents; 562 have no VBPL tree and so no
provisions or chunks, and the 9 secondary documents are metadata only. They
cannot be found by search. 361 of the 562 are consolidated texts (Văn bản hợp
nhất), which are the current reading of amended law. Measured on that run's
fetch artifacts:

| Evidence already in the bundle                          | Documents |
| ------------------------------------------------------- | --------- |
| `<id>_content.html` with text, two or more `Điều` lines | 73        |
| `<id>_content.html` with text, numbered outline         | 80        |
| `<id>_content.html` with text, plain                    | 35        |
| `<id>_content.html` empty (under 300 characters)        | 356       |
| at least one PDF attachment                             | 387       |
| at least one DOC or DOCX attachment                     | 98        |

Secondary documents hold a signed PDF each. How many PDFs carry a text layer is
not measured yet; the first plan task measures it.

## Sources of text, in order

For a document whose source tree is empty, `extract` takes the first source that
yields text, and records which one:

1. `html_transcription`: the bundle's `<id>_content.html` (never a backup copy
   such as `_content_bk_*`, `_content_origin` or `_body_content`).
2. `file_text`: the text layer of the main attachment, DOCX before PDF. The main
   attachment is the one the bundle marks `main`, else the only DOCX, else the
   only PDF; several candidates with different text are a gap, not a choice.
3. `ocr`: later work; this note reserves the name and nothing else.

A document with a source tree keeps it (`source_tree`); its extract artifact
stays byte-identical, so its chunk and embed artifacts are reused. Reason: the
transcription is the publisher's own text and needs no model; attachments need
format parsers and can disagree; OCR is lossy and costly, so it comes last and
separately.

## From text to a tree

One parser serves every text source. Input is an ordered list of paragraphs
(HTML block elements, DOCX paragraphs, PDF lines joined into paragraphs), each
NFC with whitespace collapsed; table cells become paragraphs in reading order
and are marked `table`.

- Heading lines: `Phần <roman or word>`, `Chương <roman>`, `Mục <number>`,
  `Điều <number>[.]`, then `<n>.` clauses and `<letter>)` points under an
  article. A heading's title is the rest of its line, or the next line when the
  heading line holds only the label (`Chương I` then `QUY ĐỊNH CHUNG`).
- Anchors use the existing form (`chuong-1/muc-2`, `dieu-5/khoan-2/diem-a`), so
  citations, the identity map and validity rules treat these provisions like
  source trees.
- Text before the first heading is one `preamble` provision; text after the last
  article that matches a signature or recipient block is dropped from provisions
  and kept in the artifact as `tail`.
- A document with no `Điều` uses its numbered outline: top-level `I.`/`1.` items
  become provisions of kind `muc` with anchors `muc-<n>`.
- A document with neither is one provision of kind `toan-van`, anchor
  `toan-van`, when it has at least 300 characters; shorter text is `no_text`.

Fail closed on structure, never on text: article numbers that repeat or go
backwards keep the text, mark the document `article_sequence` (the existing gap)
and number the anchors by position with a suffix (`dieu-5-2`), so nothing is
dropped and nothing is cited under a wrong number. Consolidated texts repeat
footnote numbers and quoted amendments; a line inside quotation marks or a
footnote block never opens a heading. Every provision keeps the exact source
paragraphs it came from, with paragraph ordinals, so a reader can check it
against the file.

## What is recorded

- Extract artifact: `text_source`, the file name and SHA-256 the text came from,
  the paragraph list, the tree, `tail`, and counts (paragraphs, tables,
  characters, headings by kind).
- `document.text_source` in the corpus (`source_tree`, `html_transcription`,
  `file_text`, `ocr`) and `content_state` as today (`indexed`, `no_tree`,
  `no_chunks`). Hits return `text_source`, so an agent can tell a publisher tree
  from a parsed transcription. This is corpus schema version 4.
- Gaps: `no_tree` only when no source yields text; `no_text` for text under the
  minimum; `text_source_conflict` when candidates disagree; `table_text` when
  more than a tenth of the characters sit in tables (formulas and forms are
  often images or tables and stay incomplete); `article_sequence` as above.

## Files and safety

Attachments are untrusted. Parsers run in-process in Go with no shell, no
macros, no external entity resolution and no network; DOCX is read as a zip with
the per-file and expansion limits in [budgets](budgets.md) (50 MB per file, 5
minutes per document); legacy `.doc` is not parsed in this work and is a
`file_format_unsupported` gap. PDF text comes from a pure-Go reader chosen by
the first plan task's measurement; a PDF without a text layer is `needs_ocr`,
never guessed. Text from a signed PDF says nothing about the signature;
signature validation stays out of scope.

## Recipes and reruns

The extract recipe changes, so every extract item reruns; source-tree documents
must reproduce their artifact bytes (a golden test pins this) so only documents
that gain text flow on to chunk and embed. The build recipe changes with schema
version 4. Validity rules then see effect articles of documents that had no
tree, so the review queue and automatic corrections can change; measure both
before accepting a run.

## Tests and measurement

Parser tests use recorded bundles from the run: a consolidated circular with
chapters and articles, a directive with a numbered outline, a plain decision, a
document with quoted amendments and footnotes, an empty transcription, a DOCX
and a PDF with and without a text layer. Each plan task reports how many of the
562 documents moved from `no_tree` to `indexed`, by text source, and a manual
spot check of 20 parsed documents against their files before the run is
accepted.
