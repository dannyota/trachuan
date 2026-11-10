# Go Query Embedder Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Routes (`instructions/workflow.md`): Tasks 1 to 4, 6 and 7 and Task 8 Step 3 go to Codex `implementer` on `gpt-6.1-terra`, reviewed by `gpt-6.1-sol`; Task 5 (cgo, native libraries, cancellation) goes to `implementer` with `--model gpt-6.1-sol`, reviewed by `reviewer` with `--model gpt-6-astra`; steps marked (primary) and Task 9 are the primary's (provisioning, live runs, parity evidence, host measurement). The primary verifies every task, runs the checks, owns Git and announces PostgreSQL use to the other session.

**Goal:** Serve query embeddings from a Go service, `apps/queryembed`, running Vietnamese_Embedding_v2 in dynamic INT8 per-channel on ONNX Runtime CPU; prove parity with the Python query mode on the same INT8 file; then delete that mode and rename the Python service to `apps/docembed`.

**Architecture:** A pure-Go HTTP layer (`httpapi`, `admission`, `bundle`) implements the embedder contract over two small interfaces, `Tokenizer` and `Runtime`. The native packages `tokenizer` (`github.com/daulet/tokenizers`) and `onnx` (the owner's ONNX Runtime Go binding) satisfy them only under build tag `ort`; without the tag they are stubs whose constructors fail, so default builds, tests and lint stay pure Go. Startup lives in `internal/app`; `cmd/queryembed/main.go` only wires it.

**Tech Stack:** Go 1.27.3; `github.com/daulet/tokenizers` v1.27.0 with `libtokenizers.a`; `github.com/microsoft/onnxruntime/go` replaced by `github.com/dannyota/onnxruntime/go` v1.28.4 (tag `go/v1.28.4`); ONNX Runtime 1.31.1 shared library; Python 3.14 with uv, `onnxruntime==1.31.1` and `onnx==1.23.2` for provisioning and test data.

**Spec:** [ADR 0011](../adr/0011-embedder-services.md), [ADR 0009](../adr/0009-dense-embedding-recipe.md), [embedder design](../design/embedder.md) ("Query service (Go)", "Model bundle", "Recipe report", "HTTP contract", "Checks without model weights"), [budgets](../design/budgets.md) ("Indexing", "Request limits", "Host memory"), [INT8 study](../research/query-int8-2026-11.md). The spec wins over this plan.

## Scope

|In|Out (later plans)|
|-|-|
|INT8 model in the bundle; Python query mode on INT8 as the parity reference; `apps/queryembed` with its CI job; parity gate; re-pinned query recipe; deletion of the Python query mode; rename to `apps/docembed`; Go service latency and memory on the OCI A1 host|container image, Quadlet unit and host network policy (Plan 11); server integration (Plan 9); metrics endpoint; recipe tuning (Plan 5)|

## Global Constraints

- Model `AITeamVN/Vietnamese_Embedding_v2`, revision `18b44161e041`, resolved `18b44161e041bf1d3a333ab5144b5b7b93f914d2`; 1,024 dimensions; CLS (position 0 of the first output), then L2 in float32; raw input, no prefix; query cap 256 tokens including special tokens; reject above the cap, never truncate.
- Query model file `onnx/model_int8.onnx`, made by `onnxruntime.quantization.quantize_dynamic(model_input="onnx/model.onnx", model_output="onnx/model_int8.onnx", weight_type=QuantType.QInt8, per_channel=True)` at ONNX Runtime 1.31.1, one file without external data, listed in `bundle.json` like every other file.
- Recipe report fields and order exactly as `embedder.md` "Recipe report"; query values `runtime:"query"`, `backend:"onnxruntime-cpu"`, `precision:"int8"`, `token_cap:256`; Go values `language:"go <version>"`, `tokenizer_library:"daulet/tokenizers <version>"`, `runtime_library:"onnxruntime <version>"`. `weights_sha256` covers only `onnx/model_int8.onnx`; `tokenizer_sha256` covers the files whose path contains `token` or ends with `sentencepiece.bpe.model`. Manifest hash bytes: the selected `bundle.json` entries in bundle order as a compact JSON array of objects with keys `path`, `sha256`, `size` in that order, no HTML escaping (equal to Python `json.dumps(items, ensure_ascii=False, separators=(",", ":"), sort_keys=True)`).
- HTTP contract, error codes and statuses exactly as `embedder.md` "HTTP contract"; query service accepts exactly one text; body cap 65,536 bytes enforced while reading; admission 2 active, 8 queued, 250 ms maximum queue wait, 2 s total request deadline; a timed-out request keeps its active slot until `Run` returns.
- ONNX session: CPU provider, 4 intra-op threads, 1 inter-op thread, sequential execution, graph optimization level all, CPU memory arena disabled; inputs exactly int64 `input_ids` and `attention_mask`, first output float32 `[batch, sequence, 1024]`.
- Vector validation: finite float32, absolute L2 norm error at most 0.001.
- Flags `--model-dir DIR --ort-lib FILE --listen ADDR`; `--listen` defaults to `127.0.0.1:8091`, the port the corpus config already uses.
- cgo only in `apps/queryembed/internal/tokenizer` and `apps/queryembed/internal/onnx`, only under tag `ort`. Native assets pinned in `apps/queryembed/native.lock` for `darwin-arm64` and `linux-arm64` by version, URL and SHA-256; nothing downloads at runtime.
- Never log, return or store input text or vectors; error messages are fixed text. Go files at most 400 lines, one concern each, doc comments on exported symbols, errors wrapped with `%w`; unit tests never wait on real time beyond 100 ms.

## Review Focus

- Go and Python token ids differ on real Vietnamese input (NFD marks, zero-width characters, tabs and newlines, document numbers), silently shifting vectors: Task 8 parity covers a fixed edge-text list beside every evaluation question.
- A request past its deadline returns 504 while native inference continues, and a new request starts on top of it: Task 2 `TestDo_DeadlineReturnsButSlotHeldUntilWorkEnds`; Task 5 `TestModel_CancelledContextTerminatesRun`.
- Concurrent requests reach the cgo tokenizer at once: Task 5 `TestTokenizer_ConcurrentEncodeIsSerialized` under `-race`.
- Strict JSON gaps: a duplicate key inside a nested object, invalid UTF-8 inside a string, a body exactly at the cap: Task 4 `TestPost_DuplicateKeyNested400`, `TestPost_InvalidUTF8_400`, `TestPost_BodyAtCapAccepted`.
- A bundle entry that escapes the directory through `..`, an absolute path or a symlink: Task 3 `TestVerify_RejectsTraversalAbsoluteDuplicateAndSymlink`.

---

### Task 1: INT8 model in the bundle and the Python INT8 reference

**Files:**
- Modify: `apps/embedder/scripts/make_bundle.py`, `apps/embedder/src/trachuan_embedder/{recipe.py,__main__.py}`, `apps/embedder/tests/{test_recipe.py,test_main.py}`, `internal/embedding/recipes/vi-v2-query.json`, `docs/runbooks/corpus-local.md` (provisioning line)
- Create: `apps/embedder/tests/test_make_bundle.py`

**Interfaces:**
- Produces: bundle file `onnx/model_int8.onnx`; Python query recipe with `precision:"int8"` and `weights_sha256` over that file; re-pinned `embedding.QueryRecipe1()` (Python INT8) used by every run started after this task.

- [ ] **Step 1: Write the failing tests.** `test_make_bundle_quantizes_before_manifest` (a tiny MatMul graph built with `onnx.helper` as `onnx/model.onnx` in `tmp_path`; after `make_bundle` the directory has `onnx/model_int8.onnx`, `bundle.json` lists it with size and hash, and `verify_bundle` passes); `test_query_recipe_int8_weights` (`precision == "int8"`; `weights_sha256` equals the manifest hash of exactly the `onnx/model_int8.onnx` entry; changing `onnx/model.onnx`, `onnx/model.onnx_data` or `model.safetensors` leaves the query recipe unchanged); `test_query_runtime_opens_int8_model` (`__main__` passes `onnx/model_int8.onnx` to `OnnxRuntime`).
- [ ] **Step 2: Run** `make embedder-test`. Expected: FAIL.
- [ ] **Step 3: Implement.** `make_bundle.py` quantizes with the call in Global Constraints before writing the manifest, and refuses when `onnx/model.onnx` is missing; the query recipe selects `onnx/model_int8.onnx` and reports `int8`; the document recipe is unchanged.
- [ ] **Step 4: Run** `make embedder-test` and `make pre-push`. Expected: PASS.
- [ ] **Step 5 (primary):** run `make_bundle.py` on `work/models/vi_v2-bundle`; start `make embedder-query`; re-pin `vi-v2-query.json` from `GET /v1/recipe`; smoke one Vietnamese query (token count, norm); record INT8 against FP32 cosine for the legacy questions (median near 0.987 on the Mac); `make test`; commit `feat(embedder): INT8 per-channel query model in the bundle`. Tell the other session that new runs now freeze the Python INT8 query recipe.

### Task 2: Admission

**Files:**
- Create: `apps/queryembed/internal/admission/admission.go`, `admission_test.go`

**Interfaces:**
- Produces: `type Limits struct{ Active, Queued int; Wait, Deadline time.Duration }`; `func QueryLimits() Limits` (2, 8, 250 ms, 2 s); `func New(l Limits) *Admission`; `func (a *Admission) Do(ctx context.Context, work func(context.Context) error) error`; `var ErrBusy, ErrDeadline error`.

- [ ] **Step 1: Write the failing tests** with millisecond limits: `TestQueryLimits_Values`; `TestDo_BusyWhenActiveAndQueueFull` (the request after `Active+Queued` admitted ones returns `ErrBusy` at once); `TestDo_BusyAfterQueueWait`; `TestDo_DeadlineReturnsButSlotHeldUntilWorkEnds` (`Do` returns `ErrDeadline` at the deadline; a new request cannot take that slot until `work` returns); `TestDo_WorkContextCancelledAtDeadline`; `TestDo_CancelledWhileQueuedFreesQueueSlot`; `TestDo_WorkErrorReturned`.
- [ ] **Step 2: Run** `go test -race ./apps/queryembed/internal/admission/`. Expected: FAIL.
- [ ] **Step 3: Implement.** The deadline covers queue wait and work; `work` runs in its own goroutine and releases its slot when it returns; no background queue beyond `Queued`.
- [ ] **Step 4: Run** the same command. Expected: PASS.
- [ ] **Step 5: Commit** `feat(queryembed): request admission`.

### Task 3: Bundle verification and query recipe

**Files:**
- Create: `apps/queryembed/internal/bundle/{bundle.go,recipe.go}` with tests, `apps/queryembed/internal/bundle/testdata/` (small fixture bundle)

**Interfaces:**
- Produces: `type File struct{ Path string; Size int64; SHA256 string }` (JSON `path`, `size`, `sha256`); `type Manifest struct{ Model, Revision, ResolvedRevision string; Files []File }`; `func Load(dir string) (Manifest, error)`; `func (m Manifest) Verify(dir string, paths []string) error`; `func TokenizerFiles(m Manifest) []File`; `const QueryModelPath = "onnx/model_int8.onnx"`; `type Versions struct{ Go, Tokenizers, ONNXRuntime string }` (bare versions such as `1.27.3`); `func QueryRecipe(m Manifest, v Versions) (embedding.Recipe, error)`.

- [ ] **Step 1: Write the failing tests.** `TestLoad_RejectsUnknownFieldsAndBadRevision` (resolved revision not 40 hex characters or not prefixed by the revision); `TestVerify_SizeHashAndMissingFile`; `TestVerify_RejectsTraversalAbsoluteDuplicateAndSymlink`; `TestVerify_IgnoresListedFilesNotLoaded` (a missing `model.safetensors` listed in `bundle.json` does not fail when not requested, so a host bundle may carry only query files); `TestQueryRecipe_HashesMatchPython` (expected `tokenizer_sha256`, `weights_sha256` and recipe hash for the fixture, computed with `apps/embedder` `manifest_hash` logic and pinned as constants, with the command in a comment); `TestQueryRecipe_FieldsAndVersions` (`Versions{"1.27.3","1.27.0","1.31.1"}` gives `go 1.27.3`, `daulet/tokenizers 1.27.0`, `onnxruntime 1.31.1`; values from Global Constraints); `TestQueryRecipe_RefusesWrongModelOrMissingFiles`.
- [ ] **Step 2: Run** `go test ./apps/queryembed/internal/bundle/`. Expected: FAIL.
- [ ] **Step 3: Implement** with strict JSON decoding; `Verify` hashes only the requested files, which must be listed, regular and inside `dir`; the recipe uses the manifest hash bytes from Global Constraints and `embedding.Recipe`.
- [ ] **Step 4: Run** the same command. Expected: PASS.
- [ ] **Step 5: Commit** `feat(queryembed): bundle verification and query recipe`.

### Task 4: HTTP contract

**Files:**
- Create: `apps/queryembed/internal/httpapi/{server.go,decode.go,errors.go,vector.go}` with tests

**Interfaces:**
- Consumes: Task 2 `admission`; `embedding.Recipe`.
- Produces: `type Tokenizer interface{ Encode(text string) ([]int64, error) }`; `type Runtime interface{ Embed(ctx context.Context, ids []int64) ([]float32, error) }`; `type Config struct{ Tokenizer Tokenizer; Runtime Runtime; Recipe embedding.Recipe; Admission *admission.Admission; Logger *slog.Logger }`; `func New(cfg Config) *Server`; `func (s *Server) Handler() http.Handler`; `func (s *Server) SetReady(ready bool)`; `const BodyLimit = 65536`.

- [ ] **Step 1: Write the failing tests** with a fake tokenizer (one id per space-separated word plus two special ids) and fake runtimes: `TestHealthz_AliveWhileLoading`; `TestReadyzAndRecipe_503UntilReady`; `TestRecipe_ReportShape` (`protocol_version:1`, `recipe_hash`, `recipe`); `TestTokenize_CountsWithoutTruncation` (300 words give 302); `TestEmbed_ReturnsVectorAndTokenCount` (exact response shape of `embedder.md`); `TestEmbed_ExactlyAtCapAccepted` (256); `TestEmbed_OverCap422WithIndexAndCount` (257, runtime not called); `TestPost_ExactlyOneText400`; `TestPost_EmptyOrWhitespaceText400`; `TestPost_UnknownField400`; `TestPost_DuplicateKeyNested400`; `TestPost_InvalidUTF8_400`; `TestPost_BodyOverCap413BeforeParse` (65,537 bytes of invalid JSON give 413); `TestPost_BodyAtCapAccepted`; `TestPost_WrongMediaType415`; `TestPost_RecipeMismatch409`; `TestEmbed_InvalidVector500` (wrong dimension, NaN, norm off by more than 0.001); `TestEmbed_Busy429`; `TestEmbed_Deadline504`; `TestErrors_FixedMessages` (no submitted text in any body); `TestLogs_NoInputText`.
- [ ] **Step 2: Run** `go test -race ./apps/queryembed/internal/httpapi/`. Expected: FAIL.
- [ ] **Step 3: Implement.** POST order: readiness (503), media type `application/json` with optional charset (415), capped read (413), UTF-8 (400), duplicate keys by token walk (400), strict decode of exactly `recipe_hash` and `texts` (400), one nonblank text (400), recipe hash (409), then admission around tokenize, cap check (422), runtime and validation (500), serialization. One log line per request: route, status, code, token count, latency.
- [ ] **Step 4: Run** the same command. Expected: PASS.
- [ ] **Step 5: Commit** `feat(queryembed): HTTP contract`.

### Task 5: Native tokenizer and ONNX runtime

**Files:**
- Create: `apps/queryembed/native.lock`, `scripts/queryembed-native.sh`, `apps/queryembed/internal/tokenizer/{tokenizer.go,stub.go,tokenizer_test.go}`, `apps/queryembed/internal/onnx/{model.go,stub.go,model_test.go}`, `apps/queryembed/testdata/{make_testdata.py,tiny.onnx,tokenizer.json}`
- Modify: `go.mod`, `go.sum`, `Makefile` (`queryembed-native`, `queryembed-test-ort`), `.github/workflows/ci.yml`

**Interfaces:**
- Consumes: Task 4 `httpapi.Tokenizer` and `httpapi.Runtime`.
- Produces: `func tokenizer.Open(path string) (*Tokenizer, error)`, `(*Tokenizer) Encode(text string) ([]int64, error)`, `Close() error`, `func tokenizer.Version() string`; `type onnx.Options struct{ LibraryPath string; IntraOp, InterOp int }`, `func onnx.QueryOptions(lib string) Options` (4, 1), `func onnx.Open(modelPath string, o Options) (*Model, error)`, `(*Model) Embed(ctx context.Context, ids []int64) ([]float32, error)`, `Dimensions() int`, `Close() error`, `func onnx.Version() (string, error)`; `var ErrNativeUnavailable` returned by every stub constructor.

- [ ] **Step 0 (primary, before dispatch):** `go get github.com/daulet/tokenizers@v1.27.0`; add `replace github.com/microsoft/onnxruntime/go => github.com/dannyota/onnxruntime/go v1.28.4` and `go get` it; write `native.lock` with the ONNX Runtime 1.31.1 assets `onnxruntime-osx-arm64-1.31.1.tgz` and `onnxruntime-linux-aarch64-1.31.1.tgz` and the tokenizers v1.27.0 assets `libtokenizers.darwin-arm64.tar.gz` and `libtokenizers.linux-arm64.tar.gz`, each with the SHA-256 computed from the downloaded file and the archive member to install; run `make queryembed-native` so `work/native/darwin-arm64/` holds `libonnxruntime.dylib` and `libtokenizers.a`.
- [ ] **Step 1: Write the failing tests** (tag `ort`, real native libraries, committed test data): `TestTokenizer_SpecialTokensAndNoTruncation` (an input longer than any cap keeps every id); `TestTokenizer_ConcurrentEncodeIsSerialized` (-race); `TestModel_RefusesWrongInputsOrOutput`; `TestModel_CLSNormalized` (known vector for a fixed id list); `TestModel_RejectsNonFiniteOutput`; `TestModel_CancelledContextTerminatesRun`; `TestModel_ThreadAndArenaOptions` (session options read back where the binding allows). Untagged: `TestStubs_ReturnErrNativeUnavailable`.
- [ ] **Step 2: Run** `make queryembed-test-ort` and `go test ./apps/queryembed/...`. Expected: FAIL.
- [ ] **Step 3: Implement.** `make_testdata.py` (run with `uv run --project apps/embedder`) builds a vocabulary-8 Unigram or WordLevel `tokenizer.json` with `<s>`/`</s>` template processing and a graph with the production input names whose output is an embedding lookup of shape `[batch, sequence, 8]` scaled by the attention mask. The tokenizer holds one mutex around every call. The model sets the session options from Global Constraints, initializes the environment once per process from `LibraryPath`, feeds an all-ones mask, and on context cancellation calls `SetTerminate` on that run's options. `scripts/queryembed-native.sh <platform>` downloads, checks SHA-256, extracts the named member and installs it, offline when the cache already matches. CI: preflight output `queryembed` (true for `apps/queryembed/**`, `internal/embedding/**`, `go.mod`, `go.sum`, the script, the lock or the workflow); job `queryembed` on `ubuntu-24.04-arm` with the same pinned actions as `static`, running the script for `linux-arm64`, `go test -race -tags ort ./apps/queryembed/...` and `golangci-lint run --build-tags ort ./apps/queryembed/...`; the `ci` aggregate requires it when it ran.
- [ ] **Step 4: Run** the Step 2 commands, `make test` and `make pre-push`. Expected: PASS; CI green on the pushed head.
- [ ] **Step 5: Commit** `feat(queryembed): native tokenizer and ONNX runtime behind the ort tag`.

### Task 6: Startup and command

**Files:**
- Create: `apps/queryembed/internal/app/{app.go,startup.go}` with tests, `apps/queryembed/cmd/queryembed/main.go`
- Modify: `Makefile` (`queryembed-run`), `docs/runbooks/corpus-local.md`, `docs/architecture.md`

**Interfaces:**
- Consumes: Tasks 2 to 5.
- Produces: `func app.Run(ctx context.Context, args []string, stderr io.Writer) int`; `type app.Natives struct{ OpenTokenizer func(path string) (httpapi.Tokenizer, io.Closer, error); OpenModel func(path string, o onnx.Options) (httpapi.Runtime, int, io.Closer, error); Versions func() (bundle.Versions, error) }` injected for tests.

- [ ] **Step 1: Write the failing tests** with fake natives: `TestRun_StartupOrder` (verify bundle, open tokenizer, count a fixed synthetic text of 300 repeated words and require more than 256 ids, open model, require 1,024 dimensions, warm up, then ready); `TestRun_RefusesTruncatingTokenizer`; `TestRun_RefusesBundleMismatch`; `TestRun_ReadyOnlyAfterWarmup`; `TestRun_DrainOnSignal` (readiness turns 503, in-flight requests finish, exit 0); `TestRun_DefaultListenLoopback`.
- [ ] **Step 2: Run** `go test ./apps/queryembed/...`. Expected: FAIL.
- [ ] **Step 3: Implement**; `main.go` only builds `Natives` from the `ort` packages and calls `app.Run` under `signal.NotifyContext`.
- [ ] **Step 4: Run** the same command, `make queryembed-test-ort`, `make pre-push`. Then (primary) `make queryembed-run` on the real bundle, `GET /v1/recipe`, one Vietnamese smoke query (token count and norm), RSS after warmup. Expected: PASS; the reported recipe differs from the Python INT8 one only in `language`, `tokenizer_library` and `runtime_library`.
- [ ] **Step 5: Commit** `feat(queryembed): startup and command`; update the runbook (start, flags, native assets) and the architecture table in the same commit.

### Task 7: Parity harness

**Files:**
- Create: `apps/queryembed/test/parity/{parity_test.go,edge_texts.json}`, `apps/embedder/scripts/token_ids.py`

**Interfaces:**
- Consumes: Task 5 `tokenizer.Open`; two running services; `apps/corpus/internal/evaluation/testdata/vn/legacy.json` read by path.
- Produces: `token_ids.py LEGACY EDGE --model-dir DIR --out FILE` writes `{"ids":[[int]]}` in text order (legacy questions, then edge texts) with the Python tokenizer, `add_special_tokens=True`, no truncation.

- [ ] **Step 1: Write** `TestParity_GoMatchesPython` (tags `ort parity`; environment `QUERYEMBED_PARITY_GO`, `QUERYEMBED_PARITY_PY` (service URLs), `QUERYEMBED_PARITY_PY_IDS` (the script's output), `QUERYEMBED_PARITY_MODEL_DIR`; skip when unset). Texts: every legacy question plus `edge_texts.json` (NFC and NFD Vietnamese, zero-width space and joiner, tab and newline, document numbers such as `50/2024/TT-NHNN`, digits and punctuation, emoji, CJK, a text of exactly 256 tokens and one of 257). Checks: Go token ids from `tokenizer.Open` on the bundle's `tokenizer.json` equal the Python ids for every text; identical `/v1/tokenize` counts and identical 422 decisions from both services; cosine at least 0.9999 between the two services' vectors for every accepted text. Output only text indexes, counts and the minimum cosine, never text or ids.
- [ ] **Step 2: Run** `go vet -tags 'ort parity' ./apps/queryembed/test/parity/` and `uv run --project apps/embedder ruff check scripts/token_ids.py`. Expected: PASS (the test compiles and skips without its variables).
- [ ] **Step 3: Commit** `test(queryembed): parity harness against the Python query mode`.

### Task 8: Parity gate, re-pin and switch-over (primary, then one implementer brief)

**Files:**
- Modify: `internal/embedding/recipes/vi-v2-query.json`, `docs/adr/0011-embedder-services.md` (History), `docs/design/budgets.md` (remove the transitional Python query paragraph), `docs/runbooks/corpus-local.md`, `AGENTS.md`, `instructions/python.md`, `Makefile`, `scripts/pre-push.sh`, `.github/workflows/ci.yml`
- Rename: `apps/embedder` to `apps/docembed`, package `trachuan_embedder` to `trachuan_docembed`
- Delete: `apps/docembed/src/trachuan_docembed/onnx_runtime.py`, the query branches of `__main__.py`, `recipe.py` and `app.py`, and their tests

- [ ] **Step 1 (primary):** start both query services on the same bundle; write the Python ids with `token_ids.py`; run `go test -tags 'ort parity' -count=1 ./apps/queryembed/test/parity/` with the four variables. Expected: PASS; record the counts and the minimum cosine.
- [ ] **Step 2 (primary, needs a corpus database; announce PostgreSQL use):** take run A, the first run whose frozen query recipe is the Python INT8 one (the scope run if it starts after Task 1), evaluated with the Python service. Re-pin `vi-v2-query.json` from the Go service, start run B from the same source list with the Go service, and evaluate it. Compare `cases.jsonl` of A and B for every cohort, mode, arm and limit: ordered `returned` identities and `abstain` must be identical. Precondition: equal source manifest hashes; if VBPL changed between the runs, list the changed documents, compare only cases whose expected and returned documents are outside that list, and record the excluded cases.
- [ ] **Step 3 (implementer):** delete the Python query mode and rename the service as listed; `make docembed-test`, `make docembed-run` and `make queryembed-run` replace the old embedder targets; the CI job and pre-push run ruff and pytest in `apps/docembed`. Tests: the document service refuses `--runtime query` and starts without it; the corpus config default `CORPUS_QUERY_EMBEDDER_URL` still points at `127.0.0.1:8091`.
- [ ] **Step 4:** `make docembed-test`, `make test`, `make pre-push`, CI green; ADR 0011 History records the gate evidence and the deletion. Commit `feat: Go query service replaces the Python query mode`.

### Task 9: Host latency and memory (primary, needs the OCI A1 host)

**Files:**
- Modify: `docs/design/budgets.md` (Host memory note), `docs/research/query-int8-2026-11.md` (Go service section)

- [ ] **Step 1:** build `linux-arm64` with tag `ort`, copy the binary, the pinned native library and a bundle holding only the query files with the full `bundle.json`; start it on the host.
- [ ] **Step 2:** measure through HTTP, one request at a time: short-query p50 and p95 over the legacy questions, and p50 for the 217-token long input; then two concurrent requests at 2 requests per second for 30 minutes, recording p95, p99 and RSS at the end. Compare with the direct ONNX Runtime numbers (65 ms short, 387 ms long, 0.83 GB) and the 1.2 GB RSS gate.
- [ ] **Step 3:** record the results with a note line in budgets and the research note; findings not fixed go to the backlog. Commit `docs: Go query service latency and memory on the A1 host`; mark this plan landed in `docs/plans/README.md`.
