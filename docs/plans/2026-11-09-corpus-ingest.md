# Corpus Pipeline Core and VBPL Ingest Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. In this repository the route is: Task 1 to the Codex `designer`, the plan and Task 1 to the Codex `reviewer`, Tasks 2 to 13 each to one Codex `implementer` brief, the primary verifies every task, and the Claude `reviewer` reviews the whole branch after Task 13.

**Goal:** `corpus worker` runs a resumable `BuildRelease` workflow on self-hosted Temporal that discovers, fetches, extracts and relates the Vietnam VBPL scope into the workshop database, with completeness accounting, gaps, cancellation and checkpoints.

**Architecture:** One Go module `apps/corpus` in a root `go.work`. A CLI starts, inspects and cancels runs; the worker hosts one workflow, one batch activity per stage (named after the stage) and four control activities. Each stage is a Go type with `Plan` and `Process`; every processed item publishes an immutable content-addressed artifact directory and commits its rows, gaps and `run_item` state in one fenced transaction. VBPL is read through its public JSON gateway with rate limits and recorded fixtures for tests.

**Tech Stack:** Go 1.27.3, Temporal Go SDK v1.50.0, Temporal server and admin-tools 1.32.2, Temporal UI 2.58.1, PostgreSQL 18 (pgvector image already in compose), pgx/v5, sqlc 1.31.1, goose v3 as a library, golang.org/x/text, gopkg.in/yaml.v3, podman-compose.

**Spec:** `docs/design/pipeline.md`, `docs/design/data.md` (workshop tables), `docs/design/system.md`, `docs/design/budgets.md` ("Pipeline", "Temporal"), ADR 0001, 0003, 0008. VBPL behavior (read only, never copied, ADR 0001): banhmi `docs/design/sources.md` ("vbpl"), `pkg/ingest/vbpl/*.go`, `pkg/pipeline/vbpl_tree.go`, `deploy/seed/{issuer_code,discovery_keyword,relation_type,validity_status}.csv`.

## Scope and the plans after this one

The design's single "Plan 1" is split; each plan ends in working, testable software:

|Plan|Produces|
|-|-|
|this plan|pipeline core; stages `discover`, `fetch`, `extract` (VBPL provision tree), `relate`; run ends `paused`|
|index and release|`chunk`, `embed`, lexical vectors, taxonomy import and seeds, `build`, shared retrieval package, `eval`, signed `release` to S3; recipes from the hybrid study in `docs/research/retrieval-hybrid-2026-11.md`|
|text extraction|documents without a VBPL tree (557 of 2,866 primary documents in banhmi v049, 4 of the 39 golden-cited documents): HTML transcription, PDF, OCR; secondary sources|
|classification|LLM batch classification, submission intent log, taxonomy evaluation gate|

Proofs listed in pipeline.md: crash after an activity commit and laptop restart mid-run are Task 13 here; uncertain batch submission and partial batch results belong to the classification plan; failed activation and rollback belong to the host plan.

## Global Constraints

- Module path `github.com/dannyota/traquydinh/apps/corpus`; root `go.work` with `go 1.27.3` and `use ./apps/corpus`.
- Workflow id `build-<cc>-<rel>`; `rel` matches `^[0-9]{8}(_[0-9]{1,2})?$`; `cc` matches `^[a-z]{2}$`. One `run` row per (country, rel); `generation` increments on every `corpus start`.
- One workflow `BuildRelease`; continue-as-new after every stage; payloads carry ids and counts only, at most 256 KB; a history segment above 5,000 events fails the run.
- Activity batch 100 items; heartbeat after every item; batch start-to-close 1 hour, heartbeat timeout 2 minutes, at most 5 attempts with backoff from 30 seconds to 10 minutes.
- Parser limits: 50 MB per file, 5 minutes per document.
- Artifacts at `$CORPUS_ARTIFACTS/<stage>/<input_hash>-<recipe_hash>/`, immutable, with `MANIFEST.sha256`; the artifact sha256 is the sha256 of that manifest.
- Fencing pair (`run.generation`, Temporal attempt): a commit whose pair is lower than the recorded pair changes nothing.
- Document key `vn:vbpl:<id>` with VBPL `id` verbatim (legacy numeric ItemID or UUID).
- VBPL: base `https://vbpl-bientap-gateway.moj.gov.vn/api/qtdc/public`, headers `Origin: https://vbpl.vn`, `Referer: https://vbpl.vn/`, `User-Agent: traquydinh/<version> (+https://github.com/dannyota/traquydinh)`; at most 2 requests per second and 2 in flight; 30-second timeout; retry 429, 5xx and timeouts 3 times after 2, 8 and 30 seconds; never log a presigned URL query string.
- Integration tests create a fresh database per package through `CORPUS_TEST_ADMIN_URL` (default `postgres://traquydinh@127.0.0.1:5432/postgres`, password from the environment) and drop it afterwards.
- Tests never call live VBPL; they use fixtures in `apps/corpus/internal/vbpl/testdata/`. Integration tests use the `integration` build tag, `-count=1`, the compose PostgreSQL and the Temporal SDK dev server.
- No banhmi code is copied. Files at most 400 lines, one concern each; doc comments on exported symbols; errors wrapped with `%w`.
- Secrets only in `.env` (mode 600): `POSTGRES_PASSWORD`, `PIPELINE_PASSWORD`, `TEMPORAL_DB_PASSWORD`. Never print them.

