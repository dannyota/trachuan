# Corpus pipeline

The write path builds a corpus release on the owner's MacBook and publishes a
signed dump to OCI Object Storage. Decision and alternatives:
[ADR 0008](../adr/0008-write-path-temporal-self-hosted.md).

## Shape

- Native PostgreSQL 18 (the workshop) and the Temporal development server with
  SQLite persistence run on the MacBook (`deploy/local`, `make services-up`).
  The machine may be off between runs; Temporal resumes from persisted history.
- `corpus worker` is one Go binary with one workflow,
  `BuildRelease(country, rel)`. Stage and control activities follow
  [ADR 0008](../adr/0008-write-path-temporal-self-hosted.md#history). Workflow
  ids are `build-<cc>-<rel>`. The workflow calls continue-as-new after each
  stage, so each history segment holds one stage.
- The `run` row, one per (`country`, `rel`), is the record of a run; Temporal
  only coordinates and can be rebuilt empty. First start freezes the parsed
  source list and its hash, `taxonomy_version`, worker version and previous
  release manifest hash. Each stage's recipe freezes when the run first includes
  that stage. `--until relate` freezes only discover, fetch, extract and relate,
  even when the worker already supports later stages. Start and restart extend
  the snapshot under the [run lifecycle](#run-lifecycle). Frozen recipes never
  change. Activities read the snapshot from the run row, never the working tree.
  A worker whose compiled recipe hash differs for an included stage refuses the
  run; recipes for stages not yet included cannot block it.
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
  Temporal reports an open workflow for the id, and when the state is `starting`
  and younger than the stale limit in [budgets](budgets.md). Otherwise it
  increments `generation`, sets `starting` and commits, then starts the workflow
  with that generation and a fail-on-conflict id policy. The workflow's first
  activity moves `starting` to `running` only for its own generation. A crash
  before dispatch leaves a stale `starting` row that the next start replaces; a
  lost start response leaves a workflow that marks the row `running`. If an open
  workflow of an older generation holds the id, start refuses and names
  `corpus cancel` or `corpus recover`.
- A start with a later or no `--until` appends missing stages and freezes their
  recipes at that moment, in one update recorded in the run message. A restart
  from `cancelled` or `failed` extends exactly like one from `paused`. Stages
  already in the snapshot keep their frozen recipes. A shorter `--until` only
  stops earlier; it never removes included stages or recipes.
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
  `failed` items to `pending`. An empty `next_stage` means the run is complete
  until new stages are appended. Errors in one item stay with that item: errors
  in the source data (broken payload JSON, text PostgreSQL rejects, duplicate
  ids) become gap `invalid_source_data`, which the owner may accept, and other
  item errors fail the item; only the activity's own context or an
  infrastructure fault ends the batch. `paused` means the workflow stopped after
  the stage named by `--until` or after the last implemented stage.
- `corpus cancel <cc> <rel>` sets `cancelling`. Stage activities check the state
  before each item and return early; the workflow waits for every batch of the
  stage to return, then sets `cancelled`. A stage that has not started its
  batches is cancelled before planning.
- `corpus recover <cc> <rel>` handles a restored workshop or a lost worker: it
  increments `generation`, sets `failed` with a message, and terminates an open
  workflow for the id only with `--terminate`. Recovery changes neither stages
  nor recipes. Then `corpus start` resumes under the extension rules above.
- `corpus status <cc> <rel>` and the Temporal UI show per-stage expected, done,
  gap, failed and pending counts, unaccepted gaps by kind, next retry times,
  provider job ids and cost so far.

### Source execution (proposed)

[Source execution](source-execution.md) defines shared membership readers,
evidence-bearing lookup results, fetch-close finalization, retry invalidation
and durable source pauses. These are prerequisites for source adapters. The
existing `CloseStage` invokes a stage finalizer inside its fenced transaction;
fetch cannot advance until final membership is committed. `scope_changed` blocks
start and release permanently for that run; `source_paused` requires the owner's
explicit source-resume command. Neither is an acceptable coverage gap.

## Items, artifacts and gaps

- A stage plans its items, processes each into an artifact, and resolves each
  artifact against the run. Processing is a function of the item input and the
  recipe; it writes the artifact. The artifact's rows (`document`,
  `fetched_file`, `provision`, `relation`, `chunk`) are loaded from the artifact
  files. Fetch times are separate
  [observation evidence](fetch-bundles.md#observation-commit-and-recovery), so
  adoption reproduces content rows without inventing a download time. Resolving
  is a function of the artifact and the run; it writes run rows (`listing`) and
  gaps. When planning already knows an item's input hash and the artifact
  exists, processing is skipped and only resolving runs, so a reused artifact
  still yields this run's gaps.
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
  download. [Byte stability](fetch-bundles.md#byte-stability) excludes
  observation times and run state, so unchanged source bytes under the same
  recipe keep their hash. The fetch artifact sha256 is the input hash of
  `extract` and `relate`.
- One transaction commits an item: the artifact row when there is one, the
  artifact's rows when that row is new, the run rows and gaps (idempotent), and
  the `run_item` update. The update applies only when the pair
  (`run.generation`, Temporal attempt) is not lower than the recorded pair; a
  stale commit rolls back. For an equal pair on a terminal item (`done`, `gap`,
  `failed`), compare the incoming result before any writes or pause override. An
  identical result (the item's state, message and artifact hash or null, plus
  its resource evidence; gaps and run rows are idempotent and not compared) is a
  no-op; any difference rolls back the whole commit, leaving the item, derived
  rows, gaps and observations unchanged. Compare resource evidence by resource
  key, including every stored observation field. A later source pause cannot
  replace that terminal result. Only a new committed result may write
  observations; a new failed `source_paused` result writes none. This tightens
  the shared commit fence and requires stage commit package tests. Items ending
  as a gap or failure without output commit without an artifact.
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

| Stage      | Input                                       | Output                                                                          | Completeness rule                                                                                                                                                           |
| ---------- | ------------------------------------------- | ------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `discover` | source list YAML; previous release manifest | expected document set with source modified dates                                | full listing per source every run; reconcile independent supports and distinguish selection loss from catalog absence under [source ingest rules](#source-ingest-rules)     |
| `expand`   | Frozen direct VBPL seeds                    | One-hop context membership and source evidence                                  | Proposed scope stage; never expand context output; failures keep coverage gaps, [sources](sources.md#one-hop-relation-context)                                              |
| `fetch`    | expected set                                | metadata JSON, every listed attachment, hashes                                  | attachments use a verified matching local cache entry or a fresh download; a changed attachment reprocesses the document; a missing attachment is a gap                     |
| `extract`  | files                                       | structure tree with verbatim text                                               | parser limits in [budgets](budgets.md); article count matches the table of contents; table and appendix counts match the source inventory; empty or truncated text is a gap |
| `relate`   | VBPL references, text                       | relations with kind, evidence, affected provisions when captured                | never guessed; unknown targets are gaps                                                                                                                                     |
| `chunk`    | Done fetch and extract artifacts            | Chunks and source-span/omission report                                          | [Chunk contract](chunk.md); metadata changes invalidate prefixes                                                                                                            |
| `embed`    | Verified chunk artifacts                    | Binary dense vectors, ordinal/token index and gaps                              | [Embed contract](embedder.md#embed-stage); each ordinal is embedded or explicitly rejected                                                                                  |
| `classify` | trees, vectors, taxonomy, seeds             | assignments                                                                     | every article has an assignment or an "unassigned" gap                                                                                                                      |
| `build`    | Frozen upstream artifact inventory          | Local corpus database and verified custom dump                                  | [Build contract](index.md); shared schema, dense/BM25 counts, empty taxonomy, provisional threshold                                                                         |
| `eval`     | Verified build, fixtures and identity map   | Reports, exact/ANN arm caches and previous diff                                 | [Eval contract](evaluation.md); floor failure is reported, incomplete scoring fails the stage                                                                               |
| `release`  | evaluated database                          | `corpus_<cc>_<rel>.dump`, `manifest.json`, `manifest.sig` in OCI Object Storage | refused unless every expected item is done or a gap, every gap is accepted, and eval passed                                                                                 |

The index baseline appends `chunk`, `embed`, `build`, `eval` to the existing
registry and frozen run snapshot. `classify` and `release` remain future stages;
the baseline does not register placeholders for them. Build computes sparse
vectors because BM25 needs corpus-wide statistics; there is no lexical stage.
After a completed eval the run is `evaluated`, even when measured quality fails;
status also shows `quality_pass` and `release_eligible`, never just "passed".
Existing terminal/generation rules still fence every stage commit.

Build artifacts contain the exact dump that later release work may upload; eval
emits a sidecar bound to its hash. Signing, manifest production and host
verification are designed together in the host work. Nothing in this baseline
sets `release_eligible` true. Taxonomy and filtered evaluation stay out of
scope.

## Source ingest rules

[Sources](sources.md) owns local selection over the full central VBPL listing,
named lookups, secondary searches and precedence, relation context and query
identity. Apply the stable-listing rule below to complete passes of that single
central listing; local agency and term rules do not create separate network
queries. The new scope run inserts `expand` before `fetch`; the existing ingest
run keeps its frozen stage order. `discover` supplies direct seeds and secondary
candidates; expand closes additions to the expected set before fetch planning.

- `discover` lists every query until two consecutive passes return the same id
  set, at most the pass limit in [budgets](budgets.md); otherwise the query item
  gets gap `listing_unstable` and the expected set uses the union of the passes.
  A listing is complete when its query item is done without a `listing_unstable`
  gap; a query's identity hashes its complete source-specific definition.
- Reconcile supports under [membership](membership.md): supersession, per-tuple
  retention, the two-listing withdrawal exception, relation loss and removal
  lifetime have one home there.
- VBPL English renditions (ids starting with `vbpqta_`) are not legal text in
  Vietnamese and never enter the expected set.
- `fetch` stores attachments as `files/<sha256>` and maps original names in
  `files.json`; each download attempt writes a fresh temporary file and is
  bounded by the per-file limit while streaming. API responses above the
  response limit are gaps. A gateway error (HTTP 5xx) that persists through the
  client retries makes the item gap `source_unavailable` in that attempt,
  without a batch retry (budgets); the owner may accept it, and the next run
  asks again; start retries these under
  [source execution](source-execution.md#retry-source-gaps). Known source
  failures never stall a batch or the stage close. An attachment whose document
  id, name and size match a local cache entry collected no earlier than the
  gateway's `updatedAt` for that file is taken from the cache once its sha256
  verifies; otherwise it is downloaded.
- `extract` refuses trees deeper or larger than the limits in
  [budgets](budgets.md) with gap `tree_too_large`.
- `relate` maps relation codes with the source list's table, whose hash is part
  of the `relate` recipe hash. Resolving a target against the run's expected
  set, and the `unknown_target` gap, happen when the artifact is resolved for a
  run, never inside the cached artifact. Membership alone does not prove a
  resolved corpus endpoint: a fetch gap or removed document remains unresolved
  in build. Secondary bundles have no invented VBPL references.

### Scope recipes and snapshots (proposed)

Freeze parsed source definitions, the migration comparison and later loss-review
references on first start. Source-list changes alter `source_list_hash`; never
read live YAML on resume. Use a new release id for scope adoption. An old run
cannot replace recipes or insert `expand` through the append-only extension
rule.

| Recipe                   | Scope change and reason                                                                                                                  |
| ------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------- |
| `discover`               | Bump for listing, local selection, qualified probes and secondary parsing; hash rules, queries, adapter and normalizer versions.         |
| `expand`                 | New recipe hashes relation-code table, qualifying kinds, direction, one-hop policy and parser version so scope evidence is reproducible. |
| `fetch`                  | Bump for source dispatch, secondary bundle/provenance format and gateway failure classification.                                         |
| `extract`                | Bump for explicit secondary `no_tree` handling; no PDF extraction is introduced.                                                         |
| `relate`                 | Bump for secondary empty relations; retain relation-code hash.                                                                           |
| `build`                  | Bump for source provenance, normalized numbers, membership exclusions and hand-over gaps.                                                |
| `chunk`, `embed`, `eval` | Keep algorithms; changed inputs and reviewed evaluation identity-map hashes invalidate affected artifacts.                               |

Expansion artifacts bind observed edges; inventories follow [index](index.md).
Fetch format adoption and its one-time cost follow
[fetch bundles](fetch-bundles.md#adoption-and-cost). Reuse artifacts only when
input and recipe match, and resolve gaps for this run. No old artifact, number
mapping or evaluation result establishes coverage for a new run.

## External batch submissions

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

## Release artifact

Storage endpoint, upload credentials and access boundaries are defined in
[deployment](deployment.md#object-storage).

The future `manifest.json` binds the versioned release metadata in
[data](data.md), dump `sha256` and size, evaluation report hash, built-at and
worker version. ADR 0010's chunk and retrieval fields must reach the producer
and host verifier together; the index baseline defines no partial manifest.
`manifest.sig` is an Ed25519 signature (minisign format) made with a key on the
MacBook; the host holds only the public key. The dump is produced with
`--no-owner --no-privileges`.

## Proofs before unattended releases

Crash after an activity commit, uncertain batch submission, laptop restart
mid-run, partial batch results, failed activation, and rollback each have a
scripted test under `apps/corpus` and a line in the first plan.
