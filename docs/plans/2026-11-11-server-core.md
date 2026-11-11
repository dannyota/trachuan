# Server Core and MCP Read Path Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Routes (`instructions/workflow.md`): Tasks 1, 4 and 6 go to Codex `implementer` on `gpt-6.1-terra`, reviewed by `gpt-6.1-sol`; Tasks 2, 3 and 5 (credentials, release resolution, the MCP surface a hostile client reaches) go to `implementer` with `--model gpt-6.1-sol`, reviewed by `reviewer` with `--model gpt-6-astra`, Task 2 and the final review with an adversarial pass. Task 7 is the executing session's. Executor: trachuan-dev2; coordinator: the trachuan session, which merges.

**Goal:** A Go server, `apps/server`, that serves the MCP read path behind personal bearer tokens: `guide`, `corpus_status`, `search` (compact or full, ADR 0013), `read` and `quality_gaps`, over the active or a retained corpus release.

**Architecture:** `apps/server` owns the auth database (goose migrations, sqlc store), a credential middleware shaped for the later OAuth tokens, a release resolver that opens one `retrieval.Engine` per corpus database, in-memory per-account limits, and an MCP Streamable HTTP endpoint built on the official Go SDK. Retrieval, presentation and reading stay in `internal/retrieval`. Web login, the web API and OAuth for MCP are later plans (9b, 9c).