## Review Focus

- The worker dies after the artifact rename and before the row commit: the retry must adopt the directory by manifest, not write a second one (Task 3 `TestPublish_AdoptsExistingValidDir`, Task 13 crash proof).
- A slow attempt commits after a newer attempt or generation: its rows and state change nothing (Task 4 `TestCommitItem_IgnoresLowerFence`).
- VBPL answers 200 with `success: false`, or with `effStatus`, `docType` or `documentContent` null: the item becomes a gap with the reason, the batch continues (Task 8 `TestDecodeDetail_NullObjects`, Task 10 `TestFetch_DetailNotFoundIsGap`).
- A presigned file URL expires between listing and download: the file is re-listed once and downloaded from the fresh URL (Task 8 `TestDownload_RefreshesExpiredURL`).
- New documents arrive while discover pages a newest-first listing, shifting pages: no document is lost or doubled, and one absence never withdraws a document (Task 9 `TestDiscover_DedupesShiftedPages`, `TestDiscover_RepeatsWhenTotalChanges`, `TestExpectedSet_WithdrawnAfterTwoAbsences`).

---

### Task 1: Design amendments and plan bookkeeping

Route: Codex `designer`, then the Codex `reviewer` reviews this plan with the amendments before Task 2.

**Files:**
- Modify: `docs/adr/0008-write-path-temporal-self-hosted.md` (History), `docs/design/pipeline.md`, `docs/design/data.md` (workshop section), `docs/design/budgets.md` ("Pipeline"), `instructions/go.md`, `docs/plans/README.md`, `docs/plans/backlog.md`
- Delete: `docs/plans/2026-11-09-spikes.md`

- [ ] **Step 1: pipeline.md and ADR 0008.** Add an ADR 0008 History line: each stage's batches run under an activity type named after the stage; planning, stage close, checkpoint and run-state changes are four control activities, which keeps "one activity per stage" for stage work. In pipeline.md add run state `paused`: the workflow stopped after the stage named by `corpus start --until <stage>`, or after the last implemented stage; `corpus start` resumes at the next stage with a new generation. State that the run row is one per release and the snapshot also stores the parsed source list, and that a worker whose compiled recipe hashes differ from the snapshot refuses the run. Define the fetch artifact key: `input_hash` is the sha256 of the fetched bundle manifest, so unchanged remote content keeps its hash and later stages skip it. Add the versioning rule: workflow changes use `workflow.GetVersion`, and CI replays every history in `apps/corpus/internal/workflow/testdata/histories/`. Cancellation: `corpus cancel` sets `run.state = cancelling`; activities check it between items.
- [ ] **Step 2: data.md workshop table.** Add `gap` (run, stage, item key, kind, detail, accepted_at), `gap_acceptance` (run, kind, note, accepted_at), `listing` (run, source query, item key, payload), and name the ingest tables `document`, `fetched_file`, `provision`, `relation`, each keyed by the artifact that produced it.
- [ ] **Step 3: budgets.md "Pipeline".** Add the VBPL limits and the activity timeouts from Global Constraints.
- [ ] **Step 4: instructions/go.md.** Workshop migrations live in `apps/corpus/migrations/` (goose, applied by `corpus migrate`); sqlc generates `apps/corpus/internal/store`.
- [ ] **Step 5: plans.** In `docs/plans/README.md` mark this plan `in progress` and drop the link to the spikes plan; delete the spikes plan file; in `backlog.md` remove the Temporal versioning line (this plan covers it) and add: relation targets outside the discovery scope (banhmi held 1,108 context documents) are gaps here, decide one-hop fetching before the index plan; checkpoints to S3 in addition to the external disk.
- [ ] **Step 6: Commit** `docs: amend the pipeline design for the ingest plan`.

### Task 2: Go workspace, corpus module skeleton and CI

**Files:**
- Create: `go.work`, `apps/corpus/go.mod`, `apps/corpus/cmd/corpus/main.go`, `apps/corpus/internal/cli/cli.go`, `apps/corpus/internal/config/config.go`, `apps/corpus/internal/config/config_test.go`, `apps/corpus/AGENTS.md`, `.golangci.yml`, `.github/workflows/ci.yml`
- Modify: `Makefile` (`test`, `test-integration`, `generate`, `sqlc-check`)

