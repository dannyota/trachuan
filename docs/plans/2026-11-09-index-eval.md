# Index and Evaluation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Route in this repository: each task goes to one Codex `implementer` brief (`gpt-6.1-terra` high) except Task 10 (data adjudication, Codex `implementer` with the export the primary supplies) and Task 14 (primary, local run); the primary verifies every task, runs the checks and owns Git; the Claude `reviewer` reviews the whole branch after Task 13.

**Goal:** A paused run extends through `chunk`, `embed`, `build` and `eval`: the corpus is split into provision-owned chunks, embedded by a local Python embedder, loaded into a local `corpus_<cc>_<rel>` database with BM25 and HNSW, searched by a shared Go retrieval package, and scored under ADR 0010 on the legacy and old product cohorts.

**Architecture:** One root Go module `danny.vn/trachuan`. Shared code the future server imports lives under root `internal/`: `corpusdb` (schema template), `retrieval` and `retrieval/lexical` (search and encoding), `embedding` (embedder protocol types and HTTP client), `testpg` (integration test databases). Pipeline stages live in `apps/corpus/internal/stage/{chunk,embed,build,eval}` and follow the existing `stage.Stage` contract; evaluation logic lives in `apps/corpus/internal/evaluation`. The embedder is a uv-managed Python HTTP service in `apps/embedder` with two startup runtimes (ONNX CPU FP32 for queries, PyTorch MPS FP16 for documents). Recipe 1 is the study's measured baseline; tuning, taxonomy, signing and upload are later plans.

**Tech Stack:** Go 1.27.3, Temporal Go SDK v1.50.0, pgx/v5, sqlc 1.31.1, goose v3, golang.org/x/text, PostgreSQL 18 with pgvector and ltree (`pgvector/pgvector:pg18`), Python 3.12 with uv, FastAPI, Uvicorn, NumPy, transformers tokenizers, ONNX Runtime, PyTorch (optional `document` group), pytest, ruff.

**Spec:** `docs/design/index.md`, `chunk.md`, `embedder.md`, `lexical.md`, `corpus-schema.md`, `retrieval-recipe.md`, `evaluation.md`, `budgets.md` ("Indexing", "Retrieval"), `data.md`, `pipeline.md`; ADR 0009, 0010, 0011. Study: `docs/research/retrieval-hybrid-2026-11.md`. Every task reads the spec sections it names; the spec wins over this plan.

## Scope

|In|Out (later plans)|
|-|-|
|stages `chunk`, `embed`, `build`, `eval`; `apps/embedder`; root `internal/corpusdb`, `internal/retrieval`, `internal/retrieval/lexical`, `internal/embedding`, `internal/testpg`; legacy and old product cohorts with a provisional abstention threshold; `corpus eval` and `corpus inventory`|taxonomy import and seeds; subject-filter ordering; development and sealed cases and tuning trials; signing, S3 upload and activation; one-hop relation targets and keyword review (corpus scope plan); server and MCP|

## Global Constraints

- Module `danny.vn/trachuan`; Go 1.27.3; any task may add dependencies to the root `go.mod` and `go.sum`. Root `internal/` packages never import `apps/...`.
- Dense recipe (ADR 0009): `AITeamVN/Vietnamese_Embedding_v2`, revision `18b44161e041`, 1,024 dimensions, CLS pooling then L2 in float32, raw input with no prefix; document input `strip(context_prefix) + "\n" + strip(content)` at most 2,048 tokens; query input at most 256 tokens; inputs above a cap are rejected, never truncated.
- Chunk recipe 1: rough tokens `max(1, runes/2)` (0 for empty), body target 512 rough tokens, minimum 20 normalized runes, prefix title and headings 220 runes each, number 128 runes; article first; no overlap.
- Lexical recipe 1: input `content + " " + context_prefix`; lowercase, NFD, drop Mn, `đ`→`d`, letters and digits, whitespace syllables; FNV-1a 32 modulo 1,048,576 plus 1; k1 1.2, b 0.75; query hashed presence 1; at most 16,000 nonzeros per chunk, no pruning.
- Retrieval recipe 1: dense 50 and lexical 50 candidates; RRF k 60, dense weight 1, lexical weight 0.5, routed lexical weight 1; ANN pool `max(400, 24*dense_depth)`, ef_search `min(pool,1000)`, strict iterative scan, force_custom_plan, max_scan_tuples 20,000, scan_mem_multiplier 1; hydrate first `min(union, 8*limit)` fused chunks; group under nearest `khoan`; max reducer; per-document cap 3 with demoted backfill; family collapse off; default limit 8, maximum 50; evidence 14,000 code points per hit; relations 20 per hit, 1,000 code points each; at most 8 document references of 128 runes; HNSW m 16, ef_construction 64; eval exact cache depth dense 200, lexical 100; calibration sentinels -1 and 2.
- Rejection message, exact: `Vui lòng nhập câu hỏi tiếng Việt có dấu hoặc số hiệu văn bản.`
- Embedder limits: document batch at most 16 texts and 1 MiB body, 1 active, 1 queued, 1 s queue wait, 120 s deadline, Go client timeout 125 s; query 1 text, 64 KB body, 2 active, 8 queued, 250 ms wait, 2 s deadline, ONNX 2 intra-op and 1 inter-op threads; vector norm error at most 0.001.
- Build limits: 10,000 documents, 2,000,000 provisions, 500,000 chunks, 2,000,000 distinct terms, batches of 2,000 rows, `maintenance_work_mem` 512 MiB, `max_parallel_maintenance_workers=0`, 50-minute item deadline, 15-minute dump or restore deadline; eval item 50 minutes, 60 s per exact query, artifacts at most 2 GiB.
- Floors (instructions/testing.md): recall@8 0.90, MRR@8 0.66, current-law precision 0.99, abstention 0.95; this plan reports them and never fails a stage on a measured floor; `release_eligible` is always false.
- Never log query text, chunk text, vectors, SQL arguments or secrets. Never create or read `work/eval-sealed/`.
- Tests: write the failing test first; unit tests never wait on real time beyond 100 ms; integration tests use the `integration` tag and a fresh database; no test loads the 2.3 GB model or calls the network. Go files at most 400 lines, one concern each; doc comments on exported symbols; errors wrapped with `%w`. Python follows `instructions/python.md`.
- Secrets only in `.env` (mode 600); this plan adds `BUILDER_PASSWORD`. CI uses throwaway values in the workflow file.

