# External batch submissions

This note owns how pipeline stages submit work to external batch services.
[Pipeline](pipeline.md) owns the stages and their items.

The Batch API has no idempotency key and its list endpoint returns no request
metadata (first-party docs read 2026-11-08), so the design does not rely on
either. Before calling it, the activity writes a `submission` row with a new
128-bit `id` (32 hex characters) in state `preparing`, holding the ordered item
list and its hash, and appends the same record as `submissions/<id>.json` to the
intent log on OCI Object Storage (or the external disk), which is never part of
a checkpoint and is never deleted. Every `custom_id` is
`<id>-<item index, six digits>`, 39 characters from the provider's allowed set
`[a-zA-Z0-9_-]`; the index maps to the item key through the ordered list in the
intent record. The activity creates the batch and writes the batch id to the row
and to `submissions/<id>.batch` (`submitted`) before anything else. After
storage loss, `corpus submissions --rebuild` reconstructs rows from the intent
log: a record with a `.batch` file resumes polling; a record without one is
`uncertain`. A `preparing` row older than ten minutes with no batch id, found by
any later activity start, becomes `uncertain`; so does a create call that fails
without a response. Uncertain rows stop the stage. `corpus submissions` shows
them with request count and time window; the owner lists batches in that window
and, for each candidate that has ended, downloads its results and checks the
`custom_id` prefix. A match attaches the batch id; no match after the provider's
24-hour processing window marks the row `abandoned`, which allows exactly one
new submission. Polling runs every five minutes until the provider reports an
end state or `deadline_at` (26 hours after submission) passes, which marks the
row `failed`. `corpus cancel` calls the cancel endpoint for in-flight batches
and marks their rows `cancelling`; the provider may still finish some requests
after acknowledging. Polling continues until the provider reports the end state,
then results are downloaded and every item outcome and the cost are recorded
before the row becomes `cancelled`. If the end state does not arrive by
`deadline_at`, the row becomes `failed` with a message for operator review. A
new run for the same release does not submit while any earlier submission is
non-terminal (`preparing`, `submitted`, `uncertain`, `cancelling`). Result
download and per-item application are idempotent on `custom_id`; malformed
results become `failed` items with at most two retries, then gaps. Cost per
batch is recorded on the row.