**Interfaces:**
- Produces: `config.Load(getenv func(string) string) (Config, error)` with fields `DatabaseURL` (`CORPUS_DATABASE_URL`, required), `TemporalAddress` (`CORPUS_TEMPORAL_ADDRESS`, default `127.0.0.1:7233`), `Namespace` (`CORPUS_TEMPORAL_NAMESPACE`, default `corpus`), `Artifacts` (`CORPUS_ARTIFACTS`, default `work/artifacts`), `CheckpointDir` (`CORPUS_CHECKPOINT_DIR`, required by `start` and `worker`), `Sources` (`CORPUS_SOURCES`, default `sources`), `TemporalDBURL`, `TemporalVisibilityDBURL`, `PgDump` (default `pg_dump`). `cli.Run(ctx, args []string, stdout, stderr io.Writer) int` dispatching `migrate`, `worker`, `start`, `status`, `cancel`, `gaps`, `accept-gaps`, `export-history`.

- [ ] **Step 1: Write failing tests** `TestLoad_Defaults` (only `CORPUS_DATABASE_URL` set; defaults above) and `TestLoad_MissingDatabaseURL` (error names the variable). Run `cd apps/corpus && go test ./internal/config/`; expected FAIL, package missing.
- [ ] **Step 2: Implement** `config.Load` and the CLI dispatcher; unknown commands print usage and return 2; `main.go` only wires `signal.NotifyContext` and `cli.Run`. Run the tests; expected PASS.
- [ ] **Step 3: Tooling.** `.golangci.yml` enables errcheck, govet with shadow, misspell (US). Makefile: `test` runs `go test ./...` in `apps/corpus`; `test-integration` runs `go test -tags integration -count=1 ./...` with `.env` loaded. `apps/corpus/AGENTS.md` maps packages, owned paths and checks.
- [ ] **Step 4: CI.** `.github/workflows/ci.yml` on pull requests and pushes to `main`: job `static` runs `npm ci` and `make pre-push`; job `test` runs `make test`; job `integration` runs `make test-integration` against a `pgvector/pgvector:pg18` service. Follow aboutme's `.github/workflows/ci.yml` conventions (actions pinned by commit SHA, least-privilege `permissions`).
- [ ] **Step 5: Verify** `make pre-push && make test`; expected `pre-push: ok` and PASS. Commit `feat(corpus): add the Go workspace, corpus CLI skeleton and CI`.

### Task 3: Content-addressed artifact store

**Files:**
- Create: `apps/corpus/internal/artifact/store.go`, `apps/corpus/internal/artifact/manifest.go`, `apps/corpus/internal/artifact/store_test.go`

**Interfaces:**
- Produces: `artifact.Store{Root string}`; `(Store) Publish(ctx, stage, inputHash, recipeHash string, write func(dir string) error) (Result, error)`; `Result{Path, SHA256 string; Adopted bool}`; `Verify(dir string) (sha256 string, err error)`; `(Store) PublishContent(ctx, stage, recipeHash string, write func(dir string) error) (Result, error)` for stages whose input hash is their own content: it writes to the temp dir, computes the manifest sha256 and uses it as `input_hash` in the final path; `Result` then also carries `InputHash`.

- [ ] **Step 1: Write failing tests:** `TestPublish_NewDir`; `TestPublishContent_KeyedByManifestHash` (final path is `<stage>/<manifest sha256>-<recipe>`; identical content adopts); `TestPublish_AdoptsExistingValidDir` (pre-created valid dir: `Adopted=true`, same sha256, temp dir removed); `TestPublish_ReplacesPartialDir` (dir without manifest moves to `.trash/`, new dir published); `TestPublish_ConcurrentSameKey` (two goroutines, both succeed, equal sha256, exactly one `Adopted=false`); `TestVerify_DetectsModifiedFile`.
- [ ] **Step 2: Implement.** Write into `Root/.tmp/<random>`, write `MANIFEST.sha256` as sorted lines `<sha256>  <relative path>`, `os.Rename` into place; on `EEXIST` or a non-empty target, verify and adopt or move the target to `Root/.trash/` and retry the rename once.
- [ ] **Step 3: Verify** `go test ./internal/artifact/ -race`; expected PASS. Commit `feat(corpus): add the content-addressed artifact store`.

### Task 4: Workshop schema, migrations and the fenced commit

**Files:**
- Create: `apps/corpus/migrations/00001_control.sql`, `apps/corpus/migrations/00002_ingest.sql`, `apps/corpus/sqlc.yaml`, `apps/corpus/sql/{run,item,artifact,gap,ingest}.sql`, `apps/corpus/internal/store/` (generated plus `pool.go`), `apps/corpus/internal/migrate/migrate.go`, `apps/corpus/internal/stage/commit.go`, `apps/corpus/internal/stage/commit_integration_test.go`

