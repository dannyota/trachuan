# Corpus Pipeline Core and VBPL Ingest Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Route in this repository: Tasks 2 to 13 each go to one Codex `implementer` brief; the primary verifies every task and owns Git; the Claude `reviewer` reviews the whole branch after Task 13.

**Goal:** `corpus worker` runs a resumable `BuildRelease` workflow on self-hosted Temporal that discovers, fetches, extracts and relates the Vietnam VBPL scope into the workshop database, with completeness accounting, gaps, cancellation, checkpoints and recovery.

**Architecture:** One root Go module `danny.vn/trachuan`; the pipeline lives in `apps/corpus`. The `run` row is the record of a run; Temporal coordinates. Each stage plans items, processes each into an immutable content-addressed artifact (pure function of input and recipe), and resolves the artifact against the run (run rows and gaps); one fenced transaction commits each item. VBPL is read through its public JSON gateway with rate limits; tests use recorded fixtures.

**Tech Stack:** Go 1.27.3, Temporal Go SDK v1.50.0, Temporal server and admin-tools 1.32.2, Temporal UI 2.58.1, PostgreSQL 18 (pgvector image in compose), pgx/v5, sqlc 1.31.1, goose v3 as a library, golang.org/x/text, golang.org/x/time/rate, gopkg.in/yaml.v3, podman-compose.

**Spec:** `docs/design/pipeline.md` ("Shape", "Run lifecycle", "Items, artifacts and gaps", "VBPL ingest rules"), `docs/design/data.md` (workshop tables), `docs/design/budgets.md` ("Pipeline", "Temporal"), ADR 0001, 0003, 0008. VBPL behavior (read only, never copied, ADR 0001): banhmi `docs/design/sources.md` ("vbpl"), `pkg/ingest/vbpl/*.go`, `pkg/pipeline/vbpl_tree.go`, `deploy/seed/{issuer_code,discovery_keyword,relation_type,validity_status}.csv`.

## Scope and the plans after this one

|Plan|Produces|
|-|-|
|this plan|pipeline core; stages `discover`, `fetch`, `extract` (VBPL provision tree), `relate`; a run ends `paused`|
|index and release|`chunk`, `embed`, lexical vectors, taxonomy import and seeds, `build`, shared retrieval package, `eval`, signed `release`; recipes from experiments under ADR 0010|
|text extraction|documents without a VBPL tree (557 of 2,866 primary documents in banhmi v049, 4 of the 39 golden-cited documents): HTML transcription, PDF, OCR; secondary sources|
|classification|LLM batch classification, submission intent log, taxonomy evaluation gate|

Proofs listed in pipeline.md: crash after an activity commit (and after rename, before commit), laptop restart mid-run and checkpoint recovery are Task 13 here; uncertain batch submission and partial batch results belong to the classification plan; failed activation and rollback to the host plan.

## Global Constraints