**Tech Stack:** Go 1.27.3; `github.com/modelcontextprotocol/go-sdk` v1.8.0 (the version aboutme's `mcpapi` uses); pgx v5; goose v3; sqlc (as `apps/corpus`); PostgreSQL 18; integration tests under tag `integration` with `internal/testpg`.

**Spec:** [system](../design/system.md), [auth](../design/auth.md) ("Token checks and revocation", "Account lifecycle"), [data](../design/data.md) ("Auth database"), [retrieval](../design/retrieval.md) ("Release pinning", "MCP tools"), [search results](../design/mcp-results.md), [deployment](../design/deployment.md) ("Serving bundle and release activation"), [budgets](../design/budgets.md) ("Request limits", "Retrieval"), ADR 0005, ADR 0013. The spec wins over this plan.

## Scope

|In|Out (later plans)|
|-|-|
|`apps/server` command, config, health; auth migrations and store (`account`, `personal_token`, `corpus_release`, `retired_release`, `release_journal`); personal bearer tokens and the admin CLI that issues them; credential middleware; release resolver and engine cache; `retrieval.OpenForRead` for retained releases; `Engine.Status` and `Engine.Gaps`; per-account limits; MCP tools `guide`, `corpus_status`, `search`, `read`, `quality_gaps`; runbook|Google login, web sessions, CSRF, `/api/v1`, OpenAPI (9b); OAuth AS, CIMD, RFC 9728 metadata, ChatGPT and Claude.ai acceptance (9c); `taxonomy` tool, `nodes[]`, `in_filter` (Plan 6); `doc_types[]`, `issuers[]`, `documents[]` filters and search `cursor` (not in the engine yet; a later retrieval plan); release activation commands and the image (Plan 11)|

## Global Constraints

- Limits from budgets: `search` 60 per minute and `read` 120 per minute per account (all other tools share the `read` bucket); request body 64 KB; credential lookup cache 10 s, never beyond the credential's expiry; personal token lifetime default 90 days, at most 1 year; serving statement deadline 2 s (engine profile `serving`); retained releases 7 days.
- Credentials: 32 random bytes, encoded base64url without padding, prefixed `trc_`; stored only as SHA-256 hex; compared by hash lookup. A personal token records the account's `auth_epoch` at issue and is valid only while it equals the account's current value.
- Every `/mcp` request checks: credential hash exists, `revoked_at` null, `expires_at` in the future, account not blocked, epoch equal, audience the canonical `/mcp` URL and scope `mcp:read` (implicit for personal tokens). Auth database unavailable: 503, fail closed. A missing or invalid token: 401 with `WWW-Authenticate: Bearer`.
- Release rules: the active release per country is `auth.corpus_release.active_db`; a retained release is in `retired_release` with `dropped_at` null and `retain_until` in the future; anything else named by `release` is `release_expired`. `search` on a retained release whose query or lexical recipe differs from the running embedder returns `release_incompatible`; `read` still works.
- The engine's query embedder client pins the recipe hash from the release's dense recipe (query side), never the embedded recipe file.
- MCP results follow `mcp-results.md` "Format": `structuredContent` plus the same compact JSON in one text block; each tool declares an `outputSchema`; tool errors are results with `isError` and a JSON body `{error:{code,message}}` with fixed messages. Codes: `invalid_request`, `rate_limited`, `release_expired`, `release_incompatible`, `busy`, `unavailable`, `inference_failed`.
- Never log, store or return query text, returned text or tokens; logs carry account id, tool, code and latency. Go files at most 400 lines, one concern each; errors wrapped with `%w`; no package-level mutable state; regex only for real pattern grammars.

## Review Focus

- A token revoked or an account blocked by the admin CLI (another process) keeps working for at most the 10 s cache window, never longer and never past `expires_at`: Task 2 `TestMiddleware_CacheNeverOutlivesExpiryOrWindow`.
- Two first requests for the same release race to open two engines and pools: Task 3 `TestResolver_ConcurrentFirstUseOpensOnce`.
- A JSON-RPC batch, an oversized body or a huge `items` array must fail before any database or embedder work: Task 5 `TestMCP_BodyCapAndBatchRejected`, and `read` keeps `ReadItemsMax`.
- A retained release that was pruned between resolve and query must return `release_expired`, not a server error: Task 3 `TestResolver_PrunedRetainedIsExpired`.
- An error path that formats the query or a token into a message or log line: Task 5 `TestMCP_NoQueryOrTokenInLogsOrErrors` with a capturing logger.

---

### Task 1: Auth schema, store and migrations

**Files:** Create `apps/server/migrations/00001_auth.sql`, `apps/server/migrations/embed.go`, `apps/server/sql/*.sql`, `apps/server/sqlc.yaml`, `apps/server/internal/store/` (generated), `apps/server/internal/migrate/migrate.go` with tests. Modify `Makefile` (`generate` and `sqlc-check` cover `apps/server`).

**Interfaces:** Produces `migrate.Up(ctx, url string) error`; store queries `CreateAccount`, `GetAccount`, `BlockAccount` (sets `blocked_at`, increments `auth_epoch`, revokes tokens), `CreatePersonalToken`, `GetCredential` (token row joined to account: account id, `blocked_at`, account `auth_epoch`, token `auth_epoch`, `expires_at`, `revoked_at`), `RevokePersonalToken`, `TouchPersonalToken` (`last_used_at`, at most once per minute), `GetActiveRelease(country)`, `ListRetainedReleases(country)`.

- [ ] **Step 1: Failing tests** (integration): `TestMigrate_UpTwiceIsNoop`; `TestStore_CredentialJoin` (blocked and epoch fields present); `TestStore_BlockRevokesTokensAndBumpsEpoch`; `TestStore_ReleaseQueries` (active, retained within window, dropped and expired excluded).
- [ ] **Step 2: Implement** the tables of `data.md` "Auth database" for `account` (with `auth_epoch integer NOT NULL DEFAULT 0`), `personal_token` (plus `auth_epoch`), `corpus_release` (primary key `country`), `retired_release`, `release_journal`; checks for hash format (`^[0-9a-f]{64}$`) and `expires_at <= created_at + interval '1 year'`. No question text columns.
- [ ] **Step 3: Run** `make generate`, `make sqlc-check`, `go test -tags integration ./apps/server/...`; commit `feat(server): auth schema, store and migrations`.

### Task 2: Personal tokens and the credential middleware

**Files:** Create `apps/server/internal/credential/{token.go,verify.go,cache.go,middleware.go}` with tests, `apps/server/internal/admin/{account.go,token.go}` with tests.

**Interfaces:** Consumes Task 1 store. Produces `credential.Issue(rand io.Reader) (plain, hash string, err error)`; `type Principal struct{ AccountID int64; Scope string; Audience string }`; `credential.Verifier{Store; Clock func() time.Time; CacheTTL time.Duration; Audience string}` with `Verify(ctx, bearer string) (Principal, error)` and errors `ErrUnauthorized`, `ErrUnavailable`; `credential.Middleware(v *Verifier, next http.Handler) http.Handler` (401 with `WWW-Authenticate: Bearer error="invalid_token"`, 503 on `ErrUnavailable`, principal in context); `credential.FromContext(ctx) (Principal, bool)`; admin functions `CreateAccount(ctx, googleSub, email)`, `CreateToken(ctx, accountID, name, lifetime) (plain string, err error)` (default 90 days, refuse above 365 days), `RevokeToken`, `BlockAccount`.

- [ ] **Step 1: Failing tests:** `TestIssue_FormatAndHash` (prefix, 43 base64url chars, hash is SHA-256 hex of the full string); `TestVerify_RejectsRevokedExpiredBlockedOldEpochUnknown`; `TestVerify_AuthDownFailsClosed`; `TestMiddleware_CacheNeverOutlivesExpiryOrWindow` (fake clock: cached entry dropped at TTL and at `expires_at`, whichever is first); `TestMiddleware_HeadersAndStatus`; `TestMiddleware_NoTokenInLogs`; `TestAdmin_TokenLifetimeBounds`.
- [ ] **Step 2: Implement**; cache keyed by hash with a bounded size (evict oldest); constant-time comparison is unnecessary because lookup is by hash, but never log the bearer value or its hash.
- [ ] **Step 3: Run** unit and integration tests; commit `feat(server): personal tokens and credential middleware`.

### Task 3: Release resolver and engine cache

**Files:** Create `apps/server/internal/release/{resolver.go,cache.go}` with tests. Modify `internal/retrieval/engine.go` (add `OpenForRead`), `internal/retrieval/status.go` and `internal/retrieval/gaps.go` (new) with tests.

**Interfaces:** Produces `retrieval.OpenForRead(ctx, db *pgxpool.Pool, options Options) (*Engine, error)` (validates the release row and schema like `Open` but needs no embedder; its `Search` returns `ErrSearchUnavailable`); `Engine.Status(ctx) (Status, error)` (release, document and provision counts, gap counts by kind, newest source date); `Engine.Gaps(ctx, kind string, after int64, limit int) ([]Gap, next int64, error)` (page size 200 from budgets); `release.Resolver{Auth store; Open func(dbName string) (*retrieval.Engine, error); OpenRead func(dbName string) (*retrieval.Engine, error); Clock}` with `Resolve(ctx, country, rel string, forSearch bool) (*retrieval.Engine, string, error)` and errors `ErrExpired`, `ErrIncompatible`.

- [ ] **Step 1: Failing tests:** `TestResolver_ActiveByDefault`; `TestResolver_RetainedWithinWindow`; `TestResolver_PrunedRetainedIsExpired`; `TestResolver_UnknownIsExpired`; `TestResolver_IncompatibleSearchButReadable` (open fails with an embedder recipe mismatch: search gets `ErrIncompatible`, read gets an engine from `OpenForRead`); `TestResolver_ConcurrentFirstUseOpensOnce` (single flight per database name); `TestOpenForRead_NoEmbedder`; `TestStatus_Counts`; `TestGaps_Paged`.
- [ ] **Step 2: Implement**; one `pgxpool` per corpus database (max 4 connections), opened lazily from a URL template whose database name is substituted only from `corpus_release`/`retired_release` values matching `^corpus_[a-z]{2}_[0-9]{8}(_[0-9]+)?$`; pools for databases no longer active or retained close on the next resolve.
- [ ] **Step 3: Run** unit and integration tests for both packages; commit `feat(server): release resolver and engine cache`.

### Task 4: Per-account limits

**Files:** Create `apps/server/internal/limits/limits.go` with tests.

**Interfaces:** Produces `limits.New(clock func() time.Time) *Limiter`; `(*Limiter) Allow(accountID int64, class Class) (ok bool, retryAfter time.Duration)`; `Class` values `Search` (60 per minute) and `Read` (120 per minute).

- [ ] **Step 1: Failing tests:** `TestAllow_SearchSixtyPerMinute`; `TestAllow_ReadBucketSeparate`; `TestAllow_RefillOverTime`; `TestAllow_ConcurrentSafe` (`-race`); `TestAllow_IdleAccountsEvicted` (bounded memory).
- [ ] **Step 2: Implement** token buckets in memory (one host, one process; counts reset on restart, which the design accepts); commit `feat(server): per-account limits`.

### Task 5: MCP tools

**Files:** Create `apps/server/internal/mcpapi/{server.go,guide.go,search.go,read.go,status.go,errors.go}` with tests.

**Interfaces:** Consumes Tasks 2 to 4 and `retrieval.Present`, `Read`, `DefaultLimit`, `MaxLimit`, `ReadItemsMax`. Produces `mcpapi.New(cfg Config) http.Handler` (Streamable HTTP, go-sdk v1.8.0, stateless JSON responses), `Config{Resolver; Limiter; Logger; Clock}`.

- [ ] **Step 1: Failing tests** with the go-sdk client against `httptest`: `TestMCP_ToolsListedWithSchemas` (five tools, input and output schemas); `TestMCP_SearchCompactDefault` (default limit 20, JSON equals `structuredContent`, text block holds the same compact JSON); `TestMCP_SearchFullDetail`; `TestMCP_SearchRejectsUnknownFieldsAndBadLimit` (above 50, `nodes` until Plan 6); `TestMCP_ReadBatchAndNext`; `TestMCP_ReleaseExpiredAndIncompatible`; `TestMCP_RateLimited`; `TestMCP_BodyCapAndBatchRejected`; `TestMCP_NoQueryOrTokenInLogsOrErrors`; `TestGuide_ContentRules` (states the flow and rules of `mcp-results.md` "Guide", the Vietnamese-with-diacritics rule and the citation format).
- [ ] **Step 2: Implement**; inputs per `retrieval.md` "MCP tools" restricted to the Scope (search: `country`, `query`, `as_of`, `in_force_only`, `detail`, `limit`, `release`); unknown fields are `invalid_request`; `country` must be `vn`; validation and diacritics errors keep the engine's fixed messages; `corpus_status` and `quality_gaps` use Task 3.
- [ ] **Step 3: Run** unit and integration tests; commit `feat(server): MCP tools on the read path`.

### Task 6: Command, config and runbook

**Files:** Create `apps/server/cmd/server/main.go`, `apps/server/internal/app/{app.go,config.go}` with tests, `docs/runbooks/server-local.md`. Modify `Makefile` (`server-run`), `docs/architecture.md`, `.github/workflows/ci.yml` (Go jobs cover `apps/server`).

**Interfaces:** Produces `app.Run(ctx, args []string, stderr io.Writer) int` with subcommands `serve`, `migrate`, `admin account create --google-sub --email`, `admin token create --account --name [--days]`, `admin token revoke`, `admin account block`. Environment: `SERVER_LISTEN` (default `127.0.0.1:8080`), `SERVER_AUTH_DATABASE_URL`, `SERVER_CORPUS_DATABASE_URL` (template; database name supplied by the resolver), `SERVER_QUERY_EMBEDDER_URL` (default `http://127.0.0.1:8091`), `SERVER_PUBLIC_URL` (canonical base; audience is `<base>/mcp`). Routes: `/mcp`, `/healthz`, `/readyz` (auth database and active release engine).

- [ ] **Step 1: Failing tests:** `TestConfig_RequiredAndDefaults`; `TestRun_ReadyOnlyWithAuthAndActiveRelease`; `TestRun_DrainOnSignal`; `TestAdmin_PrintsTokenOnce` (stdout only, never stderr or logs).
- [ ] **Step 2: Implement**; `main.go` only wires `app.Run` under `signal.NotifyContext`; the runbook covers local start against the native PostgreSQL, the Go query service and a built corpus database, including setting `corpus_release` by hand for local use.
- [ ] **Step 3: Run** `make test`, `make test-integration`, `make pre-push`; commit `feat(server): command, config and runbook`.

### Task 7: End-to-end proof (executing session)

- [ ] **Step 1:** Start the Go query service on 8091 and the server against the newest built corpus database; create an account and a token with the admin CLI.
- [ ] **Step 2:** With an MCP client (Claude Code with the bearer token, or the go-sdk client), run `guide`, `corpus_status`, three golden questions through `search` (compact and full), a batch `read` of the chosen handles and one `next` continuation; record the sizes and the outputs' shape in the PR, never the question text in committed files.
- [ ] **Step 3:** Open the PR; the coordinator merges after the final review and green CI.