**Interfaces:**
- Produces: tables `run`, `run_item`, `artifact`, `gap`, `gap_acceptance`, `listing`, `document`, `fetched_file`, `provision`, `relation`; `migrate.Up(ctx, url string) error`; `stage.CommitItem(ctx, pool *pgxpool.Pool, c stage.Commit) (applied bool, err error)` where `Commit` holds `RunID int64, Stage, ItemKey string, Generation, Attempt int, InputHash, RecipeHash string, Artifact artifact.Result, State, Message string, Gaps []Gap, Rows func(ctx context.Context, tx pgx.Tx, artifactID int64) error`.

- [ ] **Step 1: Write failing integration tests:** `TestMigrate_UpFromEmpty` (fresh database, all tables exist); `TestCommitItem_IgnoresLowerFence` (commit gen 2 attempt 1, then gen 1 attempt 5: second returns `applied=false`, rows unchanged); `TestCommitItem_SameArtifactTwiceKeepsOneRow` (two commits of the same (stage, input_hash, recipe_hash) leave one `artifact` row and one set of ingest rows); `TestRunItem_RejectsUnknownState`.
- [ ] **Step 2: Migrations.** `run`: identity id, `country` and `rel` with the Global Constraints checks, unique (country, rel), `generation int not null default 0`, `state` check in (running, cancelling, cancelled, failed, paused, evaluated, released), `until_stage`, `workflow_id`, `snapshot jsonb not null`, `message`, timestamps. `run_item`: primary key (run_id, stage, item_key), `batch int`, `input_hash`, `recipe_hash`, `state` check in (pending, done, gap, accepted_gap, failed), `generation`, `attempt`, `artifact_id`, `message`, `updated_at`. `artifact`: identity id, unique (stage, input_hash, recipe_hash), `item_key`, `path`, `sha256`, `written_at`. `gap`: unique (run_id, stage, item_key, kind, detail), `accepted_at`. Ingest tables reference `artifact(id)`; `provision` unique (artifact_id, path) and (artifact_id, node_id); `relation` keeps `raw_code`, `raw_label`, `method`, `evidence`, nullable `target_key`.
- [ ] **Step 3: Implement** `CommitItem`: one transaction; insert the artifact row on conflict do nothing and select its id; call `Rows` only when the row is new; insert gaps on conflict do nothing; update `run_item` where `(generation, attempt) <= ($gen, $attempt)` and `$gen` equals `run.generation`; zero rows updated rolls back and returns `applied=false`.
- [ ] **Step 4: Verify** `make generate && make test-integration`; expected PASS; `make sqlc-check` clean. Commit `feat(corpus): add the workshop schema and fenced item commit`.

### Task 5: Run lifecycle commands and the frozen snapshot

**Files:**
- Create: `apps/corpus/internal/run/{snapshot,start,status,cancel,gaps}.go` with `_test.go` files, `apps/corpus/internal/run/start_integration_test.go`

**Interfaces:**
- Consumes: `store`, `config.Config`, Temporal `client.Client`.
- Produces: `run.Snapshot{SourceList json.RawMessage; SourceListHash string; Recipes map[string]string; TaxonomyVersion string; WorkerVersion string; PreviousManifest string; Stages []string}`; `run.Start(ctx, deps, cc, rel, until, previousManifest string) (Run, error)`; `run.Status(ctx, deps, cc, rel) (Report, error)` with per-stage expected, done, gap, accepted_gap, failed, pending, and pending activities with attempt and next retry time from Temporal `DescribeWorkflowExecution` (provider job ids and cost arrive with the classification plan); `run.Cancel`; `run.ListGaps`; `run.AcceptGaps(ctx, deps, cc, rel, kind, note)`; `run.ExportHistory(ctx, deps, workflowID, runID, path string) error` writing the history JSON the SDK replayer reads.

- [ ] **Step 1: Write failing tests:** `TestValidateRel` (table: `20261110` ok, `20261110_2` ok, `2026-11-10` rejected); `TestSnapshot_HashStable` (same inputs, same JSON bytes); integration `TestStart_RejectsSecondStartWhileRunning`, `TestStart_PausedRunAppendsNewStageRecipes` (snapshot without `chunk` gains it; a changed `extract` recipe is refused), `TestStart_AfterCancelledBumpsGeneration`, `TestAcceptGaps_MovesGapItems` (items in state gap of that kind become accepted_gap, a `gap_acceptance` row records the note).
- [ ] **Step 2: Implement.** `Start` writes or updates the run row first (snapshot frozen on first start; resuming a paused run appends recipe hashes for stages missing from the snapshot and refuses any changed hash, `generation = generation + 1`, state running), then `ExecuteWorkflow` with id `build-<cc>-<rel>`, `WorkflowIDReusePolicy` allow-duplicate and conflict policy fail; a start error sets state failed with the message. `TaxonomyVersion` is `none` until the classification plan. `WorkerVersion` comes from `debug.ReadBuildInfo` vcs revision. CLI subcommands print aligned tables.
- [ ] **Step 3: Verify** `make test && make test-integration`; expected PASS. Commit `feat(corpus): add run start, status, cancel and gap acceptance`.

