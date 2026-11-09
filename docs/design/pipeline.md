# Corpus pipeline

The write path builds a corpus release on the owner's MacBook and publishes a
signed dump to S3. Decision and alternatives:
[ADR 0008](../adr/0008-write-path-temporal-self-hosted.md).

## Shape

- `deploy/compose.yml` runs PostgreSQL (workshop and Temporal persistence), the
  Temporal server and its UI locally. The machine may be off between runs;
  Temporal resumes from persisted history.
- `corpus worker` is one Go binary with one workflow,
  `BuildRelease(country, rel)`. Stage and control activities follow
  [ADR 0008](../adr/0008-write-path-temporal-self-hosted.md#history). Workflow
  ids are `build-<cc>-<rel>`. The workflow calls continue-as-new after each
  stage, so each history segment holds one stage.
- The `run` row, one per (`country`, `rel`), is the record of a run; Temporal
  only coordinates and can be rebuilt empty. The snapshot is frozen on first
  start: the parsed source list and its hash, every recipe hash,
  `taxonomy_version`, worker version, stage list and previous release manifest
  hash. Activities read the snapshot from the run row, never from the working
  tree. A worker whose compiled recipe hash differs from the snapshot for any
  stage already in it refuses the run. When a newer worker adds stages,
  `corpus start` on a paused run appends their names and recipe hashes in one
  update and records the extension in the run message; frozen recipes never
  change.
- Workflow changes use
  [`workflow.GetVersion`](https://docs.temporal.io/develop/go/workflows/versioning#patching-with-getversion).
  CI replays every history in
  `apps/corpus/internal/workflow/testdata/histories/` with the SDK
  [workflow replayer](https://pkg.go.dev/go.temporal.io/sdk/worker#WorkflowReplayer).
- Activities receive and return references (run id, generation, stage, item
  keys, artifact hashes), never text or vectors. Items are processed in fixed
  batches per activity execution; a ticker heartbeats on the interval in
  [budgets](budgets.md) while items run, so a slow item never misses a
  heartbeat.

## Run lifecycle

- States: `starting`, `running`, `cancelling`, `cancelled`, `failed`, `paused`,
  `evaluated`, `released`.
- `corpus start <cc> <rel> [--until <stage>]` locks the run row. It refuses when
  the state is `running` or `cancelling` and Temporal reports an open workflow
  for the id, and when the state is `starting` and younger than the stale limit
  in [budgets](budgets.md). Otherwise it increments `generation`, sets
  `starting` and commits, then starts the workflow with that generation and a
  fail-on-conflict id policy. The workflow's first activity moves `starting` to
  `running` only for its own generation. A crash before dispatch leaves a stale
  `starting` row that the next start replaces; a lost start response leaves a
  workflow that marks the row `running`. If an open workflow of an older
  generation holds the id, start refuses and names `corpus cancel` or
  `corpus recover`.
- Every activity first compares its generation with `run.generation`. A mismatch
  returns a non-retryable `superseded` error and the workflow ends without
  writing the run row. Control activities change state only from an allowed set
  of states and only for their own generation.
- `run.next_stage` records progress. A stage completes when no item is
  `pending`; a stage with no items completes at once. When a completed stage
  holds `failed` items the run becomes `failed` and `next_stage` stays on that
  stage; otherwise `next_stage` advances in the same transaction that computes
  the stage counts from `run_item` and `gap` rows (counts are never stored
  separately). `corpus start` resumes at `next_stage` and resets that stage's
  `failed` items to `pending`. `paused` means the workflow stopped after the
  stage named by `--until` or after the last implemented stage.
- `corpus cancel <cc> <rel>` sets `cancelling`. Stage activities check the state
  before each item and return early; the workflow waits for every batch of the
  stage to return, then sets `cancelled`. A stage that has not started its
  batches is cancelled before planning.
- `corpus recover <cc> <rel>` handles a restored workshop or a lost worker: it
  increments `generation`, sets `failed` with a message, and terminates an open
  workflow for the id only with `--terminate`. Then `corpus start` resumes.
- `corpus status <cc> <rel>` and the Temporal UI show per-stage expected, done,
  gap, failed and pending counts, unaccepted gaps by kind, next retry times,
  provider job ids and cost so far.

## Items, artifacts and gaps

- A stage plans its items, processes each into an artifact, and resolves each
  artifact against the run. Processing is a function of the item input and the
  recipe; it writes the artifact. The artifact's rows (`document`,
  `fetched_file`, `provision`, `relation`) are loaded from the artifact files,
  so an adopted directory without a committed row yields the same rows as a
  fresh one. Resolving is a function of the artifact and the run; it writes run
  rows (`listing`) and gaps. When planning already knows an item's input hash
  and the artifact exists, processing is skipped and only resolving runs, so a
  reused artifact still yields this run's gaps.
- Artifacts are immutable and content addressed:
  `artifacts/<stage>/<input_hash>-<recipe_hash>/` with `MANIFEST.sha256` (sorted
  `<sha256>  <relative path>` lines); the artifact sha256 is the manifest's
  sha256. A writer fills a temporary directory, writes the manifest, takes a
  lock file for the key, then renames into place. Under the lock a present
  directory with a valid manifest is adopted, and a partial one is moved aside
  and replaced. Paths in a manifest are relative, without `..`, symlinks or
  absolute names. Directories that no committed row references (a crash after
  rename, or content that changed before the retry) are orphans; they waste
  space but never change results.
- For `fetch` the input hash is the bundle's own manifest sha256, learned after
  download, so unchanged remote content keeps its hash. The fetch artifact
  sha256 is the input hash of `extract` and `relate`.
- One transaction commits an item: the artifact row when there is one, the
  artifact's rows when that row is new, the run rows and gaps (idempotent), and
  the `run_item` update. The update applies only when the pair
  (`run.generation`, Temporal attempt) is not lower than the recorded pair; a
  stale commit rolls back. Items that end as a gap or a failure without output
  commit without an artifact.
- Gaps are rows with a kind. `corpus accept-gaps <cc> <rel> --kind <kind>`
  accepts every gap of that kind and records the owner's note. `release` is
  refused while any expected item is `pending` or `failed` or any gap is
  unaccepted.
- Checkpoints: at every stage boundary the workflow runs a `Checkpoint` activity
  that dumps the workshop database with one `pg_dump` (one consistent snapshot
  across runs) and then copies new artifact directories, which are immutable and
  written before the rows that reference them. Temporal persistence is not part
  of a checkpoint. Recovery restores the dump and the artifacts, runs
  `corpus recover`, and resumes with `corpus start`. Submission identity does
  not depend on the checkpoint: the intent log described below lives outside the
  workshop.

## Stages

| Stage            | Input                                             | Output                                                           | Completeness rule                                                                                                                                                                         |
| ---------------- | ------------------------------------------------- | ---------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `discover`       | source list YAML; previous release manifest       | expected document set with source modified dates                 | full listing per source every run; a document absent from two consecutive listings is marked `withdrawn` and kept with a gap                                                              |
| `fetch`          | expected set                                      | metadata JSON, every listed attachment, hashes                   | every attachment is revalidated each run by size and hash of a fresh download when the source gives no ETag; a changed attachment reprocesses the document; a missing attachment is a gap |
| `extract`        | files                                             | structure tree with verbatim text                                | parser limits in [budgets](budgets.md); article count matches the table of contents; table and appendix counts match the source inventory; empty or truncated text is a gap               |
| `relate`         | VBPL references, text                             | relations with kind, evidence, affected provisions when captured | never guessed; unknown targets are gaps                                                                                                                                                   |
| `chunk`, `embed` | trees                                             | chunks, dense and sparse vectors under the release recipes       | vector count equals chunk count; dimensions equal the recipe                                                                                                                              |
| `classify`       | trees, vectors, taxonomy, seeds                   | assignments                                                      | every article has an assignment or an "unassigned" gap                                                                                                                                    |
| `build`          | all gold tables                                   | local `corpus_<cc>_<rel>` database                               | schema template applied; `release` row written                                                                                                                                            |
| `eval`           | local database, golden set, taxonomy held-out set | report; diff against the previous release                        | floors in [testing](../../instructions/testing.md) and [taxonomy](taxonomy.md)                                                                                                            |
| `release`        | evaluated database                                | `corpus_<cc>_<rel>.dump`, `manifest.json`, `manifest.sig` in S3  | refused unless every expected item is done or a gap, every gap is accepted, and eval passed                                                                                               |

VBPL ingest rules:

- `discover` lists every query until two consecutive passes return the same id
  set, at most the pass limit in [budgets](budgets.md); otherwise the query item
  gets gap `listing_unstable` and the expected set uses the union of the passes.
  A listing is complete when its query item is done without a `listing_unstable`
  gap; a query's identity is the hash of its definition (agencies and keyword).
  Earlier runs of the same country are ordered by date, then sequence. For a
  document listed by some query in an earlier complete listing, take the queries
  that ever listed it and still exist: it is `withdrawn` when every one of them
  has a complete listing in this run and in its most recent earlier complete
  listing, and both omit the document. If any of those listings is incomplete,
  the document stays expected and is fetched by id. A document whose listing
  queries are all gone from the source list is `scope_removed`. Withdrawn and
  scope-removed documents stay in the expected set as gap items that are never
  fetched.
- `fetch` stores attachments as `files/<sha256>` and maps original names in
  `files.json`; each download attempt writes a fresh temporary file and is
  bounded by the per-file limit while streaming. API responses above the
  response limit are gaps.
- `extract` refuses trees deeper or larger than the limits in
  [budgets](budgets.md) with gap `tree_too_large`.
- `relate` maps relation codes with the source list's table, whose hash is part
  of the `relate` recipe hash. Resolving a target against the run's expected
  set, and the `unknown_target` gap, happen when the artifact is resolved for a
  run, never inside the cached artifact.

Recipe changes (parser version, prompt hash, taxonomy version, model) change
`recipe_hash`, so unchanged documents are reprocessed where it matters.

## External batch submissions

The Batch API has no idempotency key and its list endpoint returns no request
metadata (first-party docs read 2026-11-08), so the design does not rely on
either. Before calling it, the activity writes a `submission` row with a new
128-bit `id` (32 hex characters) in state `preparing`, holding the ordered item
list and its hash, and appends the same record as `submissions/<id>.json` to the
intent log on S3 (or the external disk), which is never part of a checkpoint and
is never deleted. Every `custom_id` is `<id>-<item index, six digits>`, 39
characters from the provider's allowed set `[a-zA-Z0-9_-]`; the index maps to
the item key through the ordered list in the intent record. The activity creates
the batch and writes the batch id to the row and to `submissions/<id>.batch`
(`submitted`) before anything else. After storage loss,
`corpus submissions --rebuild` reconstructs rows from the intent log: a record
with a `.batch` file resumes polling; a record without one is `uncertain`. A
`preparing` row older than ten minutes with no batch id, found by any later
activity start, becomes `uncertain`; so does a create call that fails without a
response. Uncertain rows stop the stage. `corpus submissions` shows them with
request count and time window; the owner lists batches in that window and, for
each candidate that has ended, downloads its results and checks the `custom_id`
prefix. A match attaches the batch id; no match after the provider's 24-hour
processing window marks the row `abandoned`, which allows exactly one new
submission. Polling runs every five minutes until the provider reports an end
state or `deadline_at` (26 hours after submission) passes, which marks the row
`failed`. `corpus cancel` calls the cancel endpoint for in-flight batches and
marks their rows `cancelling`; the provider may still finish some requests after
acknowledging. Polling continues until the provider reports the end state, then
results are downloaded and every item outcome and the cost are recorded before
the row becomes `cancelled`. If the end state does not arrive by `deadline_at`,
the row becomes `failed` with a message for operator review. A new run for the
same release does not submit while any earlier submission is non-terminal
(`preparing`, `submitted`, `uncertain`, `cancelling`). Result download and
per-item application are idempotent on `custom_id`; malformed results become
`failed` items with at most two retries, then gaps. Cost per batch is recorded
on the row.

## Release artifact

`manifest.json` binds: `rel`, `country`, `schema_version`, `dense_recipe`,
`lexical_recipe`, `taxonomy_version`, source manifest hash, dump `sha256` and
size, evaluation report hash, built-at, worker version. `manifest.sig` is an
Ed25519 signature (minisign format) made with a key on the MacBook; the host
holds only the public key. The dump is produced with
`--no-owner --no-privileges`.

## Proofs before unattended releases

Crash after an activity commit, uncertain batch submission, laptop restart
mid-run, partial batch results, failed activation, and rollback each have a
scripted test under `apps/corpus` and a line in the first plan.
