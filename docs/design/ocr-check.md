# OCR cross-check

This note owns how a stored OCR text is checked against its PDF before `extract`
may use it. [ADR 0014](../adr/0014-ocr-on-the-write-path.md) owns the decision,
[OCR store](ocr-store.md) the records, [budgets](budgets.md#pipeline) the
limits. The fresh OCR client for PDFs absent from the store is a later note.

## What a check is

One check reads one PDF together with the stored OCR text of that PDF and
answers one question: where does the OCR text disagree with the file. The
checker is a hosted Claude model on the Claude API. It receives the pages of the
PDF as images, each after a `Page N:` label, then the OCR text between fixed
markers, then a fixed instruction. Nothing else is sent: no user question, no
personal data, only public legal text.

The PDF itself is not sent. Version 1 sent it as a document block, and the API
then gives the model each page's embedded text layer beside the page image. For
most of these scans that layer is the garbage of an earlier OCR pass, and the
first pilot (10 documents, 2026-11-11) showed the model taking it for the text
under check: its "OCR" quotes were strings that do not occur in our OCR text at
all. So the pipeline renders the pages itself and sends images only; the
instruction also says that the images are the authority and that stamps,
handwriting, received marks and signature blocks are not checked.

The answer is structured output under a JSON schema:

- `same_document`: whether the OCR text is a reading of this PDF at all.
- `pages`: the number of pages the checker read. The pipeline counts the PDF's
  pages itself before sending and stores that count with the result as
  `pdf_pages`; the two must be equal.
- `items`: disagreements, each with `kind` (`document_number`, `date`,
  `article_label`, `amount`, `missing_text`, `extra_text`), `page`, `ocr` (the
  OCR reading), `pdf` (the file's reading) and a short `note`. Differences of
  spacing, line breaks, page headers and page numbers are not items.

The answer must be verifiable against what the pipeline holds. A reading is
empty when nothing is left after whitespace is collapsed. Each kind fixes which
readings an item carries, and an item that breaks its rule fails the attempt
(`item_readings`):

- `document_number`, `date`, `article_label`, `amount`: both readings present.
- `extra_text`: `ocr` present, `pdf` empty.
- `missing_text`: `ocr` empty, `pdf` present.

Every `ocr` reading that is present must occur in the stored OCR text after
whitespace is collapsed; an answer with a quote that does not is a failed
attempt (`ocr_quote_not_found`), because it shows the checker was not reading
the text under check. The check proves only that the quote exists in the text,
not that it sits on the named page. A `missing_text` item cannot be verified by
the pipeline, which holds no reading of the page: it is a claim for the
reviewer, and it can only make a verdict stricter (flagged or rejected), never
cleaner. An item whose `ocr` and `pdf` readings are equal after whitespace is
collapsed is not a disagreement: the pipeline drops it and records how many it
dropped.

The checker never rewrites the stored text; its answer is evidence beside it.

## Verdicts

A check is complete only when the request ended normally (`end_turn`), the
answer parses under the schema with no unknown, repeated or differently cased
field, and `pages` equals the page count the pipeline established for the PDF. A
PDF whose pages cannot be counted is never checked (`page_count_unknown`), and
an answer that reports another number is a failed attempt
(`page_count_mismatch`). The stored result is the validated answer serialized
again by the pipeline, never the bytes the checker sent. Then:

- `rejected`: `same_document` is false, or `missing_text` items cover more than
  a tenth of the pages. The text must not be indexed; the document gets the gap
  `ocr_rejected` with the reason.
- `flagged`: one or more items. An `article_label` or `document_number` item
  makes OCR unusable with the gap `ocr_label_flagged`; otherwise the text is
  indexed with `ocr_flagged`. The items go to the review file.
- `clean`: no item.

A complete check writes the `ocr_check` row with the verdict inside the result.
Anything else (a refusal, a cut-off answer, a timeout, a transport or API error,
an answer that fails the schema, a PDF over the API's page or size limit) writes
no `ocr_check` row: the document stays `needs_ocr`, and the attempt is recorded
with its reason. A NUL in any answer string fails the schema (`schema:nul`).
Reason: an unfinished check must never read as a clean one.

## Checker id

`<model id>/<effort>/ocr-check-<n>`, for example
`claude-sonnet-5-5/medium/ocr-check-2`. `n` versions the instruction, the
schema, the input form and the rendering settings together (version 1: the PDF
as a document block; version 2: page images at 150 dpi, grayscale PNG, one
render process per page). Version 3 takes JPEG pages at 200 dpi and is described
[below](#version-3). A version is never changed inside a pass. The extract
recipe names one checker id; changing the model, the effort, the instruction or
the schema is a new id, new check rows and a rerun of extract for the affected
documents. The release checker id is `claude-opus-5-5/medium/ocr-check-2`. The
pilot and full pass support this choice: all 49 spot-read items were correct. It
stays the release id until the new pass has covered the store under one version,
3 or its 150 dpi fallback 4; a recipe change then names the checker id that
completed the pass.

## Version 3

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
  2,339 pixels, about 5,000 image tokens by the formula in [limits](#limits)
  against 2,800 at 150 dpi, if the model reads it at full size; the first ten
  documents measure the real figure.
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

## Running checks

`corpus ocr check <cc> <rel>` selects the run's documents whose text source
would be `ocr`, that have a store entry and no `ocr_check` row for the checker
id. Flags: `--model`, `--effort`, `--limit-docs`, `--max-usd` (required),
`--batch`, `--dry-run`, `--retry-failed`.

An ordinary invocation skips a document after two billed failed attempts for the
checker id, or when its latest attempt failed before a request with no recorded
cost or input tokens; the tool version is not recorded, so the latter rule uses
the failure reason alone. `--retry-failed` allows one more attempt per skipped
document per invocation, and repeating the flag repeats the attempt at the
operator's choice.

- Dry run: counts input tokens per document with the token counting endpoint and
  prints the estimate at the model's listed price. It writes nothing.
- Direct mode sends one request per document, one at a time. It is for the pilot
  and for retries. A request whose final usage never arrived (a dropped
  connection after the request was sent, a cut stream) is `uncertain`: the
  provider may have billed all of it.
- Batch mode submits Message Batches (half price). A batch holds at most 200 MB
  of requests. Submission follows
  [external batch submissions](batch-submissions.md) in the form this command
  needs: before the network call every attempt of the batch is written in state
  `preparing` with a new 128-bit submission id, and each request's `custom_id`
  is `<submission id>-<ocr_text row id>`. The batch id is written to those rows
  (`submitted`) before anything else happens. A `preparing` attempt older than
  ten minutes, or a create call that failed without a response, is `uncertain`:
  the provider may hold a paid batch nobody recorded. Uncertain attempts block
  new attempts for their entries. `corpus ocr check --reconcile` lists the
  provider's batches created in that window and matches the `custom_id` prefix
  in their results; a match attaches the batch id. An empty or unmatched listing
  proves nothing (another workspace's key lists other batches), so
  reconciliation never abandons an attempt. Only the operator does, with
  `corpus ocr check --abandon <submission id>` after looking at the provider's
  console, and for a batch not within 24 hours of the attempt, because a batch
  can still complete and bill up to 24 hours later. A direct attempt cannot:
  once its connection is gone no later result exists, so it may be abandoned at
  once; its worst-case charge stays counted against the cap; that allows one new
  submission. A later invocation collects the results of `submitted` attempts
  instead of submitting again; an expired or errored item becomes a failed
  attempt that a later invocation may resubmit under the retry rule. The records
  live in the workshop database, which run checkpoints dump; losing it beyond
  the last checkpoint loses the check rows and the spend written since, so that
  work is paid again when it is repeated, each invocation still bounded by its
  `--max-usd`.
- Spend: each attempt records the usage the API reported; the command adds the
  cost at the listed price and stops before a request or batch that could pass
  `--max-usd`, counting the largest possible output. An `uncertain` or
  `abandoned` attempt counts at its worst case (counted input plus the largest
  output) until a result with real usage replaces it, and an `uncertain` attempt
  stops all further spending by that command until the operator resolves it.
- Every attempt is a row in `ocr_check_attempt` (entry, checker id, mode, batch
  id, state, reason, usage). The table is working state and may be updated; the
  `ocr_check` row it leads to is immutable. Usage records request body bytes,
  response event counts and safe error class and type without error text.

Credentials come from `ANTHROPIC_API_KEY` in the environment. The key, request
bodies, document text and provider error text are never logged. The API stores
batch inputs and results for 29 days
([batch processing](https://platform.claude.com/docs/en/build-with-claude/batch-processing),
read 2026-11-11); this is why only public legal text may be sent.

## Limits

The page count comes from the PDF's page tree, read by a PDF library inside the
check command only, under the file size limit and a ten-second limit per file; a
library error or panic is `page_count_unknown`. `extract` and the worker still
parse no PDF.

Rendering. The check command renders pages with `pdftoppm` from poppler at 150
dpi, grayscale, PNG. The PDF is untrusted, so the renderer is contained and
bounded before it allocates anything:

- It runs inside an operating-system sandbox that denies all network access and
  allows writing only in a scratch directory of its own and reading only the PDF
  and the tool's runtime files (`sandbox-exec` with a fixed profile on macOS; a
  container or namespace sandbox elsewhere). When no sandbox can be enforced the
  command refuses to run: that is a configuration error, never a reason to
  render unconfined. Its environment holds no credentials.
- Page sizes are read first (`pdfinfo`, in the same sandbox). The checker
  renders the CropBox (`-cropbox`) and lowers the resolution from the larger of
  the MediaBox and CropBox extents, with an explicit raster cap of 3,000 pixels
  per side, so no page box can make the tool allocate a larger image.
- The sandbox's read permissions are the tool binaries, the dynamic libraries
  they link (resolved once, transitively), the font configuration and data
  poppler reads, and metadata of the directories those libraries sit in; nothing
  else under the package manager's tree or the user's home is readable.
- Pages are rendered one at a time; each output is checked (dimensions, 10 MB),
  re-encoded and removed from scratch before the next page is rendered; scratch
  space stays under 256 MB and the request under 100 MB.
- The limit is two minutes per document. On cancellation or timeout the whole
  process group is killed and reaped and its pipes are closed after a short
  wait, so no descendant survives and the scratch directory is always removed.
  The number of rendered pages must equal the counted page total, else the
  attempt fails with `page_count_mismatch`. A missing renderer, a timeout or a
  nonzero exit is `render_failed`; the document stays `needs_ocr`. poppler is a
  tool of the write path on the MacBook and is named in the runbook; nothing on
  the host needs it. Two alternatives were set aside: removing the text layer
  from a copy of the PDF also removes the real text of the PDFs that were born
  digital, and a renderer bound to macOS frameworks would tie the pipeline to
  one platform.

Cost of the input form. An image costs `ceil(width/28) x ceil(height/28)` tokens
([vision](https://platform.claude.com/docs/en/build-with-claude/vision), read
2026-11-11), about 2,800 for an A4 page at 150 dpi; the pilot measures the real
figure.

A request carries the pages of one PDF: at most 500 pages, each image under 10
MB, the request under 100 MB (the API allows 600 images and 128 MB, and no side
above 3,000 pixels once a request has more than 20 images); a larger document is
a failed attempt with reason `over_check_limit` and waits for a later design.
The PDF's SHA-256 is verified against the store entry before it is rendered. PDF
and OCR text are untrusted content: the instruction tells the checker to treat
them as data, and the structured answer is the only thing read back.

## Review file

`corpus ocr review <cc> <rel>` writes `ocr-review.json`: per document the
verdict, the items and the anchors of the provisions that contain each OCR
reading, so a person can open the PDF page and decide on a manual correction
under [validity corrections](validity-status.md) or accept the flag.

## Pilot

Before the full pass the primary runs direct mode on 10 documents with each
candidate model, reads every item against the PDF, and records tokens, cost,
true and false flags in a research note. A failed attempt records which rule it
broke (the field or rule name, never document content), so a failure can be
diagnosed from the attempt row. The owner sees the measured cost for the full
pass before it starts.