### Task 6: `BuildRelease` workflow, activities and stage framework

**Files:**
- Create: `apps/corpus/internal/workflow/{build,activities,stages}.go`, `apps/corpus/internal/workflow/build_test.go`, `apps/corpus/internal/workflow/replay_test.go`, `apps/corpus/internal/stage/stage.go`, `apps/corpus/internal/stage/batch.go`, `apps/corpus/internal/stage/batch_integration_test.go`, `apps/corpus/internal/worker/worker.go`

**Interfaces:**
- Consumes: `artifact.Store` (Task 3), `stage.CommitItem` (Task 4), `run.Snapshot` (Task 5).
- Produces: `stage.Stage` interface `{ Name() string; Recipe() string; Plan(ctx, env Env, r Run) ([]Item, error); Process(ctx, env Env, r Run, it Item) Outcome }`; `stage.Item{Key, InputHash string}`; `stage.Outcome{State, GapKind, Message string; Retryable bool; InputHash string; Artifact artifact.Result; Gaps []Gap; Rows func(context.Context, pgx.Tx, int64) error}`; `stage.Env{DB *pgxpool.Pool; Artifacts artifact.Store; Now func() time.Time}`; stages receive their own dependencies (VBPL client, source list) through constructors registered in `internal/worker/worker.go`, which each stage task extends; workflow input `BuildInput{RunID int64; Country, Rel, Stage, Until string; Generation, HistoryLimit int}`; activities `PlanStage(PlanInput{RunID int64; Stage string; Generation int}) (PlanResult{Batches int})`, one stage activity per stage, registered under the stage name (`discover`, `fetch`, `extract`, `relate`) with `activity.RegisterOptions{Name: <stage>}` around the shared `RunBatch(BatchInput{RunID int64; Stage string; Batch, Generation int}) (BatchResult{Done, Gap, Failed, Pending int; Cancelling bool})`, `CloseStage(StageInput{RunID int64; Stage string; Generation int}) (StageCounts)`, `Checkpoint(StageInput) (CheckpointResult{Dir string})`, `MarkRun(MarkInput{RunID int64; State, Message string})`.

- [ ] **Step 1: Write failing workflow tests** with `testsuite.WorkflowTestSuite` and mocked activities: `TestBuildRelease_ContinuesAsNewWithNextStage`; `TestBuildRelease_StopsAtUntilAndMarksPaused`; `TestBuildRelease_StopsWhenBatchReportsCancelling` (MarkRun cancelled, no further batches); `TestBuildRelease_FailsAboveHistoryLimit` (limit 10); `TestBuildRelease_RefusesRecipeMismatch` (PlanStage returns the mismatch error, run failed).
- [ ] **Step 2: Write failing batch tests** (integration): `TestRunBatch_SkipsDoneItems`; `TestRunBatch_RetryableFailureReturnsErrorUntilLastAttempt` (items stay pending, the activity errors; on attempt 5 they become failed); `TestRunBatch_GapOutcomeRecordsGap`; `TestPlanStage_ResetsFailedItemsOnNewGeneration`.
- [ ] **Step 3: Implement.** `PlanStage` calls `Stage.Plan` and inserts missing `run_item` rows for the stage with `batch = index / 100`, resets failed items to pending when the generation is new, and checks snapshot recipes against compiled recipes. `RunBatch` processes pending items of its batch, heartbeats after each, checks `run.state` before each item, and commits each outcome through `CommitItem` with the activity attempt. Batches run at most 2 at a time. `CloseStage` returns the counts and fails the stage while any item is pending. The workflow checks `GetCurrentHistoryLength` before scheduling each activity. `stages.go` lists `discover`, `fetch`, `extract`, `relate`.
- [ ] **Step 4: Replay test.** `replay_test.go` replays every JSON history in `testdata/histories/` with `worker.NewWorkflowReplayer`; it passes with an empty directory and gains a real history in Task 13.
- [ ] **Step 5: Verify** `make test && make test-integration`; expected PASS. Commit `feat(corpus): add the BuildRelease workflow and stage framework`.

### Task 7: Checkpoint activity

**Files:**
- Create: `apps/corpus/internal/checkpoint/{checkpoint,pgdump}.go`, `apps/corpus/internal/checkpoint/checkpoint_test.go`
- Modify: `apps/corpus/internal/workflow/activities.go` (wire `Checkpoint`)

**Interfaces:**
- Produces: `checkpoint.Take(ctx, cfg Config, runKey string, generation int, stage string) (Result{Dir string}, error)`; a `Dumper` interface so tests avoid real `pg_dump`.