## Review Focus

- Chunk evidence offsets that do not match the provision text they claim, so an agent quotes text the law does not contain (Task 3 `TestSplit_SpanSubstringsMatchSource` over every fixture; Task 14 runs the same check over all live extract artifacts).
- Query and document lexical encoding drifting apart, so routed number queries lose their lexical arm silently (Task 2 `TestEncode_QueryDocumentParity`).
- Vector rows misaligned with chunk ordinals when a middle chunk is rejected, so a hit carries another chunk's vector (Task 5 `TestEmbed_RejectedMiddleOrdinalKeepsAlignment`, Task 12 `TestLoad_VectorOrdinalAlignment`).
- Exact-search session settings leaking to the next pooled query, so serving silently runs without HNSW or with wrong settings (Task 6 `TestCollect_ExactSettingsRestoredOnPooledConnection`).
- A restored or rebuilt database differing from the one evaluated, so a report describes a database that no longer exists (Task 12 `TestResolve_RestoresLostDatabaseFromDump`, Task 14 evaluates both built and restored databases).

---

### Task 1: Test databases, corpus schema template and local builder role

**Files:**
- Create: `internal/testpg/testpg.go`, `internal/testpg/testpg_integration_test.go`, `internal/corpusdb/schema.sql`, `internal/corpusdb/schema.go`, `internal/corpusdb/schema_test.go`, `internal/corpusdb/schema_integration_test.go`
- Modify: `deploy/local/db-setup.sh`, `.env.example`, `docs/runbooks/corpus-local.md`

**Interfaces:**
- Produces: `testpg.NewDatabase(t *testing.T, extensions ...string) string` (build tag `integration`; admin URL from `CORPUS_TEST_ADMIN_URL`, default `postgres://trachuan@127.0.0.1:5432/postgres`; refuses a non-loopback host; creates `test_<random>` with the extensions; drops it at cleanup; returns its URL). `corpusdb.SchemaVersion = 1`, `corpusdb.SparseDimensions = 1048576`, `corpusdb.DB` interface `Exec(context.Context, string, ...any) (pgconn.CommandTag, error)`, `corpusdb.Apply(ctx context.Context, db DB, dimensions int) error` (only 1024 supported, else `corpusdb.ErrUnsupportedDimensions`), `corpusdb.SchemaSHA256(dimensions int) (string, error)`. Local role `corpus_builder` (LOGIN, CREATEDB, NOSUPERUSER, NOCREATEROLE, password from `BUILDER_PASSWORD`) and template database `corpus_template` owned by it with extensions `vector` and `ltree`.

- [ ] **Step 1: Write the failing tests.** `TestApply_RejectsUnsupportedDimensions` (768 returns `ErrUnsupportedDimensions`, no SQL sent through a recording fake `DB`); `TestSchemaSHA256_Stable` (same input, same hash; different dimension refused). Integration: `TestNewDatabase_CreatesAndDropsWithExtensions`; `TestApply_CreatesSchemaVersion1` (tables `document`, `source_file`, `provision`, `provision_validity`, `chunk`, `lexical_df`, `relation`, `coverage_gap`, `taxonomy_node`, `taxonomy_assignment`, `release` exist; `chunk.embedding` typmod 1024; `chunk.sparse` is `sparsevec(1048576)`; HNSW index on `chunk.embedding` with `vector_cosine_ops`, m 16, ef_construction 64); `TestApply_ConstraintsRejectBadRows` (a second `release` row, `document.country <> 'vn'`, a bad SHA-256 text, `eff_to < eff_from`, a relation kind outside the version 1 list, `provision.parent_id = id`).
- [ ] **Step 2: Run** `go test ./internal/corpusdb/...` and `make test-integration` filtered to `./internal/...`. Expected: FAIL, packages missing.
- [ ] **Step 3: Implement** `schema.sql` exactly as `corpus-schema.md` (every table, column, CHECK, UNIQUE, FK and index; `C` collation on identity text; one `{{dimensions}}` placeholder substituted after validation), `schema.go` embedding it, and `testpg`. Extend `db-setup.sh` idempotently: create `corpus_builder`, set its password through `\getenv` like the existing roles, create `corpus_template` owned by `corpus_builder`, install `vector` and `ltree` in it as the superuser, mark it a template. Add `BUILDER_PASSWORD` to `.env.example`; document the role and template in the runbook.
- [ ] **Step 4: Run** the same commands, then `make db-setup` twice locally (primary). Expected: PASS; second setup run prints `db-setup: ok` with no error.
- [ ] **Step 5: Commit** `feat: corpus schema template, test databases and local builder role`.

### Task 2: Lexical encoding

**Files:**
- Create: `internal/retrieval/lexical/{recipe,tokenize,hash,stats,encode}.go` and `_test.go` files beside them

