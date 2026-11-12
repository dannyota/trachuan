# Text extraction for documents without a source tree

This note owns how `extract` builds provisions when the source supplies no tree.
[Pipeline](pipeline.md) owns the stage; [chunk](chunk.md) owns splitting;
[fetch bundles](fetch-bundles.md) owns the artifacts read here;
[corpus schema](corpus-schema.md) owns the columns; [OCR store](ocr-store.md)
owns the OCR records.

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

A second measurement on the run 6 corpus found source trees that hold headings
only: in 974 documents (564 primary, 410 relation context) every article is a
leaf whose text is its heading line, while the body sits in the bundle's
transcription; 972 of them have a usable `<id>_content.html`. Search finds these
documents by title only. Trees that hold part of the text (some articles bare,
or far less text than the transcription) also exist and are not handled here;
the backlog carries them with their counts.

## Text source per document

Exactly one source supplies a document's text, chosen in this order:

1. `source_tree`: the source's tree, as today, unless it is a heading-only tree:
   it has at least one article, no article has a child, and every node's text is
   only its own label and heading (an article's heading line, a structure node's
   label and title), so no node anywhere holds body text. A heading-only tree is
   used only when no later source is usable, and a later source that exceeds a
   parsing limit is not usable; the document then keeps its provisions and gets
   the gap `tree_headings_only`.
2. `html_transcription`: `<id>_content.html` of the bundle when its text has at
   least 300 characters. `_content_origin`, `_body_content` and dated backups
   are never read: the inventory shows they add no document, and where the
   origin differs we take the published version.
3. `pdf_text`: the PDF's own text layer when it qualifies under
   [PDF text layer](pdf-text.md) (owner decision 2026-11-11: prefer the file's
   text, OCR only when it fails; decided, not yet implemented, at most 13
   documents measured).
4. `ocr`: OCR text of the document's PDF under
   [ADR 0014](../adr/0014-ocr-on-the-write-path.md). The PDF is the bundle's
   only distinct PDF by SHA-256 (261 of the measured documents list one file
   under several names), or the only one with OCR text; several distinct
   candidates are `text_source_ambiguous`. OCR text comes from the OCR store,
   keyed by the PDF's SHA-256: first banhmi's Google Vision output reused as
   data (266 documents matched by hash), else a fresh Google Cloud Vision
   request (48 documents, 1,342 pages measured).
5. `none`: the document stays `no_tree` with a gap naming why (`needs_ocr` until
   the OCR store holds its text with a completed cross-check, `no_text_source`,
   `file_format_unsupported` for `.doc` and archives, `text_source_ambiguous`,
   `ocr_rejected` or `ocr_label_flagged` as defined in the
   [OCR store](ocr-store.md)).

Wrong record, before any source is chosen: when the transcription's header names
another document, the record holds another document's text and nothing is
indexed. The header is the transcription text before the first preamble marker
(`Căn cứ`, `Chiểu`, `Theo đề nghị`, `Xét đề nghị`, `Điều 1`); its number is the
first `Số:` or upper-case `SỐ` token there. When that number's leading digits
differ from the detail's `docNum`, the number does not occur in the detail title
(a title that names an amended act), and the document is not a consolidated
text, the document gets the gap `text_number_mismatch` (detail: both numbers)
and no provisions from any source, the source tree included. A header without a
number is not checked. Measured on 3,832 VBPL bundles of run vn/20261111_4
(2026-11-12): 2,106 transcription headers carry a number, 2,060 match the
docNum, the others are title-named acts or consolidated texts, and two are real
mismatches: 15/SL holds 14/SL and 50/2025/TT-BKHCN holds 47/2025/TT-BKHCN, in
the tree, the transcription and the DOCX alike. The gap is a coverage gap like
the other `none` gaps: it counts in build coverage and appears in the evaluation
report's gap list. The scanned PDF in the 50/2025/TT-BKHCN bundle is the real
document (QCVN 137:2025/BKHCN, page 1 checked), with a text layer that holds
only the signature stamp: reading it needs the OCR path, and the OCR text's own
header check against the docNum, which is a later change; until then the
document has no text.

