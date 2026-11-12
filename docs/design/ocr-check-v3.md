# OCR cross-check version 3

Version 3 of the [OCR cross-check](ocr-check.md): its renderer, request mode and
part of its budget. The cross-check note owns what a check is, verdicts, checker
ids, running checks and limits; this note owns only what version 3 changes.

Owner decision, 2026-11-12: check every stored OCR text again with a stronger
reading, quickly, and keep the version 2 results beside the new ones.

| Setting         | Version 2                | Version 3                                                                       |
| --------------- | ------------------------ | ------------------------------------------------------------------------------- |
| Checker id      | `.../medium/ocr-check-2` | `claude-opus-5-5/high/ocr-check-3`                                              |
| Effort          | medium                   | high                                                                            |
| Page images     | PNG, 150 dpi, grayscale  | JPEG, 200 dpi, quality 90                                                       |
| Renderer        | one process per page     | one process per group of same-size pages, as [fresh OCR](ocr-vision.md#request) |
| Mode            | batch                    | direct, 8 requests in flight                                                    |
| Instruction     | version 2 text           | the same text                                                                   |
| Schema, verdict | as above                 | the same                                                                        |

- Input. The request is built as for version 2 with JPEG page images in place of
  PNG: page labels, images, the OCR text between its markers, the instruction.
  Images are read from the renderer's page files one document at a time; the
  request body is assembled once per attempt. An A4 page at 200 dpi is 1,654 by
  2,339 pixels, about 5,000 image tokens by the formula in
  [limits](ocr-check.md#limits) against 2,800 at 150 dpi, if the model reads it
  at full size; the first ten documents measure the real figure.
- Rendering bounds are the JPEG renderer's: 7 MB per page file, the 256 MB
  scratch ceiling, and 30 seconds plus 2 seconds per page. A page or scratch
  overflow is `over_check_limit`; any other render failure is `render_failed`.
- Size, decided in this order before any generation request: locally, more than
  500 pages or a request body over 100 MB is `over_check_limit`; then the free
  token count: counted input tokens plus the largest output (32,000) must not
  exceed 1,000,000, the model's context window, else `over_check_limit`. At 200
  dpi that excludes documents of roughly 150 pages and more: 3 to 5 of the 327
  in the store on 2026-11-12. They keep their version 2 result.
- Concurrency. Up to 8 documents are in flight. A document is started only after
  its worst case (counted input plus the largest output, at the listed price)
  has been reserved against `--max-usd` in one atomic step shared by all
  workers; when the reservation does not fit, no new document starts and the
  ones in flight finish. An `uncertain` attempt stops new starts the same way.
  Rendering runs before the reservation and costs nothing. A crash leaves the
  attempts in flight as `preparing` with their worst case reserved. Recovery is
  what direct mode has today, for each of them and whatever key the next
  invocation uses: they block new work for their entries, become `uncertain`
  after ten minutes, cannot be reconciled (there is no batch to list), are never
  sent again automatically, and only the operator's `--abandon` (at once for
  these direct attempts) frees the entry; the reserved charge stays counted. The
  lost answers are not recovered.
- Scope. From version 3 on the check selects every store entry of the run's OCR
  documents under any accepted engine id, not only the entry the engine order
  prefers, so a comparison text stored beside an imported one is checked too
  (three PDFs on 2026-11-12). `extract` keeps choosing one entry by engine order
  and reads only its recipe's checker id. A result under version 2 does not
  count: every stored text is checked again. The retry rule applies per entry
  and checker id.
- Spend. `--max-usd` stays cumulative per checker id, read from the attempt
  rows, so a pass can continue under another API key with a higher cumulative
  limit. The key is whatever `ANTHROPIC_API_KEY` holds in the environment of
  that one command; the operator passes it per command and never exports it for
  the session. `--limit-docs` bounds an invocation, and the summary of each
  invocation reports cost and verdict counts, so the pass is reported in steps.
- Batch mode with version 3 settings is not offered: the pass is direct.
- Resolution is part of the version. If the first ten documents project the
  store above the agreed cost, the pass uses version 4 instead:
  `claude-opus-5-5/high/ocr-check-4`, identical except for 150 dpi. The version
  3 results already written stay as they are; a version is never changed inside
  a pass.
