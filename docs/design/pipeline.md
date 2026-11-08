# Corpus pipeline

The write path builds a corpus release on the owner's MacBook and publishes a
signed dump to S3. Decision and alternatives:
[ADR 0008](../adr/0008-write-path-temporal-self-hosted.md).

## Shape

- `deploy/compose.yml` runs PostgreSQL (workshop and Temporal persistence),
  the Temporal server and its UI locally. The machine may be off between runs;
  Temporal resumes from persisted history.
- `corpus worker` is one Go binary with one workflow, `BuildRelease(country,
  rel)`, and one activity per stage. Workflow ids are `build-<cc>-<rel>`; a
  second start while one is running is rejected. The workflow calls
  continue-as-new after each stage, so each history segment holds one stage.
- `corpus start <rel>` first writes the `run` row with a frozen snapshot: the
  source list revision, every recipe hash, `taxonomy_version`, worker version
  and the previous release manifest hash. Activities read the snapshot from the
  run row, never from the working tree, so a run is reproducible.
- Activities receive and return references (run id, stage, item keys, artifact
  hashes), never text or vectors. Items are processed in fixed batches per
  activity execution with heartbeats; the size is in [budgets](budgets.md).
- Artifacts are immutable and content addressed:
  `artifacts/<stage>/<input_hash>-<recipe_hash>/`. An activity skips an item
  whose artifact row exists, writes new output to a temporary directory,
  verifies it, renames it into place, then inserts the `artifact` row and
  updates `run_item`. If a retry finds the directory present without a row, it
  verifies the sha256 and inserts the row; if the directory is partial, it
  deletes and redoes it. Fencing uses the pair (`run.generation`, Temporal
  attempt): `generation` increments on every `corpus start` for the release,
  so it survives restarts; a commit whose pair is lower than the recorded pair
  is ignored. When two activities publish the same artifact directory, the
  first successful rename wins and the other verifies and adopts it; the
  filesystem rename is the atomic winner selection. Different releases share
  artifacts by hash without conflict.
- `corpus status <rel>` and the Temporal UI show per-stage expected, done, gap,
  failed and pending counts, next retry times, provider job ids and cost so far.
- Run states: `running`, `cancelling`, `cancelled`, `failed`, `evaluated`,
  `released`. `corpus cancel <rel>` requests cancellation; activities finish
  their current batch and the run becomes `cancelled`. `corpus start <rel>`
  after `cancelled` or `failed` starts a new workflow with the same snapshot and
  reuses artifacts. A gap becomes accepted only through `corpus accept-gaps
  <rel>`, which records the owner's approval per gap kind; `release` refuses
  unaccepted gaps.
- Checkpoints: at every stage boundary, before continue-as-new, the workflow
  runs a `checkpoint` activity that dumps the workshop database, syncs
  `artifacts/` and dumps Temporal persistence to an external disk or S3. No
  activity is in flight at that point, so the three copies are consistent.
  Recovery from storage loss restores the latest checkpoint and restarts the
  run with a new generation; the stage in progress is redone from its
  artifacts, and submissions recorded after the checkpoint are reconciled by
  the rule below before any new submission.

## Stages

| Stage      | Input                                                   | Output                                                        | Completeness rule                                                        |
| ---------- | ------------------------------------------------------- | ------------------------------------------------------------- | ------------------------------------------------------------------------ |
| `discover` | source list YAML; previous release manifest             | expected document set with source modified dates             | full listing per source every run; a document absent from two consecutive listings is marked `withdrawn` and kept with a gap |
| `fetch`    | expected set                                            | metadata JSON, every listed attachment, hashes               | every attachment is revalidated each run by size and hash of a fresh download when the source gives no ETag; a changed attachment reprocesses the document; a missing attachment is a gap |
| `extract`  | files                                                   | structure tree with verbatim text                            | parser limits in [budgets](budgets.md); article count matches the table of contents; table and appendix counts match the source inventory; empty or truncated text is a gap |
| `relate`   | VBPL references, text                                   | relations with kind, evidence, affected provisions when captured | never guessed; unknown targets are gaps                                |
| `chunk`, `embed` | trees                                             | chunks, dense and sparse vectors under the release recipes   | vector count equals chunk count; dimensions equal the recipe               |
| `classify` | trees, vectors, taxonomy, seeds                         | assignments                                                   | every article has an assignment or an "unassigned" gap                     |
| `build`    | all gold tables                                         | local `corpus_<cc>_<rel>` database                            | schema template applied; `release` row written                             |
| `eval`     | local database, golden set, taxonomy held-out set       | report; diff against the previous release                     | floors in [testing](../../instructions/testing.md) and [taxonomy](taxonomy.md) |
| `release`  | evaluated database                                      | `corpus_<cc>_<rel>.dump`, `manifest.json`, `manifest.sig` in S3 | refused unless every expected item is done or an accepted gap and eval passed |

Recipe changes (parser version, prompt hash, taxonomy version, model) change
`recipe_hash`, so unchanged documents are reprocessed where it matters.

## External batch submissions

The Batch API has no idempotency key and its list endpoint returns no
request metadata (first-party docs read 2026-11-08), so the design does not rely
on either. Before calling it, the activity writes a `submission` row with a
new UUID `id` in state `preparing`, holding the ordered item list and its hash.
Every `custom_id` is `<submission id>:<item key>`, so any batch result
identifies its submission exactly even when item keys recur across releases.
The activity creates the batch and stores the returned batch id in the row
(`submitted`) before anything else. A `preparing` row older than ten minutes
with no batch id, found by any later activity start, becomes `uncertain`; so
does a create call that fails without a response. Uncertain rows stop the
stage. `corpus submissions` shows them with request count and time window;
the owner lists batches in that window and, for each candidate that has
ended, downloads its results and checks the `custom_id` prefix. A match
attaches the batch id; no match after the provider's 24-hour processing
window marks the row `abandoned`, which allows exactly one new submission.
Polling runs every five minutes until the provider reports an end state or
`deadline_at` (26 hours after submission) passes, which marks the row
`failed`. `corpus cancel` cancels in-flight batches through the cancel
endpoint and marks their rows `cancelled`. Result download and per-item
application are idempotent on `custom_id`; malformed results become `failed`
items with at most two retries, then gaps. Cost per batch is recorded on the
row.

## Release artifact

`manifest.json` binds: `rel`, `country`, `schema_version`, `dense_recipe`,
`lexical_recipe`, `taxonomy_version`, source manifest hash, dump `sha256` and
size, evaluation report hash, built-at, worker version. `manifest.sig` is an
Ed25519 signature (minisign format) made with a key on the MacBook; the host
holds only the public key. The dump is produced with `--no-owner
--no-privileges`.

## Proofs before unattended releases

Crash after an activity commit, uncertain batch submission, laptop restart
mid-run, partial batch results, failed activation, and rollback each have a
scripted test under `apps/corpus` and a line in the first plan.