A PDF's text layer is read only through that gate: most measured layers are
garbage from an earlier non-Vietnamese OCR pass, and a scan with an embedded
layer goes to OCR however fluent the layer looks. DOCX is not read: it would
recover no document. Reason for the order: the transcription and the file's own
text are the publisher's; OCR is a machine reading and says so.

## Paragraphs

Every text source becomes an ordered list of paragraphs, each NFC with
whitespace collapsed and a locator: the HTML block's ordinal, or the line of the
stored OCR text. Table cells become paragraphs in reading order, marked `table`;
images and formulas leave a marker paragraph `[hình]`, never invented text.

## From paragraphs to provisions

Provision kinds stay those the schema has (`phan`, `chuong`, `muc`, `dieu`,
`khoan`, `diem`, `other`). Generated `node_id` values use the `tx:` namespace
defined under "What is recorded", which no source id uses, so a source relation
endpoint can never resolve to one by accident.

An article's `text` holds its heading line and every paragraph up to the next
article, structure heading or scope close, so it is the whole article. A
structure provision (`phan`, `chuong`, `muc`) holds only its label and title
lines. Every paragraph of a scope therefore sits in exactly one provision.

Quoted text is tracked by two independent states, counted through every
paragraph in order: a depth for directional double marks (an opening `“` or `«`
adds one, a closing `”` or `»` removes one) and an open flag for the straight
mark `"`, which each occurrence flips. Single marks are ignored. A paragraph
that starts while the depth is above zero or the flag is set is inside quoted
text. No heading and no scope boundary is recognized inside quoted text, so an
amended article quoted over several paragraphs, with nested quotations, stays in
the article that quotes it. A depth that goes below zero, or a depth above zero
or a set flag at the end of the document, fails the check of the scope where it
happened or started: the marks are unbalanced and the reading cannot be trusted.
A paragraph records at most one such failure, and scope checks read the failures
with one pass.

Grammar, applied to paragraph starts outside quoted text:

- `Phần <roman|ordinal word>` (`thứ nhất`, `thứ hai`, ...),
  `Chương <roman|number>`, `Mục <number>`: structure headings, the number
  followed by the end of the line or by `.`, `:` or a dash; the title is the
  rest of the line or the next line in capitals. Any other word after `Chương`,
  `Phần` or `Mục` (`Chương này quy định ...`) is running text.
- `Điều <n>.` or `Điều <n><letter>.` (an inserted article such as `Điều 3a.`),
  the period directly after the number, then a title starting with a capital
  letter or the end of the line: an article. A line such as
  `Điều 6 của Luật ...` is running text wrapped by the source or the OCR, never
  a heading.
- The number token is the digits, plus one lowercase letter only when that
  letter is not followed by another letter (`3a.`, or `3a` before a space; in
  `2Phạm` and `2phạm` the token is `2`).
- Every other paragraph that starts with `Điều`, spaces and a number token is
  either running text or a failed heading candidate. It is running text only
  when the token is followed by at least one space and then a lowercase letter,
  or directly or after spaces by `,`, `;` or `)`, as in `Điều 6 của Luật ...`.
  Anything else (`Điều 2: ...`, `Điều 2 : Phạm vi`, `Điều 2 Phạm vi`,
  `Điều 2.phạm vi`, `Điều 2Phạm vi`, `Điều 2phạm vi`) is a failed candidate and
  fails the check of its scope: it may be a heading the grammar missed.

Parsed and OCR text asserts articles only. Clauses and points stay inside their
article's text and get no provision, anchor or citation of their own in this
work: a missed clause heading would attach text to the wrong clause with no
check able to catch it, while an article boundary is verified by the strict
sequence below. The loss of granularity is recorded in the backlog, with the
measurement that can lift it (clause and point parsing compared against
documents that have both a source tree and a transcription).

