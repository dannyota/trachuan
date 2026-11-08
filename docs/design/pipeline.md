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
  second start with the same id is rejected, which fences overlapping runs.
- Activities receive and return references (run id, stage, item keys, artifact
  hashes), never text or vectors. Items are processed in batches of 100
  documents per activity execution with heartbeats.
- Every activity is idempotent: it computes `recipe_hash` for its stage, skips
  items whose `(input_hash, recipe_hash)` artifact exists, writes new output to
  a temporary path, verifies it, then renames and records the `artifact` and
  `run_item` rows in one transaction.
- `corpus status <rel>` and the Temporal UI show per-stage expected, done, gap,
  failed and pending counts, next retry times, provider job ids and cost so far.
  `corpus cancel <rel>` cancels the workflow; completed artifacts stay.

## Stages

| Stage      | Input                                                   | Output                                                        | Completeness rule                                                        |
| ---------- | ------------------------------------------------------- | ------------------------------------------------------------- | ------------------------------------------------------------------------ |
| `discover` | source list YAML; previous release manifest             | expected document set with source modified dates             | full listing per source every run; delta is computed by the pipeline, never trusted from a change feed |
| `fetch`    | expected set                                            | metadata JSON, every listed attachment, hashes               | a document with a missing attachment is a gap, not a silent skip          |
| `extract`  | files                                                   | structure tree with verbatim text                            | article count matches the table of contents; empty text is a gap           |
| `relate`   | VBPL references, text                                   | relations with kind, evidence, affected provisions when captured | never guessed; unknown targets are gaps                                |
| `chunk`, `embed` | trees                                             | chunks, dense and sparse vectors under the release recipes   | vector count equals chunk count; dimensions equal the recipe               |
| `classify` | trees, vectors, taxonomy, seeds                         | assignments                                                   | every article has an assignment or an "unassigned" gap                     |
| `build`    | all gold tables                                         | local `corpus_<cc>_<rel>` database                            | schema template applied; `release` row written                             |
| `eval`     | local database, golden set, taxonomy held-out set       | report; diff against the previous release                     | floors in [testing](../../instructions/testing.md) and [taxonomy](taxonomy.md) |
| `release`  | evaluated database                                      | `corpus_<cc>_<rel>.dump`, `manifest.json`, `manifest.sig` in S3 | refused unless every expected item is done or an accepted gap and eval passed |

Recipe changes (parser version, prompt hash, taxonomy version, model) change
`recipe_hash`, so unchanged documents are reprocessed where it matters.

## External batch submissions

Before calling the Claude Batch API, the activity writes a `submission` row
with an idempotency key and the item list, then submits with that key in
`metadata` and each item's key as `custom_id`. On restart it lists recent
batches by metadata and reconciles: found means poll, not found means
resubmit. Results apply per item after JSON validation; failed items become
`failed` run items with bounded retries, then gaps. Cost per batch is recorded.

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
