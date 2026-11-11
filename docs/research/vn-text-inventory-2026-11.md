# Text inventory of documents without a tree (2026-11-11)

Measured on the fetch artifacts of run `vn/20261110_5` for the 562 documents
whose source tree is empty (553 VBPL, 9 from vanban.chinhphu.vn). Not authority.
A read-only script classified every HTML variant and attachment; PDF text was
sampled from the first three pages and the middle page with `pypdf`. Feeds
[text extraction](../design/text-extraction.md).

## First usable text source per document

| Source                                                      | Documents |
| ----------------------------------------------------------- | --------- |
| `<id>_content.html` with at least 300 characters of text    | 188       |
| PDF with a Unicode Vietnamese text layer                    | 23        |
| PDF text layer present but not Vietnamese (earlier bad OCR) | 203       |
| Scanned PDF without a text layer                            | 88        |
| No attachment (58), only `.doc` (1), only `.rar` (1)        | 60        |

The rows sum to 562 and include the secondary documents: one of the 23
(05/2026/TT-BKHCN) and eight of the 88, among them 142/2026/NĐ-CP (97 pages).
The nine secondary documents have one signed PDF each.

## HTML transcriptions

Of 544 documents with `_content.html`, 356 are empty (under 300 characters). Of
the 188 with text: 73 have two or more `Điều` lines, 80 a numbered outline, 35
plain text; 46 have more than two tables and 9 have images. 485 documents also
carry `_content_origin.html`: both empty in 333, identical text in 98, different
text in 49, text only in `_content.html` in 5, text only in the origin in none.
A few documents carry `_body_content.html` or dated backups.

## PDFs

915 PDFs in 387 documents; 278 documents have more than one. A sampled garbage
layer reads `ceNG HoA xA ngr cHU xcnia. vIET NAM` for "CỘNG HÒA XÃ HỘI CHỦ NGHĨA
VIỆT NAM": Latin letters with no Vietnamese diacritics. Counting letters with
diacritics separated the two groups cleanly: the garbage layers had none in the
sample pages. 36 DOCX and 66 DOC files exist, all in documents that also have a
usable transcription or PDF, except one document with only a `.doc`.

## OCR workload

Documents with text only in a PDF: 314. banhmi's cache holds Google Vision OCR
text keyed by the PDF's SHA-256 for 266 of them (261 list one distinct PDF under
several names, 5 have exactly one PDF with cached text); 48 documents, 1,342
pages, need fresh OCR. Pages over the 291 documents without a good text layer:
about 10,500 taking the largest PDF per document; median 25 pages, 95th
percentile 90, largest 507. By type among the VBPL ones: 263 consolidated texts,
14 decrees, 6 circulars, 6 decisions, 1 law, 1 telegram.

## By document type (VBPL, 553)

| Type             | HTML | PDF text | Needs OCR | Nothing |
| ---------------- | ---- | -------- | --------- | ------- |
| Văn bản hợp nhất | 43   | 16       | 263       | 39      |
| Thông tư         | 46   | 6        | 6         | 10      |
| Chỉ thị          | 58   | 0        | 0         | 0       |
| Nghị định        | 5    | 0        | 14        | 1       |

Counts in the last three columns for consolidated texts and circulars are
derived from the strata above and the per-type totals; the per-type split of
"nothing" was not measured separately and is the remainder.

## Limits of this measurement

Text layers were sampled, not read in full, so a PDF counted as having a text
layer may still have image-only pages. Reading order, tables and formulas were
not checked. Role labels in the bundle (`main`, `appendix`) are mostly
`unknown`, so "the main PDF" is not yet defined for the 278 documents with
several PDFs.