Scopes. The main scope starts at the first article and closes at the first of: a
recipient or signature block (`Nơi nhận:`, `TM.`, `KT.`, a signing title alone
on a line, `XÁC THỰC VĂN BẢN HỢP NHẤT`), an appendix or form title (a line
wholly in capitals that begins with `PHỤ LỤC` or `MẪU SỐ`, or
`Phụ lục <number>`, `Phụ lục số <number>` or `Mẫu số <number>` (Arabic or Roman,
letters allowed after it as in `01a`) as the whole paragraph, a closing `.` or
`:` allowed; any longer mixed-case line such as `Phụ lục 1 của Thông tư này` is
running text, and an appendix whose title has that shape is still cut off by the
signature block before it), or an instrument title: a line wholly in capitals
that begins with `QUY CHẾ`, `QUY ĐỊNH` or `ĐIỀU LỆ` and is not the title line of
the structure heading just before it (`Chương I` then `QUY ĐỊNH CHUNG` is a
chapter title). Everything after it belongs to later scopes. An instrument title
followed, with no article in between, by an article numbered 1 opens an
instrument scope: a regulation "ban hành kèm theo", with anchors
`kem-theo-<k>/dieu-<n>` and the label "Điều n (văn bản kèm theo k)"; its
structure headings carry the same prefix (`kem-theo-<k>/chuong-i`). Structure
headings before the first article belong to the main scope and are ancestors of
its articles. Any other later scope (appendices, forms, footnotes, a restart at
`Điều 1` without an instrument title) is a tail scope and never asserts
articles.

Fail closed. The article-level reading is accepted for the main scope or an
instrument scope only when its article numbers run 1, 2, 3, ... where an
inserted `n<letter>` may follow `n`, with no gap, repeat or step back before the
scope closes, no failed heading candidate, no quoted text left open, and no
structure anchor used twice. If the check fails, that scope asserts no article:
its paragraphs are grouped into neutral provisions of kind `other`, anchor
`doan-<n>` numbered across the whole document, so anchors stay unique when
several scopes are neutral (a new one at each line that looks like a heading,
else every 20 paragraphs), and the document gets the gap `structure_uncertain`
with the failed check. Tail scopes use the same neutral provisions without the
gap. Structure headings are provisions only in an accepted scope; in a neutral
or tail scope a structure heading starts a new neutral provision, so every
anchor in a document is unique. Reason: a missed heading attaches text to the
wrong article silently; a strict sequence check catches it, and a neutral anchor
is a true citation where "Điều 5" might not be. Measured on recorded texts:
decree 142/2026/NĐ-CP reads as articles 1 to 46 with wrapped references such as
`Điều 6 của Luật` correctly ignored and its sample forms (which restart at
`Điều 1`) in tail scopes; a consolidated text whose footnotes repeat `Điều 4`
before its appendix fails the check and stays neutral.

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
  `none`; `pdf_text` joins with schema version 5) and
  `document.text_complete boolean`, false when any gap of `tree_headings_only`,
  `structure_uncertain`, `table_text` (more than a tenth of the characters
  inside provisions are in tables; letterhead and signature tables outside any
  provision do not count), `image_content`, `ocr_flagged`, `ocr_rejected`,
  `ocr_label_flagged` or `needs_ocr` applies; build derives `source_tree` for
  artifacts without `text.json`. Search and read results return both on the
  document entry, with each provision's locator (hits, read units, ranges and
  continued parts alike). This is corpus schema version 4.

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
document the effect article (selected by its title without the `Điều n.` label)
is review evidence: it appears in `validity-review.json` with a proposed manual
entry and changes nothing. Consolidated texts are skipped here as they are for
source trees. Reason: nothing proves a transcription or an OCR reading is
complete, and a partial text can show a commencement sentence and hide its
exception; a reviewed manual entry is the safe way to use it.

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