**Interfaces:**
- Produces: `lexical.Recipe` (fields of `lexical.md` "Recipe and statistics"), `lexical.Recipe1() Recipe`, `(Recipe) JSON() ([]byte, error)` (compact fixed-field), `(Recipe) Hash() string`, `lexical.Entry{Index int32; Value float32}`, `lexical.Vector []Entry`, `lexical.Stats{N, TotalTokens int64; AvgDL float64; DF map[string]int64}`, `lexical.StatsSummary` (the `lexical_stats` JSON object) with `(Stats) Summary(recipe Recipe, nnz []int) StatsSummary`, `lexical.TextIterator` interface `Next(context.Context) (string, bool, error)`, `Tokenize(text string, recipe Recipe) ([]string, error)`, `Train(ctx context.Context, texts TextIterator, recipe Recipe) (Stats, error)`, `Document(text string, stats Stats, recipe Recipe) (Vector, error)`, `Query(text string, recipe Recipe) (Vector, error)`, `Dimension(token string, recipe Recipe) int32`.

- [ ] **Step 1: Write the failing tests.** `TestTokenize_FoldsVietnamese` (`"Điều 3. Ngân hàng"` → `["dieu","3","ngan","hang"]`); `TestTokenize_SplitsPunctuationAndKeepsDigits` (`"50/2024/TT-NHNN"` → `["50","2024","tt","nhnn"]`); `TestDimension_FNV1aModuloPlusOne` (known token hashes, range 1..1048576); `TestTrain_CountsDFOncePerChunk`; `TestDocument_BM25Weight` (hand-computed value for a two-chunk corpus within 1e-6); `TestDocument_SumsCollisionsInSortedTokenOrder`; `TestQuery_HashedPresenceIgnoresFrequency`; `TestEncode_QueryDocumentParity` (every query dimension of a text appears among that text's document dimensions); `TestTrain_EmptyCorpusAvgDLOne`; `TestDocument_RejectsOverNonzeroLimit`; `TestRecipe_UnknownAlgorithmRejected`.
- [ ] **Step 2: Run** `go test ./internal/retrieval/lexical/`. Expected: FAIL.
- [ ] **Step 3: Implement** per `lexical.md`: float64 statistics, float32 output sorted by index, deterministic summation order, `golang.org/x/text/unicode/norm`.
- [ ] **Step 4: Run** the same command. Expected: PASS.
- [ ] **Step 5: Commit** `feat: BM25 lexical encoding shared by build and search`.

### Task 3: Chunk stage

**Files:**
- Create: `apps/corpus/internal/stage/chunk/{chunk,split,render,prefix,artifact,rows}.go` with tests, `apps/corpus/migrations/00003_chunk.sql`, `apps/corpus/sql/chunk.sql`
- Modify: `apps/corpus/internal/store/` (regenerated by `make generate`, never hand-edited)

**Interfaces:**
- Consumes: `stage.Stage`, `stage.RecipeHash`, `artifact.Store`, extract `provisions.json` rows (`node_id, parent_node_id, kind, label, heading, ordinal, path, text`), fetch `document` rows.
- Produces: `chunk.New() *chunk.Stage` (name `chunk`); `chunk.Recipe1` with `(Recipe) Hash() string`; `chunk.Node` (extract provision row), `chunk.Meta{Key, Number, Title, EffFrom, IssueDate string}`; `chunk.Split(nodes []Node, meta Meta, recipe Recipe) (File, Coverage, error)`; artifact types `chunk.File` (`chunks.json`, fields as `chunk.md`) and `chunk.Coverage` (`coverage.json`); `chunk.ReadArtifact(dir string) (File, Coverage, error)` (verifies nothing; callers verify the manifest first); workshop table `chunk` and sqlc queries `InsertChunk`, `ListChunks(artifact_id)`.

- [ ] **Step 1: Write the failing tests** for each case in `chunk.md` "Workshop projection and checks": `TestSplit_SmallArticleOneChunk`, `TestSplit_LargeArticleRecursesIntoClausesWithLead`, `TestSplit_LabelOnlyLeadNotCarried`, `TestSplit_ParentContainsChildrenUsesParentOnce`, `TestSplit_AmbiguousContainmentGap`, `TestSplit_ShortAndLabelLeavesOmitted`, `TestSplit_DuplicateKeepsFirstAndRecordsOrdinal`, `TestSplit_LongLineShardsByWords`, `TestSplit_OversizedWordStaysIntact`, `TestSplit_NonArticleTextChunked`, `TestSplit_EmptyTreeNoTreeGap`, `TestSplit_InvalidParentIsSourceData`, `TestSplit_SpanSubstringsMatchSource` (every span's source substring equals the rendered substring, every non-whitespace source interval covered or omitted), `TestSplit_OrdinalsRepeatable`, `TestPrefix_FieldsCapsAndDateFallback`, `TestChunkStage_InputHashChangesWithFetchMetadata` (same extract hash, different fetch hash). Integration: `TestChunkStage_RowsAndResolveGaps` (rows land in `chunk` with the composite FK to `provision`; resolve recreates `chunk_omission` and `no_chunks` gaps).
- [ ] **Step 2: Run** `go test ./apps/corpus/internal/stage/chunk/` and the integration test. Expected: FAIL.
- [ ] **Step 3: Implement** per `chunk.md` (plan one item per done extract item with a done fetch item; input hash `stage.RecipeHash("chunk-input","1",document_key,fetch_sha256,extract_sha256)`; recipe `stage.RecipeHash("chunk","vn-article/1",recipe_json)`); migration `00003_chunk.sql` with the `chunk` table of `chunk.md`; regenerate the store.
- [ ] **Step 4: Run** the same commands plus `make sqlc-check`. Expected: PASS.
- [ ] **Step 5: Commit** `feat(corpus): chunk stage with provision-owned spans`.

### Task 4: Embedder service

**Files:**
- Create: `apps/embedder/pyproject.toml`, `apps/embedder/uv.lock`, `apps/embedder/.python-version`, `apps/embedder/AGENTS.md`, `apps/embedder/src/trachuan_embedder/{__init__,__main__,app,admission,recipe,tokenizer,runtime,onnx_runtime,torch_runtime,bundle}.py`, `apps/embedder/tests/test_*.py`, `apps/embedder/scripts/make_bundle.py`
- Modify: `scripts/pre-push.sh`, `.github/workflows/ci.yml`, `Makefile` (`embedder-test`, `embedder-query`, `embedder-document`)

**Interfaces:**
- Produces: the HTTP contract in `embedder.md` exactly (`GET /v1/recipe`, `POST /v1/tokenize`, `POST /v1/embed`, `GET /healthz`, `GET /readyz`, error table); `python -m trachuan_embedder --runtime query|document --model-dir DIR [--host H] [--port P]`; `scripts/make_bundle.py MODEL_DIR` writes `bundle.json` (model id, revision, resolved revision, file paths, sizes, SHA-256) that startup verifies. Default ports: document 8092 on 127.0.0.1, query 8091.

- [ ] **Step 1: Write the failing tests** with an injected fake tokenizer, runtime and clock: `test_recipe_hash_fixed_field_json`, `test_tokenize_counts_without_truncation`, `test_embed_rejects_batch_when_one_input_over_cap` (422 with index and count, no runtime call), `test_embed_returns_float32_l2_normalized`, `test_embed_rejects_non_finite_or_zero_norm` (500), `test_recipe_mismatch_409`, `test_unknown_field_and_duplicate_key_400`, `test_body_over_cap_413_before_parse`, `test_wrong_media_type_415`, `test_query_mode_accepts_exactly_one_text`, `test_admission_busy_429_when_queue_full`, `test_cancelled_request_keeps_active_slot_until_inference_ends`, `test_deadline_504`, `test_readyz_503_until_bundle_verified`, `test_bundle_hash_mismatch_refuses_ready`, `test_document_mode_refuses_without_mps`, `test_no_text_in_logs`; a tiny generated ONNX graph tests `onnx_runtime` CLS selection and normalization.
- [ ] **Step 2: Run** `make embedder-test`. Expected: FAIL.
- [ ] **Step 3: Implement** per `embedder.md` and ADR 0011; pin Python 3.12 and every dependency in `uv.lock`; `document` dependency group adds PyTorch; dev group adds pytest and ruff. `pre-push.sh` runs `uv run --frozen --only-group dev ruff check` and `ruff format --check` in `apps/embedder` when `uv` is installed. CI: preflight output `embedder` (true when `apps/embedder/**` or the workflow changes), job `embedder` using `astral-sh/setup-uv` pinned by full SHA (verify the tag against the GitHub release before pinning), running ruff and `uv run --frozen pytest`; the `ci` aggregate requires it when it ran.
- [ ] **Step 4: Run** `make embedder-test` and `make pre-push`; the primary runs the opt-in real-model smoke (`make embedder-query`, one Vietnamese query, token count and norm) and records the output. Expected: PASS.
- [ ] **Step 5: Commit** `feat(embedder): Python embedding service with query and document runtimes`.

### Task 5: Embedding client and embed stage

**Files:**
- Create: `internal/embedding/{recipe,client,errors}.go` with tests, `internal/embedding/recipes/vi-v2-document.json`, `internal/embedding/recipes/vi-v2-query.json`, `apps/corpus/internal/stage/embed/{embed,artifact,rows}.go` with tests

**Interfaces:**
- Consumes: Task 3 `chunk.ReadArtifact`, `chunk.File`; Task 4 HTTP contract; the bundle hashes the primary records from `make_bundle.py` on the MacBook into the two recipe files.
- Produces: `embedding.Recipe` (fields of `embedder.md` "Recipe report") with `(Recipe) Hash() string`; `embedding.Report{ProtocolVersion int; RecipeHash string; Recipe Recipe}`; `embedding.Vector{Values []float32; TokenCount int; RecipeHash string}`; `embedding.DocumentRecipe1() Recipe` and `embedding.QueryRecipe1() Recipe` from the embedded JSON files; `embedding.NewClient(baseURL string, hc *http.Client) *Client`; `(*Client) Recipe(ctx) (Report, error)`, `Tokenize(ctx, recipeHash string, texts []string) ([]int, error)`, `Embed(ctx, recipeHash string, texts []string) ([]Vector, error)`; `*embedding.Error{Status int; Code string; Index, TokenCount *int}`; `embedding.Query{Client *Client; RecipeHash string}` with `Recipe(ctx) (Recipe, error)` and `Embed(ctx, text string) (Vector, error)`. `embed.New(client *embedding.Client, expected embedding.Recipe) *embed.Stage` (name `embed`); artifact `vectors.f32`, `index.json`, `report.json`; `embed.ReadArtifact(dir string) (Index, error)` and `embed.OpenVectors(dir string, index Index) (*VectorReader, error)` with `(*VectorReader) Row(i int) ([]float32, error)`.

- [ ] **Step 1: Write the failing tests** against an `httptest` fake embedder: `TestClient_DecodesErrorTable`, `TestClient_RejectsWrongDimensionsOrIndex`, `TestEmbed_InputIsStrippedPrefixNewlineContent`, `TestEmbed_TokenizeThenOmitOverCap` (gap `embedding_input_too_long` with token count), `TestEmbed_PacksBatchesUnderByteCap`, `TestEmbed_SingleInputOverByteCapIsGap` (`embedding_input_bytes_exceeded`), `TestEmbed_RejectedMiddleOrdinalKeepsAlignment` (rows skip the rejected ordinal, each row's `input_sha256` matches its chunk), `TestEmbed_RecipeMismatchIsInfrastructureError`, `TestEmbed_RetryableStatuses` (429, 503, 504, transport error are retryable; 400, 409, 415, 422 after tokenize, malformed output are not), `TestEmbed_EmptyChunkArtifactNoCalls`, `TestArtifact_SizeAndNormChecked`.
- [ ] **Step 2: Run** `go test ./internal/embedding/ ./apps/corpus/internal/stage/embed/`. Expected: FAIL.
- [ ] **Step 3: Implement** per `embedder.md` "Embed stage" (input hash = chunk artifact hash; recipe `stage.RecipeHash("embed","dense/1",document_recipe_json)`; `Rows` is a no-op; `Resolve` regenerates gaps; no nested retry loop).
- [ ] **Step 4: Run** the same command. Expected: PASS.
- [ ] **Step 5: Commit** `feat(corpus): embed stage and embedder client`.

### Task 6: Retrieval engine, validation and candidate arms

**Files:**
- Create: `internal/retrieval/{types,recipe,validate,numbers,engine,dense,lexical,hydrate}.go` with unit tests, `internal/retrieval/engine_integration_test.go`, `internal/retrieval/testdata/` (fixture builder in Go, no large files)

**Interfaces:**
- Consumes: Task 1 `corpusdb.Apply`, `testpg.NewDatabase`; Task 2 `lexical.Query`, `lexical.Recipe`; Task 5 `embedding.Recipe`, `embedding.Vector`.
- Produces: the types of `retrieval-recipe.md` "Go boundary" with `Embedder` as `Recipe(context.Context) (embedding.Recipe, error)` and `Embed(context.Context, string) (embedding.Vector, error)`; `retrieval.Recipe1() Recipe` (all values but the threshold), `(Recipe) JSON() ([]byte, error)`, `(Recipe) Hash() string`; `retrieval.Release` (release row fields); `retrieval.Options{Now func() time.Time; Profile Profile}`; `Open(ctx, db *pgxpool.Pool, e Embedder, o Options) (*Engine, error)`; `OpenDraft(ctx, db *pgxpool.Pool, e Embedder, r Release, o Options) (*Engine, error)`; `(*Engine) Release() Release`; `(*Engine) Collect(ctx, req Request, d Depths) (Arms, error)`; `(*Engine) CollectLegacy(ctx, req Request, d Depths) (Arms, error)` (skips only the diacritics gate); `retrieval.ValidateQuery(q string) (Validation, error)`; `retrieval.NormalizeNumber(s string) string`; `retrieval.ParseReferences(q string) ([]string, error)`.

- [ ] **Step 1: Write the failing tests.** Unit: `TestValidateQuery_DiacriticsRequired` (exact message), `TestValidateQuery_UnsupportedQuery`, `TestValidateQuery_DocumentNumberPassesWithoutMarks`, `TestParseReferences_Grammar` (accepts `50/2024/TT-NHNN`, `21/VBHN-NHNN`; rejects `2024`, `50/2024`, `%`, more than 8 references, a reference over 128 runes), `TestNormalizeNumber` (NFC, uppercase, `Đ`→`D`, spaces around `/` and `-` removed, leading zeros kept), `TestRecipe1_HashStable`. Integration on a synthetic corpus built by the test (three documents with current, expired and missing-date validity; deterministic 1,024-dimension vectors; sparse vectors from `lexical.Document`): `TestOpen_RejectsMissingReleaseFieldsAndTypmodMismatch`, `TestOpen_RejectsEmbedderRecipeMismatch`, `TestOpenDraft_SearchRefusedUntilCalibrated`, `TestCollect_ExactDenseOrdersByDistanceThenID`, `TestCollect_ExactPlanAvoidsHNSW` (EXPLAIN), `TestCollect_ANNFallsBackToExactOnShortfall`, `TestCollect_LexicalPositiveScoresOnly`, `TestCollect_EligibilityCurrentDefaultAndAsOf`, `TestCollect_ExactSettingsRestoredOnPooledConnection` (pool of one connection: exact then ANN; settings back to defaults, also after a forced error), `TestCollect_HydratesUnionWithMemberCosines`, `TestCollect_NoQueryTextInErrors`.
- [ ] **Step 2: Run** `go test ./internal/retrieval/` and `make test-integration`. Expected: FAIL.
- [ ] **Step 3: Implement** per `retrieval-recipe.md` "Go boundary", "Validation and routing", "Candidate SQL and ranking" (arms and hydration only; shaping is Task 7). All SQL values bound; transaction-local settings; evaluation and serving statement deadlines from budgets.
- [ ] **Step 4: Run** the same commands. Expected: PASS.
- [ ] **Step 5: Commit** `feat(retrieval): engine, query validation and candidate arms`.

### Task 7: Retrieval shaping, evidence and abstention

**Files:**
- Create: `internal/retrieval/{fusion,group,validity,evidence,relations,shape,search}.go` with unit tests; extend `engine_integration_test.go`

**Interfaces:**
- Consumes: Task 6 `Arms`, `Recipe`, `Engine`.
- Produces: `retrieval.Shape(arms Arms, recipe Recipe, limit int, mode ArmMode) (Response, error)`; `(*Engine) Search(ctx, req Request) (Response, error)`; `retrieval.ErrCacheInsufficient`.

- [ ] **Step 1: Write the failing tests.** `TestFuse_WeightedRRFWithMissingArm`, `TestFuse_TiesByChunkID`, `TestFuse_RoutedLexicalWeight`, `TestGroup_PointRollsUpToNearestClause`, `TestGroup_ArticleOwnedStaysArticle`, `TestGroup_MaxReducerNeverSums`, `TestCap_ThreePerDocumentWithDemotedBackfill`, `TestAbstain_FirstGroupBestCosineIncludingLexicalOnly`, `TestAbstain_EqualityPasses`, `TestAbstain_NoPrimaryHits`, `TestValidity_DefaultCurrentAndExplicitAsOf`, `TestValidity_MissingStartGapAndIndeterminateCurrent`, `TestValidity_NonCurrentGroupUsesRemainingSlots`, `TestEvidence_FullTextWithinCap`, `TestEvidence_CutAtCapMarksIncompleteWithNextPart`, `TestEvidence_FragmentsSliceHitText`, `TestEvidence_AmbiguousParentNoChildCredit`, `TestCitation_FromAncestryLabels`, `TestRelations_CappedSortedWithOmissionGap`, `TestShape_CacheInsufficientFails`, `TestShape_SingleArmRemovesOtherArmBeforeFusion`. Integration: `TestSearch_EndToEndOnSyntheticCorpus` (limit 8 and 20, both modes).
- [ ] **Step 2: Run** `go test ./internal/retrieval/` and `make test-integration`. Expected: FAIL.
- [ ] **Step 3: Implement** per `retrieval-recipe.md` "Candidate SQL and ranking" (fusion onward), "Validity and evidence".
- [ ] **Step 4: Run** the same commands. Expected: PASS.
- [ ] **Step 5: Commit** `feat(retrieval): fusion, provision grouping, evidence and abstention`.

### Task 8: Evaluation matching and metrics

**Files:**
- Create: `apps/corpus/internal/evaluation/{cases,mapping,legacy,product,metrics}.go` with unit tests and small synthetic fixtures under `apps/corpus/internal/evaluation/testdata/unit/`

**Interfaces:**
- Consumes: Task 7 `retrieval.Response`, `retrieval.Hit`.
- Produces: `evaluation.Case`, `evaluation.Expectation`, `evaluation.LoadLegacy(r io.Reader) ([]Case, error)`; `evaluation.IdentityMap`, `evaluation.LoadIdentityMap(r io.Reader) (IdentityMap, error)`, `(IdentityMap) Validate(cases []Case, legacySHA256, inventoryHash string) error` (`mapping_incomplete` on a missing or ambiguous entry); `evaluation.Suite{Legacy []Case; Map IdentityMap; Rejections []string; Hashes FixtureHashes}`; `evaluation.MatchLegacy(e Expectation, hits []retrieval.Hit) (rank int, relation bool)`; `evaluation.MatchProduct(entry MapEntry, hits []retrieval.Hit) (rank int)`; `evaluation.Metrics` with `RecallAt`, `MRRAt8`, `CurrentLawPrecision`, `Abstention`, `EvidenceCharacters`, `Quantile(sorted []float64, p float64) *float64` (nearest rank `sorted[ceil(p*n)-1]`).

- [ ] **Step 1: Write the failing tests.** `TestLoadLegacy_ValidatesIDsAndFlags`, `TestValidate_MissingOrAmbiguousEntryBlocks`, `TestValidate_InventoryHashMismatchBlocks`, `TestMatchLegacy_NumberEqualityThenContainment`, `TestMatchLegacy_ArticleClausePointTokens`, `TestMatchLegacy_RelationOkDocumentOnly`, `TestMatchProduct_ExactKeyAndAnchor`, `TestMatchProduct_NoSubstringCredit` (`09/2020/TT-NHNN` never matches `109/2020/TT-NHNN`), `TestMatchProduct_ParentChildCreditNeedsCompleteChild`, `TestMatchProduct_RelationAlternativeNeedsEdgeAndHash`, `TestMatchProduct_AbsentAlwaysMiss`, `TestMetrics_RecallMicroMacroMRR`, `TestMetrics_PrecisionCountsEveryPrimaryHitNullOnZero`, `TestMetrics_LegacyPrecisionTrimsTrailingNonCurrent`, `TestMetrics_AbstentionConfusion`, `TestQuantile_NearestRank`.
- [ ] **Step 2: Run** `go test ./apps/corpus/internal/evaluation/`. Expected: FAIL.
- [ ] **Step 3: Implement** per `evaluation.md` "Fixtures and identity adjudication", "Scoring contracts", "Metrics". Reimplement the legacy matcher from the description; copy no banhmi code.
- [ ] **Step 4: Run** the same command. Expected: PASS.
- [ ] **Step 5: Commit** `feat(corpus): evaluation matchers and metrics`.

### Task 9: Build inventory, normalization and loading

**Files:**
- Create: `apps/corpus/internal/stage/build/{inventory,normalize,load,lexical}.go` with tests, `apps/corpus/sql/build.sql`, `apps/corpus/internal/cli/inventory.go`
- Modify: `apps/corpus/internal/store/` (regenerated), `apps/corpus/internal/cli/commands.go`

**Interfaces:**
- Consumes: Task 1 `corpusdb.Apply`; Task 2 `lexical.Train`, `lexical.Document`, `lexical.Recipe1`; Task 3 `chunk.ReadArtifact`; Task 5 `embed.ReadArtifact`, `embed.OpenVectors`; Task 6 `retrieval.NormalizeNumber`; `artifact.Store`, `stage.Run`.
- Produces: `build.Inventory` with `Items []InventoryItem`, `Gaps []InventoryGap`, `ExpectedKeys []string`, `(Inventory) SourceManifestHash() string`, `(Inventory) IdentityHash() string`; `build.LoadInventory(ctx, q store.Querier, run stage.Run, through string) (Inventory, error)` (stages up to and including `through`); `build.ParseDate(raw string) (*time.Time, error)`, `build.Status(code string, codes source.StatusCodes) (class string, current bool, known bool)`; `build.Load(ctx, conn *pgx.Conn, inv Inventory, store artifact.Store, recipes Recipes, codes source.StatusCodes) (Counts, error)` loading documents, source files, provisions (preorder, ltree paths), chunks with vectors, BM25 sparse vectors in two streaming passes, `lexical_df`, relations and `coverage_gap` into a database that already has the schema; `build.Counts`. CLI `corpus inventory <cc> <rel> --out FILE` writes the identity inventory JSON and prints `identity_inventory_hash`.

- [ ] **Step 1: Write the failing tests.** Unit: `TestParseDate_FormatsAndRejects`, `TestStatus_UnknownCode`, `TestInventory_SortedAndHashStable`, `TestInventory_IdentityHashIgnoresChunkAndEmbed`. Integration (workshop via `testdb`, corpus via `testpg` + `corpusdb.Apply`, artifacts in a temp store): `TestLoad_IncludesMetadataForNoTree`, `TestLoad_ExcludesWithdrawnAndFetchGaps`, `TestLoad_VectorOrdinalAlignment`, `TestLoad_ChunkCountReconciles` (chunks = embedded + rejected), `TestLoad_EmbeddingGapDocument`, `TestLoad_UnresolvedRelationsKept`, `TestLoad_InvalidDatesBecomeNullWithGap`, `TestLoad_LexicalStatsAndDF`, `TestLoad_RefusesArtifactHashMismatch`, `TestLoad_OverLimitFails`.
- [ ] **Step 2: Run** `go test ./apps/corpus/internal/stage/build/` and `make test-integration`. Expected: FAIL.
- [ ] **Step 3: Implement** per `index.md` "Build stage" (inventory and hashes), "Inclusion and losses", "Normalization", and `corpus-schema.md` id assignment rules. No database lifecycle here.
- [ ] **Step 4: Run** the same commands plus `make sqlc-check`. Expected: PASS.
- [ ] **Step 5: Commit** `feat(corpus): build inventory, normalization and corpus loading`.

### Task 10: Fixtures and identity map for vn/20261109

**Files:**
- Create: `apps/corpus/internal/evaluation/testdata/vn/legacy.json` (banhmi `deploy/eval/golden.json` byte for byte), `fixtures.json`, `old-product.json`, `identity-map.json`

**Interfaces:**
- Consumes: Task 8 formats and `Validate`; Task 9 `corpus inventory vn 20261109`; an export the primary writes to the scratchpad (for every number and alternate number in the 96 expectations: matching document keys, VBPL ids, fetch hashes, provision anchors, labels and text).
- Produces: one map entry per expectation, `resolved` with adjudicated alternatives and quoted evidence, or `absent` with every number searched.

- [ ] **Step 1: Write the failing test** `TestVNFixtures_Validate` (loads the four files, `Validate` passes against the recorded inventory hash, 92 cases, 96 expectations, 3 rejection ids, 5 negatives).
- [ ] **Step 2: Run** `go test ./apps/corpus/internal/evaluation/ -run TestVNFixtures`. Expected: FAIL, files missing.
- [ ] **Step 3: Write the fixtures.** Adjudicate each expectation from the stored source text in the export, never from retrieval ranks; quote the span that supports each provision anchor; mark the five documents missing from the run `absent` with the numbers searched. The primary spot-checks at least 20 entries against the source text and lists every judgment call in the report.
- [ ] **Step 4: Run** the same command. Expected: PASS.
- [ ] **Step 5: Commit** `test(corpus): vn legacy cases and adjudicated identity map`.

### Task 11: Evaluation run, calibration, cache and report

**Files:**
- Create: `apps/corpus/internal/evaluation/{calibrate,cache,report,run}.go` with tests

**Interfaces:**
- Consumes: Tasks 6 and 7 `Engine.Collect`, `Engine.CollectLegacy`, `Shape`; Task 8 suite and metrics.
- Produces: `evaluation.Options{EvaluationDate civil date; Modes []retrieval.Mode; PreviousReport string; ...}`; `evaluation.Calibrate(ctx, engine *retrieval.Engine, suite Suite) (Calibration, error)`; `evaluation.Run(ctx, engine *retrieval.Engine, suite Suite, o Options) (Report, Cache, error)`; `(Report) WriteFiles(dir string) error` and `(Cache) WriteFiles(dir string) error` writing `report.json`, `cases.jsonl`, `arms.jsonl`, `queries.f32`, `query-index.json`.

- [ ] **Step 1: Write the failing tests** with a fake engine interface over recorded arms: `TestCalibrate_MidpointsAbstainAllNegativesMaximizePositives`, `TestCalibrate_TieChoosesLowest`, `TestCalibrate_NoFeasibleKeepsAlwaysAbstain`, `TestCalibrate_RoundedMidpointUsesUpper`, `TestRun_RejectionChecksNoEmbedCall`, `TestRun_RejectionsOutsideDenominators`, `TestRun_FloorsPassFailUnavailable`, `TestRun_ReleaseEligibleAlwaysFalse`, `TestRun_InfrastructureErrorIsIncompleteNotAbstention`, `TestRun_QueryVectorReusedAcrossLimitsAndModes`, `TestReport_NoQuestionText` (no fixture question appears in any written file), `TestReport_PreviousDiffRejectsIncompatibleScorer`, `TestCache_DeepArmsAndGroupsComplete`.
- [ ] **Step 2: Run** `go test ./apps/corpus/internal/evaluation/`. Expected: FAIL.
- [ ] **Step 3: Implement** per `evaluation.md` "Metrics", "Provisional calibration", "Cache and report format".
- [ ] **Step 4: Run** the same command. Expected: PASS.
- [ ] **Step 5: Commit** `feat(corpus): evaluation run, provisional calibration and reports`.

### Task 12: Build stage lifecycle

**Files:**
- Create: `apps/corpus/internal/stage/build/{build,database,dump,resolve,calibration}.go` with tests, `apps/corpus/internal/stage/build/lifecycle_integration_test.go`

**Interfaces:**
- Consumes: Task 9 `LoadInventory`, `Load`; Task 11 `Calibrate`; Task 6 `OpenDraft`; Task 1 template database.
- Produces: `build.Config{BuilderURL, PgDump, PgRestore string; Embedder retrieval.Embedder; Suite evaluation.Suite; Now func() time.Time}`; `build.New(cfg Config) *build.Stage` (name `build`); artifact `corpus.dump`, `build.json`, `inputs.json`, `calibration.json`; `build.DatabaseName(cc, rel string) (string, error)`.

- [ ] **Step 1: Write the failing tests** (integration, fake embedder returning deterministic vectors): `TestBuild_CreatesFinalDatabaseWithReleaseRow`, `TestBuild_RefusesPlanWhilePredecessorPendingOrFailed`, `TestBuild_RefusesIdentityMapMismatch`, `TestBuild_TemporaryNameAndOwnershipMarker`, `TestBuild_ForeignTemporaryDatabaseNotAdopted`, `TestBuild_StaleGenerationCannotCommit`, `TestBuild_DumpMatchesBuildRecord`, `TestResolve_RestoresLostDatabaseFromDump`, `TestResolve_MismatchedFinalDatabaseIsError`, `TestResolve_CrashBeforeRenameRecreatesOnlyOwnedTemporary`, `TestBuild_HostileNamesRejected`, `TestBuild_EmptyTaxonomyTables`, `TestBuild_CalibrationRecordedInRecipe`, `TestBuild_CancellationStopsCopy`.
- [ ] **Step 2: Run** `make test-integration` for the package. Expected: FAIL.
- [ ] **Step 3: Implement** per `index.md` "Database lifecycle", "Build artifact and resolve": advisory lock per country and rel; clone `corpus_template`; load; HNSW; ANALYZE; calibrate on `OpenDraft`; insert the release row; `pg_dump -Fc --no-owner --no-privileges` and `pg_restore --no-owner --no-privileges --exit-on-error` as argument lists, never a shell; heartbeat throughout.
- [ ] **Step 4: Run** the same command. Expected: PASS.
- [ ] **Step 5: Commit** `feat(corpus): build stage with dump-backed local corpus databases`.

### Task 13: Eval stage, CLI and worker wiring

**Files:**
- Create: `apps/corpus/internal/stage/eval/eval.go` with tests, `apps/corpus/internal/cli/eval.go`, `apps/corpus/test/proof/index_test.go`
- Modify: `apps/corpus/internal/workflow/registry.go`, `apps/corpus/internal/run/deps.go`, `apps/corpus/internal/config/config.go` and test, `apps/corpus/internal/cli/{commands,worker,stages}.go`, `apps/corpus/test/proof/harness_test.go`, `docs/runbooks/corpus-local.md`, `docs/architecture.md`

**Interfaces:**
- Consumes: every earlier task.
- Produces: `eval.New(cfg eval.Config) *eval.Stage` (name `eval`); `workflow.Default` registers `discover, fetch, extract, relate, chunk, embed, build, eval`; config `CORPUS_EMBEDDER_URL` (document, default `http://127.0.0.1:8092`), `CORPUS_QUERY_EMBEDDER_URL` (default `http://127.0.0.1:8091`), `CORPUS_BUILDER_DATABASE_URL` (required by stages `build` and `eval`), `CORPUS_PG_RESTORE` (default `pg_restore`), `CORPUS_EVAL_FIXTURES` (default: the embedded `vn` fixtures); CLI `corpus eval <cc> <rel> --out DIR [--previous FILE] [--require-floors]` with exit codes 0, 1, 2 as `evaluation.md`.

- [ ] **Step 1: Write the failing tests.** `TestConfig_EmbedderAndBuilderSettings`, `TestEvalStage_ReportStatusAndGaps`, `TestEvalCLI_ExitCodes`, `TestRegistry_StageOrder`, `TestReplay_Histories` still passes on the recorded `relate` history. Proof `TestProof_IndexStagesAfterIngest` (fake gateway and fake embedder, a small fixture set mapped to the proof corpus through `CORPUS_EVAL_FIXTURES`; a paused run extends through `eval`; the corpus database exists; the report is complete) and `TestProof_LostCorpusDatabaseRestored` (drop the final database, rerun resolve through `corpus start`, the database returns with identical counts). Both under 60 seconds.
- [ ] **Step 2: Run** `make test`, `make test-integration`, `make test-proof`. Expected: FAIL.
- [ ] **Step 3: Implement** the stage (one item `<cc>/<rel>` after a done build; input and recipe hashes as `evaluation.md`), the CLI, config, registry and deps; update the runbook (start both embedders, extend a paused run) and architecture table.
- [ ] **Step 4: Run** the same commands and `make pre-push`. Expected: PASS.
- [ ] **Step 5: Commit** `feat(corpus): eval stage, eval command and index stage wiring`.

### Task 14: Live run and evidence (primary)

**Files:**
- Create: `docs/research/index-baseline-2026-11.md`
- Modify: `docs/research/README.md`, `docs/plans/README.md`, `docs/plans/backlog.md`

- [ ] **Step 1:** Provision the bundle with `make_bundle.py work/models/vi_v2`; start both embedders; run the real-model smoke and the span check over all live extract artifacts (`TestSplit_SpanSubstringsMatchSource` logic through a one-off command).
- [ ] **Step 2:** `corpus start vn 20261109` (appends `chunk`, `embed`, `build`, `eval` to the paused run); watch status; record embed time, build time, RSS, disk, sparse nnz p50/p95/max, dump size.
- [ ] **Step 3:** `corpus eval vn 20261109 --out` a scratch directory; drop and restore the local corpus database through resolve; evaluate again; compare both reports.
- [ ] **Step 4:** Write the research note: coverage (86 of 96 citations have a document; five absent documents), legacy and old product metrics for exact and ANN, ANN loss, evidence characters, provisional threshold, every limitation. Measured values replace estimates in budgets only with a History or note line.
- [ ] **Step 5:** Commit `docs: index baseline results for vn/20261109`; mark this plan landed in the plans README; findings not fixed go to the backlog.
