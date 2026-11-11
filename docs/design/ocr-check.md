# OCR cross-check

This note owns how a stored OCR text is checked against its PDF before `extract`
may use it. [ADR 0014](../adr/0014-ocr-on-the-write-path.md) owns the decision,
[OCR store](ocr-store.md) the records, [budgets](budgets.md#pipeline) the
limits. The fresh OCR client for PDFs absent from the store is a later note.

## What a check is

One check reads one PDF together with the stored OCR text of that PDF and
answers one question: where does the OCR text disagree with the file. The
checker is a hosted Claude model on the Claude API. It receives the PDF as a
document block (the API reads each page as text and image, so the pipeline needs
no PDF renderer), then the OCR text, then a fixed instruction. Nothing else is
sent: no user question, no personal data, only public legal text.

The answer is structured output under a JSON schema:

- `same_document`: whether the OCR text is a reading of this PDF at all.
- `pages`: the number of pages the checker read. The pipeline counts the PDF's
  pages itself before sending and stores that count with the result as
  `pdf_pages`; the two must be equal.
- `items`: disagreements, each with `kind` (`document_number`, `date`,
  `article_label`, `amount`, `missing_text`, `extra_text`), `page`, `ocr` (the
  OCR reading), `pdf` (the file's reading) and a short `note`. Differences of
  spacing, line breaks, page headers and page numbers are not items.

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
- `flagged`: one or more items. The text is indexed with the gap `ocr_flagged`
  and the items go to the review file.
- `clean`: no item.

A complete check writes the `ocr_check` row with the verdict inside the result.
Anything else (a refusal, a cut-off answer, a timeout, a transport or API error,
an answer that fails the schema, a PDF over the API's page or size limit) writes
no `ocr_check` row: the document stays `needs_ocr`, and the attempt is recorded
with its reason. Reason: an unfinished check must never read as a clean one.

## Checker id

`<model id>/<effort>/ocr-check-<n>`, for example
`claude-sonnet-5-5/medium/ocr-check-1`. `n` versions the instruction and the
schema together. The extract recipe names one checker id; changing the model,
the effort, the instruction or the schema is a new id, new check rows and a
rerun of extract for the affected documents. The id used for a release is chosen
after the pilot and recorded here.

## Running checks

`corpus ocr check <cc> <rel>` selects the run's documents whose text source
would be `ocr`, that have a store entry and no `ocr_check` row for the checker
id. Flags: `--model`, `--effort`, `--limit-docs`, `--max-usd` (required),
`--batch`, `--dry-run`.

- Dry run: counts input tokens per document with the token counting endpoint and
  prints the estimate at the model's listed price. It writes nothing.
- Direct mode sends one request per document, a few at a time. It is for the
  pilot and for retries.
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
  in their results; a match attaches the batch id, and no match 24 hours after
  the attempt marks it `abandoned`, which allows one new submission. A later
  invocation collects the results of `submitted` attempts instead of submitting
  again; an expired or errored item becomes a failed attempt that the next
  invocation may resubmit. The records live in the workshop database only; if it
  is lost, at most the batches then in flight are paid twice, bounded by
  `--max-usd`.
- Spend: each attempt records the usage the API reported; the command adds the
  cost at the listed price and stops before a request or batch that could pass
  `--max-usd`, counting the largest possible output.
- Every attempt is a row in `ocr_check_attempt` (entry, checker id, mode, batch
  id, state, reason, usage). The table is working state and may be updated; the
  `ocr_check` row it leads to is immutable.

Credentials come from `ANTHROPIC_API_KEY` in the environment. The key, request
bodies and document text are never logged; errors keep the status and the
request id only. The API stores batch inputs and results for 29 days
([batch processing](https://platform.claude.com/docs/en/build-with-claude/batch-processing),
read 2026-11-11); this is why only public legal text may be sent.

## Limits

The page count comes from the PDF's page tree, read by a PDF library inside the
check command only, under the file size limit and a ten-second limit per file; a
library error or panic is `page_count_unknown`. `extract` and the worker still
parse no PDF.

A request carries one PDF of at most 600 pages and 30 MB with its OCR text
([PDF support](https://platform.claude.com/docs/en/build-with-claude/pdf-support),
read 2026-11-11); a larger file is a failed attempt with reason
`over_check_limit` and waits for a later design. The PDF's SHA-256 is verified
against the store entry before it is sent. PDF and OCR text are untrusted
content: the instruction tells the checker to treat them as data, and the
structured answer is the only thing read back.

## Review file

`corpus ocr review <cc> <rel>` writes `ocr-review.json`: per document the
verdict, the items and the anchors of the provisions that contain each OCR
reading, so a person can open the PDF page and decide on a manual correction
under [validity corrections](validity-status.md) or accept the flag.

## Pilot

Before the full pass the primary runs direct mode on 50 pages across 10
documents with each candidate model, reads every item against the PDF, and
records tokens, cost, true and false flags in a research note. The owner sees
the measured cost for the full pass before it starts.