- [ ] **Step 1: Write failing tests:** `TestTake_CopiesOnlyNewArtifacts` (second checkpoint copies nothing already present and verifies manifests); `TestTake_WritesCheckpointJSONLast` (a dumper failure leaves no `checkpoint.json`); `TestTake_KeepsLastThree`; `TestPgDumpVersion_RequiresMajor18`.
- [ ] **Step 2: Implement.** Layout `$CORPUS_CHECKPOINT_DIR/<cc>-<rel>/<generation>-<stage>/` with `workshop.dump`, `temporal.dump`, `temporal_visibility.dump` (`pg_dump -Fc`) and `checkpoint.json` (stage, generation, time, dump sha256s) written last; artifacts copied once into `$CORPUS_CHECKPOINT_DIR/artifacts/` with the same relative paths. Older checkpoint directories beyond three are deleted; artifacts are never deleted.
- [ ] **Step 3: Runbook.** Create `docs/runbooks/corpus-local.md` with "Restore from a checkpoint": stop the worker, `pg_restore` the three dumps, copy artifacts back, `corpus start` (new generation). Link it from `docs/runbooks/README.md`.
- [ ] **Step 4: Verify** `make test`; expected PASS. Commit `feat(corpus): add stage-boundary checkpoints`.

### Task 8: VBPL client and recorded fixtures

**Files:**
- Create: `apps/corpus/internal/vbpl/{client,types,download}.go`, `apps/corpus/internal/vbpl/client_test.go`, `apps/corpus/internal/vbpl/testdata/record.sh`, `apps/corpus/internal/vbpl/testdata/*.json`

**Interfaces:**
- Produces: `vbpl.Client` with `List(ctx, ListQuery{AgencyIDs []string; Keyword string; Page int}) (ListPage, error)`, `Detail(ctx, id) (json.RawMessage, Detail, error)`, `Tree(ctx, id) (json.RawMessage, []Node, error)`, `Diagram(ctx, id) (json.RawMessage, error)`, `Files(ctx, id) ([]File, error)`, `Download(ctx, id string, f File, w io.Writer) (n int64, sha256 string, err error)`; `ErrNotFound` for 404 and `success: false`.

- [ ] **Step 1: Record fixtures** with `record.sh` (curl, the headers above, 1 request per second): listing page 1 for agencies 62 and 908 with page size 5; detail, tree, diagram and files for `67878` (numeric id, tree present: 11 articles, 34 clauses, 17 points), the newest VBHN document from the listing (UUID id), and one document whose tree is empty (find its id with `select a.external_id from silver.document d join silver.document_alias a on a.document_id = d.id and a.source = 'vbpl' where d.index_class = 'primary' and not exists (select 1 from silver.document_section s where s.document_id = d.id and s.ptype is not null) limit 1` on the local `banhmi_vn`). Strip presigned query strings from recorded files lists.
- [ ] **Step 2: Write failing tests** against `httptest` servers: `TestClient_RetriesOn503ThenSucceeds`; `TestClient_LimitsRate` (10 calls take at least 4.5 seconds with a fake clock or measured); `TestDecodeDetail_NullObjects`; `TestDecodeTree_NestedNodes` (fixture 67878 counts); `TestDownload_RefreshesExpiredURL` (first URL 403, re-list once, second URL 200); `TestClient_NeverLogsSignedQuery` (captured slog output has no `X-Amz-`).
- [ ] **Step 3: Implement** with `golang.org/x/time/rate`, a semaphore of 2, and the retry schedule from Global Constraints.
- [ ] **Step 4: Verify** `go test ./internal/vbpl/`; expected PASS. Commit `feat(corpus): add the VBPL gateway client with fixtures`.

### Task 9: Source list and the `discover` stage

**Files:**
- Create: `sources/vn.yaml`, `apps/corpus/internal/source/list.go`, `apps/corpus/internal/source/list_test.go`, `apps/corpus/internal/stage/discover/discover.go`, `apps/corpus/internal/stage/discover/discover_test.go`, `apps/corpus/internal/stage/discover/expected_integration_test.go`

**Interfaces:**
- Produces: `source.List{Country string; VBPL VBPLSources; RelationCodes map[string]RelationCode; StatusCodes map[string]StatusCode}` with `VBPLSources{SweepAgencies []string; KeywordAgencies []string; Keywords []string}`; discover items `vbpl:sweep` and `vbpl:keyword:<term>`; `listing` rows; `discover.ExpectedSet(ctx, db *pgxpool.Pool, r stage.Run) ([]Expected{Key, VBPLID string; Withdrawn bool}, error)`, which `fetch.Plan` calls. Stages read the source list from `run.Snapshot.SourceList`, never from the working tree.

