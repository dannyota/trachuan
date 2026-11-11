# OCR store

This note owns where OCR text and its cross-checks are kept and how `extract`
reads them. [ADR 0014](../adr/0014-ocr-on-the-write-path.md) owns the decision;
[text extraction](text-extraction.md) owns source selection and parsing. The
cross-check and fresh OCR clients have their own later note; this one fixes the
records they write.

## Records

OCR text is kept outside run artifacts because it is paid for once and reused by
every run. Two workshop tables index it, and the text itself is a file:

- Text file: `<CORPUS_OCR_STORE>/text/<text sha256>.txt`, UTF-8, the engine's
  output byte for byte, written once under a temporary name and renamed. A file
  that exists is never rewritten.
- `ocr_text`: one row per PDF SHA-256 and engine id, with the text's SHA-256 and
  size, the origin (`banhmi` for imported output, `vision` for a fresh request),
  the page count when the engine reported pages (unknown for imported text) and
  the creation time.
- `ocr_check`: one row per `ocr_text` row and checker id, written only for a
  complete check as [OCR cross-check](ocr-check.md#verdicts) defines it: the
  whole PDF went to the checker in one request, the answer parsed under the
  result schema, and it reports the pages read (equal to the entry's page count
  when the engine recorded one). The store validates that contract (verdict,
  pages, items) before it writes the row, which holds the SHA-256 of the
  canonical result JSON, the result and the number of flagged items. A check in
  progress or a failed one has no row here.

Both tables refuse `UPDATE` and `DELETE` by trigger. A new engine version or a
new checker (model or prompt) is a new id and so a new row; a release can always
name the reading it was built from.

Ids. An engine id names the engine and version as its operator names them:
`vision-ocr/builtin-latest` for banhmi's stored Google Vision output, and the
fresh client's id when it exists. A checker id names the model and the prompt
version. The extract recipe carries the ordered list of accepted engine ids and
the one checker id, so changing either reruns extract.

Writing an entry: the text file is written and synced, linked to its final name,
and its directory synced, before the row is inserted; a row therefore never
names a file that a crash could lose.

## Import of banhmi output

`corpus ocr import-banhmi <cc> <rel> <dir>` reads `<dir>/ocr/<sha256>.txt`. A
file is imported only when its name is the SHA-256 of a PDF listed in a fetch
bundle of that run; others are counted and skipped. A PDF that already has a row
for the engine id is skipped, never overwritten. The command needs no network
and no credential. The directory is untrusted: each file is opened without
following symbolic links and without blocking on special files, and the checks
(regular file, size under the 50 MB file limit, valid UTF-8) run on the opened
file, never on a path looked up twice. The command and prints counts: imported,
already present, not in the run, rejected.

## How extract reads the store

For a document whose text source would be `ocr`, planning looks up the
document's PDF by SHA-256 (one distinct PDF in the bundle, or the only one with
a store row; several candidates are `text_source_ambiguous`). The entry is the
row of the first accepted engine id that exists. It is usable only when
`ocr_check` has a row for the recipe's checker id.

- Usable: the input hash freezes the fetch hash, `ocr`, the text SHA-256 and the
  check result SHA-256. `text.json` records the PDF name and SHA-256, engine id,
  text SHA-256, checker id, result SHA-256 and flagged count. One or more
  flagged items add the gap `ocr_flagged`; the text is still indexed.
- No entry, or no completed check: the input hash freezes `absent` and the gap
  the selection ends with (`needs_ocr`, `text_source_ambiguous`,
  `file_format_unsupported` or `no_text_source`), so a store change that moves a
  document from one gap to another reruns extract instead of reusing the
  artifact with the old gap.

Planning records what it selected. The store can change between planning and
processing, and a planned item keeps its input hash for the life of the run, so
for every document whose selection consulted the store planning writes one row
in `run_ocr_selection` (run, item key, gap or the `ocr_text` and `ocr_check` row
ids, PDF SHA-256). Planning first reads that row and derives the input hash from
it; only when none exists does it consult the store and insert one. Processing
reads the row and never selects from the live store. A frozen run therefore
processes exactly what it planned, and a new run sees the store as it is then. A
heading-only tree kept because no OCR text was usable is such a document too:
its row carries the gap `tree_headings_only` and its input hash stays the fetch
hash. A check with verdict `rejected` gives the gap `ocr_rejected` instead of
`needs_ocr`, so the reason is frozen with the selection and the artifact is not
shared with the unchecked state.

`extract` verifies the text file against its SHA-256 before parsing and fails
the item, never the run, on a mismatch or a missing file. Without
`CORPUS_OCR_STORE` the store is empty: every such document is `needs_ocr`.

Locators for OCR text are `line:<first>-<last>`, the 1-based lines of the stored
text file that the provision covers. Imported text has no page breaks, so a line
range is the only position every OCR text has.