- Module path `danny.vn/trachuan` (one `go.mod` at the repository root, `go 1.27.3`, like the owner's other projects); packages under `danny.vn/trachuan/apps/corpus/...`. Any task may add dependencies to the root `go.mod` and `go.sum`.
- Workflow id `build-<cc>-<rel>`; `cc` matches `^[a-z]{2}$`; `rel` matches `^[0-9]{8}(_[0-9]{1,2})?$` and orders by date, then sequence number as an integer.
- Run states and transitions, the start protocol, `next_stage`, cancellation drain and `corpus recover` exactly as pipeline.md "Run lifecycle"; commit and gap rules exactly as "Items, artifacts and gaps".
- One workflow `BuildRelease`; continue-as-new after every stage; payloads carry ids and counts only, at most 256 KB; a history segment above 5,000 events fails the run.
- Batch 100 items, heartbeat every 20 seconds while a batch runs, start-to-close 1 hour, heartbeat timeout 2 minutes, at most 5 attempts, backoff 30 seconds to 10 minutes; at most 2 batches of a stage at once. A `starting` row is stale after 2 minutes.
- Limits: 50 MB per file, 5 minutes per document, API response 20 MB, tree depth 32 and 50,000 nodes, 3 listing passes per query.
- VBPL: base `https://vbpl-bientap-gateway.moj.gov.vn/api/qtdc/public`, headers `Origin: https://vbpl.vn`, `Referer: https://vbpl.vn/`, `User-Agent: trachuan/<version> (+https://github.com/dannyota/trachuan)`; at most 2 requests per second and 2 in flight; 30-second timeout; retry 429, 5xx and timeouts 3 times after 2, 8 and 30 seconds; never log a presigned URL query string. Document key `vn:vbpl:<id>` with the VBPL `id` verbatim.
- Recipe hash: `stage.RecipeHash(stage, version, extra...)` = hex sha256 of the arguments joined by a NUL byte.
- Tests never call live VBPL; fixtures live in `apps/corpus/internal/vbpl/testdata/`. Integration tests use the `integration` tag, `-count=1`, a fresh database per package created through `CORPUS_TEST_ADMIN_URL` and dropped afterwards, and the Temporal SDK dev server. Proof tests add the `faultinject` tag.
- No banhmi code is copied. Files at most 400 lines, one concern each; doc comments on exported symbols; errors wrapped with `%w`.
- Secrets only in `.env` (mode 600): `POSTGRES_PASSWORD`, `PIPELINE_PASSWORD`, `TEMPORAL_DB_PASSWORD`. Never print them. CI uses a throwaway `ci` password set in the workflow file, not a secret.

## Review Focus

- An older workflow generation keeps running after `corpus start` or `corpus recover`: every activity must return `superseded` and change nothing (Task 6 `TestActivities_SupersededGenerationChangesNothing`).
- The worker dies after rename and before commit, or two publishers repair the same partial directory: one valid directory, one committed row (Task 3 `TestPublish_ConcurrentPartialRepair`, Task 13 crash proof).
- Pages shift while a newest-first listing is read, with equal totals: no document is lost and instability becomes a gap, not a silent pass (Task 9 `TestDiscover_EqualTotalReplacementTriggersThirdPass`).
- Hostile attachment names and a timeout after a partial body: nothing escapes the artifact directory and no corrupted bytes get a valid hash (Task 8 `TestDownload_TimeoutAfterPartialBodyRetriesFresh`, Task 10 `TestFetch_HostileFileNames`).
- A restored workshop with a stale `running` row: recovery bumps the generation and resumes without duplicate work (Task 13 `TestProof_RecoverFromCheckpoint`).

---

### Task 1: Design amendments and plan bookkeeping (done)

- [x] Designer amendments in commit `8f7b1c0`: ADR 0008 History, `pipeline.md`, `data.md`, `budgets.md`, `instructions/go.md`, plans README, backlog, spikes plan deleted.
- [x] Primary revision after the plan review (findings S-01 to S-19): pipeline.md "Run lifecycle", "Items, artifacts and gaps", "VBPL ingest rules"; data.md run and run_item columns and the publication rule; budgets.md heartbeat, stale start and ingest bounds.

### Task 2: Go workspace, corpus module skeleton and CI

**Files:**
- Create: `go.work`, `apps/corpus/go.mod`, `apps/corpus/cmd/corpus/main.go`, `apps/corpus/internal/cli/cli.go`, `apps/corpus/internal/config/config.go`, `apps/corpus/internal/config/config_test.go`, `apps/corpus/AGENTS.md`, `.golangci.yml`, `.github/workflows/ci.yml`
- Modify: `Makefile`

**Interfaces:**
- Produces: `config.Load(getenv func(string) string) (Config, error)` with `DatabaseURL` (`CORPUS_DATABASE_URL`, required), `TemporalAddress` (`CORPUS_TEMPORAL_ADDRESS`, default `127.0.0.1:7233`), `Namespace` (`CORPUS_TEMPORAL_NAMESPACE`, default `corpus`), `Artifacts` (`CORPUS_ARTIFACTS`, default `work/artifacts`), `CheckpointDir` (`CORPUS_CHECKPOINT_DIR`, required by `start` and `worker`), `PgDump` (`CORPUS_PG_DUMP`, default `pg_dump`). `cli.Run(ctx, args []string, stdout, stderr io.Writer) int` with a command table that later tasks extend: `migrate`, `worker`, `start`, `status`, `cancel`, `recover`, `gaps`, `accept-gaps`, `export-history`.

- [ ] **Step 1: Write failing tests** `TestLoad_Defaults` (only `CORPUS_DATABASE_URL` set; defaults above) and `TestLoad_MissingDatabaseURL` (error names the variable). Run `cd apps/corpus && go test ./internal/config/`; expected FAIL, package missing.
- [ ] **Step 2: Implement** `config.Load` and the dispatcher; unregistered commands print usage and return 2; `main.go` only wires `signal.NotifyContext` and `cli.Run`. Expected PASS.
- [ ] **Step 3: Makefile.** `test` runs `go test ./...` in `apps/corpus`; `test-integration` adds `-tags integration -count=1`; `test-proof` uses `-tags 'integration faultinject' -count=1 ./test/proof/...`; `generate` and `sqlc-check` as in aboutme. Targets load `.env` only when it exists. `.golangci.yml` enables errcheck, govet with shadow, misspell (US). `apps/corpus/AGENTS.md` maps packages, owned paths and checks.
- [ ] **Step 4: CI** `.github/workflows/ci.yml` on pull requests and pushes to `main`, following aboutme's conventions (actions pinned by SHA, least-privilege `permissions`): `static` runs `npm ci` and `make pre-push`; `test` runs `make test`; `integration` and `proof` run `make test-integration` and `make test-proof` with a `pgvector/pgvector:pg18` service (`POSTGRES_PASSWORD: ci`), `CORPUS_TEST_ADMIN_URL=postgres://postgres:ci@localhost:5432/postgres`, and `postgresql-client-18` installed from the PGDG apt repository for `pg_dump`.
- [ ] **Step 5: Verify** `make pre-push && make test`; expected `pre-push: ok` and PASS. Commit `feat(corpus): add the Go workspace, corpus CLI skeleton and CI`.

### Task 3: Content-addressed artifact store

**Files:**
- Create: `apps/corpus/internal/artifact/{store,manifest,lock}.go`, `apps/corpus/internal/artifact/store_test.go`

**Interfaces:**
- Produces: `artifact.Store{Root string}`; `Result{Path, SHA256, InputHash string; Adopted bool}`; `(Store) Publish(ctx, stage, inputHash, recipeHash string, write func(dir string) error) (Result, error)`; `(Store) PublishContent(ctx, stage, recipeHash string, write func(dir string) error) (Result, error)` whose input hash is the manifest sha256; `(Store) Open(stage, inputHash, recipeHash string) (Result, bool, error)` for reuse; `Verify(dir string) (string, error)`.

- [ ] **Step 1: Write failing tests:** `TestPublish_NewDir`; `TestPublishContent_KeyedByManifestHash` (path `<stage>/<manifest sha256>-<recipe>`; identical content adopts); `TestPublish_AdoptsExistingValidDir`; `TestPublish_ReplacesPartialDir` (no manifest: moved to `.trash/`, replaced); `TestPublish_ConcurrentSameKey` (two goroutines, equal sha256, exactly one `Adopted=false`); `TestPublish_ConcurrentPartialRepair` (two publishers over one partial directory: one valid directory remains, both results equal); `TestVerify_DetectsModifiedFile`; `TestVerify_RejectsUnsafePaths` (`..`, absolute path, symlink).
- [ ] **Step 2: Implement.** Temp dir under `Root/.tmp/`, manifest as sorted `<sha256>  <relative path>` lines, an exclusive `flock` on `Root/.locks/<stage>-<input>-<recipe>.lock` around inspect, repair and rename.
- [ ] **Step 3: Verify** `go test -race ./internal/artifact/`; expected PASS. Commit `feat(corpus): add the content-addressed artifact store`.

### Task 4: Workshop schema, shared types and the fenced commit

**Files:**
- Create: `apps/corpus/migrations/00001_control.sql`, `apps/corpus/migrations/00002_ingest.sql`, `apps/corpus/sqlc.yaml`, `apps/corpus/sql/{run,item,artifact,gap,ingest}.sql`, `apps/corpus/internal/store/` (generated plus `pool.go`), `apps/corpus/internal/migrate/migrate.go`, `apps/corpus/internal/stage/{types,commit}.go`, `apps/corpus/internal/stage/types_test.go`, `apps/corpus/internal/stage/commit_integration_test.go`
- Modify: `apps/corpus/internal/cli/cli.go` (register `migrate`)

**Interfaces:**
- Consumes: `artifact.Result` (Task 3).
- Produces: tables `run` (with `next_stage`, `until_stage`, `state` check including `starting`), `run_item` (states pending, done, gap, failed; `batch`; `artifact_id` nullable), `artifact` (unique stage, input_hash, recipe_hash), `gap` (unique run_id, stage, item_key, kind, detail; `accepted_at`, `note`), `listing`, `document`, `fetched_file`, `provision`, `relation`; `migrate.Up(ctx, url string) error`. In `internal/stage/types.go`: `Snapshot{SourceList json.RawMessage; SourceListHash string; Stages []string; Recipes map[string]string; TaxonomyVersion, WorkerVersion, PreviousManifest string}`; `Run{ID int64; Country, Rel string; Generation int; Snapshot Snapshot}`; `Gap{Kind, Detail string}`; `Item{Key, InputHash string; Skip *Gap}`; `Counts{Expected, Done, Gap, Failed, Pending int; UnacceptedGaps map[string]int}`; `RecipeHash(stage, version string, extra ...string) string`. In `commit.go`: `Commit{RunID int64; Stage, ItemKey string; Generation, Attempt int; RecipeHash string; Artifact *artifact.Result; ArtifactRows func(context.Context, pgx.Tx, int64) error; RunRows func(context.Context, pgx.Tx) error; Gaps []Gap; State, Message string}` and `CommitItem(ctx, pool *pgxpool.Pool, c Commit) (applied bool, err error)`.

- [ ] **Step 1: Write failing tests:** `TestRecipeHash_Stable` (fixed vector); integration `TestMigrate_UpFromEmpty`; `TestCommitItem_IgnoresLowerFence` (gen 2 attempt 1, then gen 1 attempt 5: `applied=false`, nothing changed); `TestCommitItem_SameArtifactTwiceKeepsOneRow` (`ArtifactRows` called once); `TestCommitItem_RunRowsAndGapsIdempotent` (two commits leave one listing row and one gap); `TestCommitItem_GapWithoutArtifact` (`Artifact` nil, state gap, `artifact_id` null); `TestRunItem_RejectsUnknownState`.
- [ ] **Step 2: Migrations and queries** per data.md; ingest tables reference `artifact(id)`; `provision` unique (artifact_id, path) and (artifact_id, node_id); `relation` keeps `raw_code`, `raw_label`, `kind`, `method`, `evidence`, `target_vbpl_id`, `target_number`, with no run-dependent target key.
- [ ] **Step 3: Implement** `CommitItem` in one transaction: artifact row on conflict do nothing then select id; `ArtifactRows` only for a new row; `RunRows`; gaps on conflict do nothing; `run_item` update where the stored pair is not above (`Generation`, `Attempt`) and `Generation` equals `run.generation`; zero rows rolls back with `applied=false`.
- [ ] **Step 4: Verify** `make generate && make test && make test-integration && make sqlc-check`; expected PASS and clean. Commit `feat(corpus): add the workshop schema and fenced item commit`.

### Task 5: Run lifecycle commands

**Files:**
- Create: `apps/corpus/internal/run/{start,status,cancel,recover,gaps,history,snapshot}.go` with `_test.go` files, `apps/corpus/internal/run/lifecycle_integration_test.go`
- Modify: `apps/corpus/internal/cli/cli.go` (register `start`, `status`, `cancel`, `recover`, `gaps`, `accept-gaps`, `export-history`)

**Interfaces:**
- Consumes: `stage.Snapshot`, `stage.RecipeHash` (Task 4); Temporal `client.Client`.
- Produces: `run.Deps{DB *pgxpool.Pool; Temporal client.Client; Recipes map[string]string; Stages []string; Sources []byte; Now func() time.Time}` (worker-compiled stage list and recipes are supplied by Task 6's registry); `run.Start(ctx, d Deps, cc, rel, until, previousManifest string) error`; `run.Status(ctx, d, cc, rel) (Report, error)`; `run.Cancel`; `run.Recover(ctx, d, cc, rel string, terminate bool) error`; `run.ListGaps`; `run.AcceptGaps(ctx, d, cc, rel, kind, note string) (int, error)`; `run.Publishable(ctx, db, runID int64) (bool, []string, error)`; `run.ExportHistory(ctx, d, workflowID, runID, path string) error`.

- [ ] **Step 1: Write failing tests:** `TestValidateRel` (table: `20261110` ok, `20261110_2` ok, `2026-11-10` rejected); `TestCompareRel` (`20261110_10` after `20261110_2`); `TestSnapshot_Extend` (adds missing stages and recipes; a changed frozen recipe is an error); integration with the dev server: `TestStart_RefusesWhileWorkflowOpen`, `TestStart_RefusesFreshStarting`, `TestStart_ReplacesStaleStarting`, `TestStart_RefusesOlderGenerationWorkflow` (message names `corpus cancel` and `corpus recover`), `TestStart_ResumesAtNextStage`, `TestStart_PausedRunAppendsNewStageRecipes`, `TestRecover_BumpsGenerationAndFails`, `TestAcceptGaps_AcceptsOnlyThatKind` (item with two kinds stays unpublishable), `TestPublishable_DoneItemWithUnacceptedGap`.
- [ ] **Step 2: Implement** the start protocol exactly as pipeline.md "Run lifecycle" (row lock, open-workflow check through `DescribeWorkflowExecution`, `starting` with a new generation committed before `ExecuteWorkflow`, fail-on-conflict id policy). Status prints per-stage counts, unaccepted gaps by kind, and pending activities with attempt and next retry time. `TaxonomyVersion` is `none` until the classification plan; `WorkerVersion` from `debug.ReadBuildInfo`.
- [ ] **Step 3: Verify** `make test && make test-integration`; expected PASS. Commit `feat(corpus): add run start, status, cancel, recover and gap acceptance`.

### Task 6: `BuildRelease` workflow, activities and stage framework

**Files:**
- Create: `apps/corpus/internal/workflow/{build,activities,registry}.go`, `apps/corpus/internal/workflow/{build,activities_integration,replay}_test.go`, `apps/corpus/internal/workflow/testdata/histories/.gitkeep`, `apps/corpus/internal/stage/{stage,batch}.go`, `apps/corpus/internal/stage/batch_integration_test.go`, `apps/corpus/internal/fault/fault.go` (`//go:build !faultinject`, no-op), `apps/corpus/internal/worker/worker.go`
- Modify: `apps/corpus/internal/cli/cli.go` (register `worker`; give `run.Deps` the registry's stages and recipes)

**Interfaces:**
- Consumes: Tasks 3 to 5.
- Produces: `stage.Stage` interface `{ Name() string; Recipe(s Snapshot) string; Plan(ctx, env Env, r Run) ([]Item, error); Process(ctx, env Env, r Run, it Item) (Output, error); Rows(ctx, art artifact.Result) (func(context.Context, pgx.Tx, int64) error, error); Resolve(ctx, env Env, r Run, it Item, art artifact.Result) (Resolution, error) }` where `Rows` builds the artifact's rows from its files only; `Output{Artifact *artifact.Result; State, Message string; Gaps []Gap}`; `Resolution{RunRows func(context.Context, pgx.Tx) error; Gaps []Gap}`; `stage.Retryable(err error) error`; `stage.Env{DB *pgxpool.Pool; Artifacts artifact.Store; Now func() time.Time}`. `workflow.Registry` lists stages in order; stage tasks append their constructor in `registry.go`. Workflow input `BuildInput{RunID int64; Country, Rel string; Generation int; Stage, Until string; Begin bool; HistoryLimit int}`. Control activities: `PlanStage(StageInput{RunID int64; Generation int; Stage string}) (PlanResult{Batches int; Cancelling bool})`, `CloseStage(StageInput) (CloseResult{Counts stage.Counts; NextStage string; Failed bool})`, `Checkpoint(StageInput) (CheckpointResult{Dir string})`, `MarkRun(MarkInput{RunID int64; Generation int; From []string; To, Message string}) (bool, error)`. Stage activities registered under the stage name with `activity.RegisterOptions{Name: name}` around one `RunBatch(BatchInput{RunID int64; Generation int; Stage string; Batch int}) (BatchResult{Done, Gap, Failed, Pending int; Cancelling bool})`. Fault hook `fault.Hit(point string, n int)` with points `after_rename` and `after_commit`.

- [ ] **Step 1: Write failing workflow tests** (`testsuite.WorkflowTestSuite`, mocked activities): `TestBuildRelease_BeginMarksRunning`; `TestBuildRelease_ContinuesAsNewWithNextStage`; `TestBuildRelease_StopsAtUntilAndMarksPaused`; `TestBuildRelease_DrainsBatchesThenCancels` (two batches, one reports cancelling: the workflow waits for both, then `MarkRun` cancelling→cancelled); `TestBuildRelease_CancelledBeforePlanning`; `TestBuildRelease_FailedItemsFailRun` (`CloseResult.Failed`: run failed, no checkpoint, no continue-as-new); `TestBuildRelease_SupersededEndsQuietly` (no `MarkRun` call); `TestBuildRelease_FailsAboveHistoryLimit` (limit 10).
- [ ] **Step 2: Write failing activity tests** (integration): `TestActivities_SupersededGenerationChangesNothing` (every activity); `TestMarkRun_OnlyFromAllowedStates` (cancelling never becomes paused); `TestRunBatch_SkipsDoneItems`; `TestRunBatch_ReusesExistingArtifactAndResolves` (no `Process` call, gaps recorded for this run); `TestRunBatch_AdoptedDirWithoutRowLoadsRows` (directory present, no artifact row: `Rows` inserts the same rows as a fresh run); `TestRunBatch_SkipItemCommitsGap`; `TestRunBatch_RetryableFailureErrorsUntilLastAttempt` (items stay pending; on attempt 5 they become failed); `TestRunBatch_HeartbeatsDuringSlowItem` (heartbeat timeout 2 s, item 5 s, interval 500 ms in the test); `TestPlanStage_ResetsFailedOnResume`; `TestCloseStage_EmptyStageAdvances`.
- [ ] **Step 3: Implement.** `PlanStage` checks generation and recipes, reports `Cancelling`, calls `Plan`, inserts missing `run_item` rows with `batch = index / 100`. `RunBatch` heartbeats from a ticker, checks `run.state` before each item, reuses an artifact through `Store.Open` when `Item.InputHash` is set, otherwise calls `Process`, calls `fault.Hit("after_rename", i)` after an artifact is published, then `Rows`, `Resolve` and `CommitItem` with the activity attempt, then `fault.Hit("after_commit", i)`. `CloseStage` records counts and `next_stage` in one transaction. Superseded is a non-retryable application error type `superseded`.
- [ ] **Step 4: Replay test** `replay_test.go` replays every JSON history in `testdata/histories/` with `worker.NewWorkflowReplayer`; it passes with none and gains one in Task 13.
- [ ] **Step 5: Verify** `make test && make test-integration`; expected PASS. Commit `feat(corpus): add the BuildRelease workflow and stage framework`.

### Task 7: Checkpoint activity and the restore runbook

**Files:**
- Create: `apps/corpus/internal/checkpoint/{checkpoint,pgdump}.go`, `apps/corpus/internal/checkpoint/checkpoint_test.go`, `docs/runbooks/corpus-local.md`
- Modify: `apps/corpus/internal/workflow/activities.go` (wire `Checkpoint`), `docs/runbooks/README.md`

**Interfaces:**
- Produces: `checkpoint.Take(ctx, cfg Config, runKey string, generation int, stage string) (Result{Dir string}, error)` with a `Dumper` interface for tests.

- [ ] **Step 1: Write failing tests:** `TestTake_CopiesOnlyNewArtifacts` (verifies manifests; nothing recopied); `TestTake_WritesCheckpointJSONLast` (dumper failure leaves no `checkpoint.json`); `TestTake_KeepsLastThree`; `TestPgDumpVersion_RequiresMajor18`.
- [ ] **Step 2: Implement.** `$CORPUS_CHECKPOINT_DIR/<cc>-<rel>/<generation>-<stage>/` holds `workshop.dump` (`pg_dump -Fc` once) and `checkpoint.json` (stage, generation, time, dump sha256) written last; artifacts are copied after the dump into `$CORPUS_CHECKPOINT_DIR/artifacts/` with the same relative paths; older checkpoint directories beyond three are deleted, artifacts never.
- [ ] **Step 3: Runbook** `docs/runbooks/corpus-local.md` "Restore from a checkpoint": stop the worker, restore `workshop.dump` into a fresh `workshop` database, copy artifacts back, `corpus recover <cc> <rel> [--terminate]`, `corpus start <cc> <rel>`. Link it from the runbooks README.
- [ ] **Step 4: Verify** `make test && make pre-push`; expected PASS. Commit `feat(corpus): add stage-boundary checkpoints`.

### Task 8: VBPL client and recorded fixtures

**Files:**
- Create: `apps/corpus/internal/vbpl/{client,types,download}.go`, `apps/corpus/internal/vbpl/client_test.go`, `apps/corpus/internal/vbpl/testdata/record.sh`, `apps/corpus/internal/vbpl/testdata/*.json`

**Interfaces:**
- Produces: `ListQuery{AgencyIDs []string; Keyword string; Page int}`; `ListPage{Total int; Items []ListItem}`; `ListItem{ID, DocNum, Title, IssueDate string; Raw json.RawMessage}`; `Detail{ID, DocNum, Title, DocType, Agency, IssueDate, EffFrom, EffTo, StatusCode, StatusName string; IsConsolidated, HasContent bool; References []Reference}`; `Reference{TargetID, TargetNumber, Code, Label string; Raw json.RawMessage}`; `Node{ID, Title, Level, Content string; PType, OrderIndex int; Children []Node}`; `File{Name, FullPath string; Size int64; URL string}`; `Downloaded{Path, SHA256 string; Size int64}`. Methods: `List(ctx, ListQuery) (ListPage, error)`, `Detail(ctx, id) (json.RawMessage, Detail, error)`, `Tree(ctx, id) (json.RawMessage, []Node, error)`, `Diagram(ctx, id) (json.RawMessage, error)`, `Files(ctx, id) ([]File, error)`, `Download(ctx, id string, f File, dir string, limit int64) (Downloaded, error)`. Errors `ErrNotFound` (404 or `success: false`), `ErrTooLarge`.

- [ ] **Step 1: Record fixtures** with `record.sh` (curl, the headers above, 1 request per second, presigned query strings removed): listing page 1 for agencies 62 and 908 with page size 5; detail, tree, diagram and files for `67878` (11 articles, 34 clauses, 17 points), the newest VBHN document from the listing (UUID id), and a document with an empty tree (id from `select a.external_id from silver.document d join silver.document_alias a on a.document_id = d.id and a.source = 'vbpl' where d.index_class = 'primary' and not exists (select 1 from silver.document_section s where s.document_id = d.id and s.ptype is not null) limit 1` on local `banhmi_vn`).
- [ ] **Step 2: Write failing tests** with `httptest`: `TestClient_RetriesOn503ThenSucceeds`; `TestClient_LimitsRate` (burst 1: 10 calls take at least 4.5 s); `TestClient_ResponseOverLimit` (`ErrTooLarge`); `TestDecodeDetail_NullObjects`; `TestDecodeTree_NestedNodes` (fixture counts); `TestDownload_RefreshesExpiredURL` (403, re-list once, 200); `TestDownload_TimeoutAfterPartialBodyRetriesFresh` (first attempt stalls after half the body; the result hash equals the full body's); `TestDownload_OverLimit` (`ErrTooLarge`, no file left); `TestClient_NeverLogsSignedQuery`.
- [ ] **Step 3: Implement** with a `rate.Limiter`, a semaphore of 2, `io.LimitReader(limit+1)` on every body, and a new temporary file per download attempt renamed into `dir` only on success.
- [ ] **Step 4: Verify** `go test ./internal/vbpl/`; expected PASS. Commit `feat(corpus): add the VBPL gateway client with fixtures`.

### Task 9: Source list and the `discover` stage

**Files:**
- Create: `sources/vn.yaml`, `apps/corpus/internal/source/list.go`, `apps/corpus/internal/source/list_test.go`, `apps/corpus/internal/stage/discover/{discover,expected}.go`, `apps/corpus/internal/stage/discover/discover_test.go`, `apps/corpus/internal/stage/discover/expected_integration_test.go`
- Modify: `apps/corpus/internal/workflow/registry.go`

**Interfaces:**
- Consumes: `vbpl.Client` (Task 8), `stage` framework (Task 6).
- Produces: `source.List{Country string; VBPL VBPLSources; RelationCodes map[string]RelationCode; StatusCodes map[string]StatusCode}`, `VBPLSources{SweepAgencies, KeywordAgencies, Keywords []string}`, `RelationCode{Kind, Label string; Amending, Superseding bool}`, `StatusCode{Class string; Current bool}`, `source.Parse([]byte) (List, error)`, `(List) RelationCodesHash() string`; discover items `vbpl:sweep` and `vbpl:keyword:<term>`; `discover.ExpectedSet(ctx, db *pgxpool.Pool, r stage.Run) ([]Expected, error)` with `Expected{Key, VBPLID string; Skip *stage.Gap}`.

- [ ] **Step 1: `sources/vn.yaml`** from banhmi seeds (data reuse, ADR 0001): sweep agencies `62`, `908`; keyword agencies = `issuer_code.csv` rows with source `vbpl`, `in_scope` true, `is_sbv` false; keywords = `discovery_keyword.csv` rows with source `vbpl`; relation codes from `relation_type.csv`; status codes from the empty-source rows of `validity_status.csv`. Stages read the list from `Run.Snapshot.SourceList`.
- [ ] **Step 2: Write failing tests:** `TestParseVN` (2 sweep agencies, 8 keyword agencies, 26 keywords, 11 relation codes, 8 status codes); `TestDiscover_StableAfterTwoPasses`; `TestDiscover_EqualTotalReplacementTriggersThirdPass`; `TestDiscover_UnstableAfterThreePassesIsGap` (gap `listing_unstable`, listing rows hold the union); `TestDiscover_DedupesShiftedPages`; integration `TestExpectedSet_WithdrawnAfterTwoAbsences`; `TestExpectedSet_OneAbsenceKeepsDocument`; `TestExpectedSet_IncompleteListingKeepsDocument` (unstable or failed listing, accepted or not, never counts as an absence); `TestExpectedSet_QueryDefinitionChangeIsNotComparable` (same keyword, different agencies); `TestExpectedSet_OrdersRelsNumerically`; `TestExpectedSet_ScopeRemovedIsSkipGap`.
- [ ] **Step 3: Implement.** Pages of 50, `sortBy=issueDate`, `sortDirection=desc`, `groupVbpl=false`, `optionDoc=title`, `matchMode=all_words`; `Process` publishes the raw passes with `PublishContent` and a `report.json`; `Resolve` writes `listing` rows and the `listing_unstable` gap; `ExpectedSet` applies pipeline.md "VBPL ingest rules". Register the stage.
- [ ] **Step 4: Verify** `make test && make test-integration`; expected PASS. Commit `feat(corpus): add the VN source list and discover stage`.

### Task 10: The `fetch` stage

**Files:**
- Create: `apps/corpus/internal/stage/fetch/{fetch,bundle}.go`, `apps/corpus/internal/stage/fetch/fetch_test.go`
- Modify: `apps/corpus/internal/workflow/registry.go`

**Interfaces:**
- Consumes: `vbpl.Client`, `discover.ExpectedSet`.
- Produces: fetch artifact with `detail.json`, `tree.json`, `diagram.json`, `files.json` (original name, sha256, size; no query strings), `files/<sha256>`, `report.json` (missing and oversized attachments); `document` and `fetched_file` rows.

- [ ] **Step 1: Write failing tests** against a fixture-backed `httptest` server: `TestFetch_WritesBundleAndRows`; `TestFetch_SameContentSameHash`; `TestFetch_ChangedFileChangesHash`; `TestFetch_MissingAttachmentIsGap` (item done, gap `missing_attachment`); `TestFetch_FileOverLimitIsGap` (`file_too_large`); `TestFetch_DetailNotFoundIsGap` (`not_found`, no artifact); `TestFetch_ResponseOverLimitIsGap` (`response_too_large`); `TestFetch_HostileFileNames` (`../../x`, duplicates, NFC and NFD forms of one name: every file stored by sha256 inside the bundle, names only in `files.json`); `TestFetch_RetryAfterRenameWithChangedContent` (one committed artifact; the earlier directory is an orphan).
- [ ] **Step 2: Implement.** `Plan` maps `ExpectedSet` to items (withdrawn and scope-removed as `Skip` gaps); `Process` downloads into `PublishContent`; `Resolve` turns `report.json` into gaps. Register the stage.
- [ ] **Step 3: Verify** `go test ./internal/stage/fetch/`; expected PASS. Commit `feat(corpus): add the fetch stage`.

### Task 11: The `extract` stage from the VBPL provision tree

**Files:**
- Create: `apps/corpus/internal/stage/extract/{extract,tree,text,labels}.go` with `_test.go` files
- Modify: `apps/corpus/internal/workflow/registry.go`

**Interfaces:**
- Consumes: fetch artifact `tree.json`; this run's fetch items in state done.
- Produces: artifact `provisions.json` and `report.json`; `provision` rows `{node_id, parent_node_id, kind, label, heading, ordinal, path, text}`; version `tree/1`.

- [ ] **Step 1: Write failing tests:** `TestExtract_Fixture67878` (11 `dieu`, 34 `khoan`, 17 `diem`; first article label `Điều 1`, heading `Phạm vi điều chỉnh và đối tượng áp dụng`, path `dieu-1`; clause path `dieu-2/khoan-1`); `TestText_ConvertsBreaksAndEntities`; `TestText_NormalizesToNFC`; `TestExtract_EmptyTreeIsGap` (`no_tree`); `TestExtract_DuplicatePathsDisambiguated` (`dieu-5~2`); `TestExtract_ArticleSequenceGap` (articles 1, 2, 4: `article_sequence`, detail `3`); `TestExtract_UnknownKindIsOther` (`unknown_node_kind`); `TestExtract_EmptyArticleTextIsGap` (`empty_text`); `TestExtract_TreeTooLargeIsGap` (depth 33; 50,001 nodes).
- [ ] **Step 2: Implement.** `Plan` returns this run's done fetch items with the fetch artifact sha256 as `InputHash`. Kind from `level` (Part `phan`, Chapter `chuong`, Section `muc`, Article `dieu`, Clause `khoan`, Point `diem`), else `ptype` (1 `phan`, 2 `chuong`, 3 or 4 `muc`, 5 `dieu`, 6 `khoan`, 7 `diem`), else `other`. Paths `phan-<n>`, `chuong-<roman, lowercase>`, `chuong-<roman>/muc-<n>`, `dieu-<n>`, `dieu-<n>/khoan-<m>`, `dieu-<n>/khoan-<m>/diem-<letter>`; repeats get `~2`, `~3` in node order. Text is each node's content as served: `<br>` to newline, tags dropped, entities unescaped, NFC. 5-minute context per document. Register the stage.
- [ ] **Step 3: Verify** `go test ./internal/stage/extract/`; expected PASS. Commit `feat(corpus): add the extract stage for VBPL trees`.

### Task 12: The `relate` stage

**Files:**
- Create: `apps/corpus/internal/stage/relate/{relate,codes}.go`, `apps/corpus/internal/stage/relate/relate_test.go`
- Modify: `apps/corpus/internal/workflow/registry.go`

**Interfaces:**
- Consumes: fetch artifact `detail.json` (`references`) and `diagram.json` (`documentNamesByType`); `source.List.RelationCodes` and `RelationCodesHash`; `discover.ExpectedSet`.
- Produces: artifact `relations.json`; `relation` rows; version `vbpl-relations/1` with the relation-code hash in the recipe.

- [ ] **Step 1: Write failing tests:** `TestRelate_MapsCodes` (code `10` to `amends_supplements`, raw code kept); `TestRelate_MergesReferencesAndDiagram`; `TestRelate_UnmappedCodeIsGap` (`unknown_relation_code`); `TestRelate_UnknownTargetIsGap` (target outside the expected set); `TestRelate_ReusedArtifactResolvesAgainstNewExpectedSet` (same artifact, target now expected: no gap); `TestRelate_CodeTableChangeChangesRecipe`.
- [ ] **Step 2: Implement.** Evidence is the verbatim label or snippet from the payload; provision ids only when the payload carries them; `Resolve` checks targets against `ExpectedSet`. Register the stage.
- [ ] **Step 3: Verify** `go test ./internal/stage/relate/`; expected PASS. Commit `feat(corpus): add the relate stage`.

### Task 13: Local services, proofs and the first live run

**Files:**
- Create: `deploy/local/{db-setup,temporal-schema,temporal-namespace}.sh`, `apps/corpus/internal/fault/fault_inject.go` (`//go:build faultinject`), `apps/corpus/test/proof/{harness,crash_after_rename,crash_after_commit,restart_midrun,recover_checkpoint}_test.go`, `apps/corpus/internal/workflow/testdata/histories/<first-run>.json`, `docs/research/vbpl-ingest-2026-11.md`
- Modify: `deploy/compose.yml`, `.env.example`, `Makefile` (`db-up` starts only `postgres`; `db-setup`, `temporal-schema`, `temporal-up`, `temporal-namespace`, `services-up` in that order), `docs/runbooks/corpus-local.md`, `docs/architecture.md`, `docs/research/README.md`, `docs/plans/README.md`

**Interfaces:**
- Produces: frontend `127.0.0.1:7233`, UI `127.0.0.1:8233`; databases `workshop` (role `pipeline`), `temporal` and `temporal_visibility` (role `temporal`); namespace `corpus`, 30-day retention.

- [ ] **Step 1: Local services.** Compose gains `temporal` (`docker.io/temporalio/server:1.32.2`, `DB=postgres12`, `POSTGRES_SEEDS=postgres`, `DBNAME=temporal`, `VISIBILITY_DBNAME=temporal_visibility`, user `temporal`) and `temporal-ui` (`docker.io/temporalio/ui:2.58.1`). `services-up` runs: PostgreSQL up and ready; roles and databases (idempotent, through `podman exec`, passwords from `.env`); schema through `temporalio/admin-tools:1.32.2` `temporal-sql-tool` (create, setup-schema, update-schema for both databases); Temporal and UI up and healthy; namespace created if missing. Verify image tags with `podman manifest inspect` first; record substitutes in the runbook. Expected: `make services-up` exits 0 twice in a row, and the UI lists `corpus`.
- [ ] **Step 2: Write the proof tests** (`integration faultinject`, dev server, fixture VBPL server with 30 documents, a real `pg_dump`): `TestProof_CrashAfterRename` (`CORPUS_FAULT=after_rename:7` exits the worker; a new worker finishes; artifact directories equal distinct items; no duplicate rows; each item done once); `TestProof_CrashAfterCommit` (`CORPUS_FAULT=after_commit:7` exits the worker after item 7 commits, so the activity completion is lost; the retried batch skips done items; rows, gaps and counts equal an uninterrupted run); `TestProof_RestartMidRun` (worker stopped during fetch and restarted after 5 s; reaches `paused` after `relate` with counts identical to an uninterrupted run); `TestProof_RecoverFromCheckpoint` (after the fetch checkpoint, drop the workshop database, restore the dump and artifacts, `corpus recover --terminate`, `corpus start`: completes with identical counts and no reprocessed fetch items).
- [ ] **Step 3: Verify** `make test-proof`; expected PASS.
- [ ] **Step 4: First live run.** `make services-up`, `corpus migrate`, `corpus worker` in the background, `corpus start vn <run date as YYYYMMDD> --until relate`. Record in `docs/research/vbpl-ingest-2026-11.md`: listing totals and passes per query, expected set, done and gaps per stage and kind, duration, checkpoint sizes, and a comparison with banhmi v049 (primary documents, documents with a tree, relations). Export one stage's history with `corpus export-history` into `testdata/histories/` and run the replay test.
- [ ] **Step 5: Docs.** `docs/architecture.md` describes `apps/corpus` as built; the runbook covers services, run, status, cancel, recover, gaps, accept-gaps and checkpoints; `docs/plans/README.md` marks this plan landed.
- [ ] **Step 6: Verify and commit** `make pre-push && make test && make test-integration && make test-proof`; expected PASS. Commit `feat(corpus): prove recovery and record the first VBPL run`.