- [ ] **Step 1: Write `sources/vn.yaml`** from banhmi seeds (data reuse, ADR 0001): sweep agencies `62`, `908`; keyword agencies = `issuer_code.csv` rows with source `vbpl`, `in_scope` true, `is_sbv` false; keywords = `discovery_keyword.csv` rows with source `vbpl`; relation codes from `relation_type.csv` (code, label, is_amending, is_superseding); status codes from the empty-source rows of `validity_status.csv`.
- [ ] **Step 2: Write failing tests:** `TestLoadVN` (2 sweep agencies, 8 keyword agencies, 26 keywords, 11 relation codes, 8 status codes); `TestDiscover_PagesUntilShortPage`; `TestDiscover_DedupesShiftedPages` (a document repeated across pages appears once); `TestDiscover_RepeatsWhenTotalChanges` (page 1 re-read at the end reports a new total: one more pass, then stop); integration `TestExpectedSet_WithdrawnAfterTwoAbsences` (present in run A, absent in B and C: in C its fetch item is a gap `withdrawn`; absent only in C: still fetched).
- [ ] **Step 3: Implement.** 50 rows per page, `sortBy=issueDate`, `sortDirection=desc`, `groupVbpl=false`, `optionDoc=title`, `matchMode=all_words`; the artifact holds the raw pages; `listing` stores one row per item per query. Expected set = union of listed ids plus ids present in the previous run's expected set; withdrawn per the rule above, comparing the two previous runs of the same country in rel order.
- [ ] **Step 4: Verify** `make test && make test-integration`; expected PASS. Commit `feat(corpus): add the VN source list and discover stage`.

### Task 10: The `fetch` stage

**Files:**
- Create: `apps/corpus/internal/stage/fetch/{fetch,bundle}.go`, `apps/corpus/internal/stage/fetch/fetch_test.go`

**Interfaces:**
- Consumes: `vbpl.Client` (Task 8), expected set items (Task 9).
- Produces: fetch artifact with `detail.json`, `tree.json`, `diagram.json`, `files.json` (no query strings), `files/<name>`; `document` and `fetched_file` rows; the artifact sha256 is the extract input hash.

- [ ] **Step 1: Write failing tests** with a fixture-backed `httptest` server: `TestFetch_WritesBundleAndRows` (document number, type, issuer, dates, status code, `is_consolidated`); `TestFetch_SameContentSameHash` (second fetch adopts); `TestFetch_ChangedFileChangesHash`; `TestFetch_MissingAttachmentIsGap` (item done, gap `missing_attachment`); `TestFetch_FileOverLimitIsGap` (gap `file_too_large`, nothing written for that file); `TestFetch_DetailNotFoundIsGap` (item gap `not_found`).
- [ ] **Step 2: Implement.** `fetch.Plan` returns `discover.ExpectedSet`; download into the publish temp dir; `input_hash` = `artifact.PublishContent`, so the input hash is the bundle manifest sha256; withdrawn items become gap `withdrawn` without a request. `extract.Plan` and `relate.Plan` return this run's fetch items in state done with the fetch artifact sha256 as input hash.
- [ ] **Step 3: Verify** `go test ./internal/stage/fetch/`; expected PASS. Commit `feat(corpus): add the fetch stage`.

### Task 11: The `extract` stage from the VBPL provision tree

**Files:**
- Create: `apps/corpus/internal/stage/extract/{extract,tree,text,labels}.go` with `_test.go` files

**Interfaces:**
- Consumes: fetch artifact `tree.json`.
- Produces: `provision` rows `{node_id, parent_node_id, kind, label, heading, ordinal, path, text}`; recipe `tree/1`.

- [ ] **Step 1: Write failing tests:** `TestExtract_Fixture67878` (11 `dieu`, 34 `khoan`, 17 `diem`; first article label `Điều 1`, heading `Phạm vi điều chỉnh và đối tượng áp dụng`, path `dieu-1`; clause path `dieu-2/khoan-1`); `TestText_ConvertsBreaksAndEntities` (`<br>` to newline, tags dropped, entities unescaped); `TestText_NormalizesToNFC` (decomposed input, same tone placement); `TestExtract_EmptyTreeIsGap` (gap `no_tree`); `TestExtract_DuplicatePathsDisambiguated` (second `dieu-5` becomes `dieu-5~2`); `TestExtract_ArticleSequenceGap` (articles 1, 2, 4: gap `article_sequence` with detail `3`); `TestExtract_UnknownKindIsOther` (gap `unknown_node_kind`); `TestExtract_EmptyArticleTextIsGap` (an article with no text and no children: gap `empty_text` with its path).
- [ ] **Step 2: Implement.** Kind from `level` (Part `phan`, Chapter `chuong`, Section `muc`, Article `dieu`, Clause `khoan`, Point `diem`), else `ptype` (1 `phan`, 2 `chuong`, 3 or 4 `muc`, 5 `dieu`, 6 `khoan`, 7 `diem`), else `other`. Paths: `phan-<n>`, `chuong-<roman, lowercase>`, `chuong-<roman>/muc-<n>`, `dieu-<n>` (articles number through the whole document, so no chapter prefix), `dieu-<n>/khoan-<m>`, `dieu-<n>/khoan-<m>/diem-<letter>`; a repeated path gets `~2`, `~3` in node order. Table and appendix counts wait for the text extraction plan. Text is each node's content as served; an article keeps its full text and its clauses keep theirs. 5-minute context per document.
- [ ] **Step 3: Verify** `go test ./internal/stage/extract/`; expected PASS. Commit `feat(corpus): add the extract stage for VBPL trees`.

### Task 12: The `relate` stage

**Files:**
- Create: `apps/corpus/internal/stage/relate/{relate,codes}.go`, `apps/corpus/internal/stage/relate/relate_test.go`

**Interfaces:**
- Consumes: fetch artifact `detail.json` (`references`) and `diagram.json` (`documentNamesByType`); `source.List.RelationCodes`; the run's expected set.
- Produces: `relation` rows `{kind, raw_code, raw_label, target_vbpl_id, target_key, target_number, evidence, method, source_node_id, target_node_id}`; recipe `vbpl-relations/1`.

- [ ] **Step 1: Write failing tests:** `TestRelate_MapsCodes` (code `10` to `amends_supplements`, raw code kept); `TestRelate_MergesReferencesAndDiagram` (same target and kind once, methods `vbpl_references,vbpl_diagram`); `TestRelate_UnknownTargetIsGap` (target outside the expected set: `target_key` null, gap `unknown_target` with the VBPL id and number); `TestRelate_UnmappedCodeIsGap` (kind `unknown`, gap `unknown_relation_code`).
- [ ] **Step 2: Implement.** Evidence is the verbatim label or snippet from the payload; provision ids only when the payload carries them; nothing is inferred from text.
- [ ] **Step 3: Verify** `go test ./internal/stage/relate/`; expected PASS. Commit `feat(corpus): add the relate stage`.

### Task 13: Local services, proofs and the first live run

**Files:**
- Create: `deploy/local/db-setup.sh`, `deploy/local/temporal-schema.sh`, `apps/corpus/internal/fault/fault.go` (build tag `faultinject`), `apps/corpus/test/proof/{crash_after_commit,restart_midrun}_test.go`, `apps/corpus/internal/workflow/testdata/histories/<first-run>.json`, `docs/research/vbpl-ingest-2026-11.md`
- Modify: `deploy/compose.yml`, `.env.example`, `Makefile` (`db-setup`, `temporal-setup`, `temporal-up`, `temporal-down`), `docs/runbooks/corpus-local.md`, `docs/architecture.md`, `docs/research/README.md`, `docs/plans/README.md`

**Interfaces:**
- Consumes: everything above.
- Produces: services on `127.0.0.1:7233` (frontend) and `127.0.0.1:8233` (UI); databases `workshop` (role `pipeline`), `temporal` and `temporal_visibility` (role `temporal`); namespace `corpus` with 30-day retention.

- [ ] **Step 1: Local services.** Add `temporal` (`docker.io/temporalio/server:1.32.2`, env `DB=postgres12`, `POSTGRES_SEEDS=postgres`, `DBNAME=temporal`, `VISIBILITY_DBNAME=temporal_visibility`, user `temporal`) and `temporal-ui` (`docker.io/temporalio/ui:2.58.1`) to compose. `db-setup.sh` creates roles and databases idempotently through `podman exec` with passwords from `.env`; `temporal-schema.sh` runs `temporalio/admin-tools:1.32.2` once with `temporal-sql-tool` (create, setup-schema, update-schema for both databases) and creates the namespace. Verify each image tag with `podman manifest inspect` first and record any substitute in the runbook. Expected: `make db-setup temporal-setup temporal-up` exits 0 and the UI lists namespace `corpus`.
- [ ] **Step 2: Write the proof tests** (integration, `faultinject` tag, SDK dev server, fixture VBPL server, 30 fake documents): `TestProof_CrashAfterCommit` (the fault hook exits the worker process after the rename of item 7 and before its commit; a new worker finishes; artifact directories equal distinct items, no duplicate rows, every item done once); `TestProof_RestartMidRun` (the worker is stopped during the fetch batch and restarted 5 seconds later; the run reaches `paused` after `relate` with identical counts to an uninterrupted run).
- [ ] **Step 3: Verify** `go test -tags 'integration faultinject' -count=1 ./test/proof/`; expected PASS.
- [ ] **Step 4: First live run.** With services up: `corpus migrate`, `corpus worker` in the background, `corpus start vn <run date as YYYYMMDD> --until relate`. Record in `docs/research/vbpl-ingest-2026-11.md`: listing totals per query, expected set, done and gaps per stage and kind, duration, checkpoint sizes, and a comparison with banhmi v049 (primary documents, documents with a tree, relations). Export the workflow history of one stage with `corpus export-history` into `testdata/histories/` and run the replay test.
- [ ] **Step 5: Docs.** `docs/architecture.md` describes `apps/corpus` as built; the runbook covers services, run, status, cancel, gaps, accept-gaps and checkpoints; `docs/plans/README.md` marks this plan landed.
- [ ] **Step 6: Verify and commit** `make pre-push && make test && make test-integration`; expected PASS. Commit `feat(corpus): prove crash and restart recovery and record the first VBPL run`.
