# Server Web API, Login and Sessions Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Routes (`instructions/workflow.md`): Tasks 1, 4, 6 and 7 go to Codex `implementer` on `gpt-6.1-terra`, reviewed by `gpt-6.1-sol`; Tasks 2, 3 and 5 (login, sessions and CSRF, the document reader) go to `implementer` with `--model gpt-6.1-sol`, reviewed by `reviewer` with `--model gpt-6-astra`, Tasks 2 and 3 and the final review with an adversarial pass. Task 8 is the executing session's. Executor: trachuan-dev2; coordinator: the trachuan session, which merges. Starts after Plan 9a merges.

**Goal:** The website's `/api/v1`: Google login, cookie sessions with CSRF, account and personal token management, and search, read, document and status endpoints over the same engine as `/mcp`, with an OpenAPI 3.1 contract and a generated TypeScript client.

**Architecture:** New packages in `apps/server/internal`: `weblogin` (Google OIDC with a database-held login transaction), `websession` (session store, cookie, sliding expiry, CSRF middleware), `accountapi` (account, sessions, tokens), `webapi` (search, read, documents, status handlers that call the same code as `mcpapi`). The contract lives in `packages/schema/openapi.yaml`; Go contract tests parse it and drive the real handlers; the TypeScript client is generated into `packages/schema/gen/openapi.ts`.

**Tech Stack:** Go 1.27.3; `github.com/coreos/go-oidc/v3` v3.21.0 and `golang.org/x/oauth2` v0.37.0 (aboutme's versions); `gopkg.in/yaml.v3` for contract tests; Node with `openapi-typescript` 7.13.0 and `openapi-fetch` 0.17.0, pinned.

**Spec:** [web API](../design/web-api.md) (all), [auth](../design/auth.md), [data](../design/data.md) "Auth database", [retrieval](../design/retrieval.md), [search results](../design/mcp-results.md), [caching](../design/caching.md), [budgets](../design/budgets.md) "Request limits", ADR 0005, ADR 0007. Reference implementation for login, sessions and CSRF: aboutme `apps/server/internal/auth` at `187830407c46` (copy patterns and tests, not code verbatim; adapt names and rules to the spec). The spec wins over this plan.

## Scope

|In|Out (later plans)|
|-|-|
|migration for `web_session`, `login_transaction`, personal token ids; Google login; sessions; CSRF; logout; `/api/v1/me`, sessions and tokens; account deletion request and the 7-day removal job; `/api/v1/search`, `read`, `documents/{key}`, `status`; per-IP login limit; OpenAPI document, contract tests, generated client and CI drift check; runbook for a local Google client|OAuth for MCP, CIMD, DCR, RFC 9728, authorized MCP clients on the account page (9c); `taxonomy` (Plan 6); the Nuxt app (Plan 10)|

## Global Constraints

- Cookies: `__Host-login` (`Path=/`, `Max-Age=600`, `Secure`, `HttpOnly`, `SameSite=Lax`) and `__Host-session` (`Path=/`, `Max-Age` 30 days, `Secure`, `HttpOnly`, `SameSite=Lax`). Tokens, handles, state, nonce and CSRF secrets: 32 random bytes, base64url without padding; tokens and handles stored only as SHA-256 hex.
- Login transaction valid 10 minutes, claimed once atomically. ID token checks: signature, issuer `https://accounts.google.com`, audience the client id, expiry, nonce, `email_verified` true. Account lookup by `google_sub` only. `next` path rules exactly as the spec.
- Sessions: sliding 30-day expiry capped at 90 days after creation, `last_seen_at` written at most once an hour; new session on every login and the previous cookie's session revoked; checks per request as the spec, 10-second lookup cache never beyond `expires_at`, entries dropped at once on every revocation the server process makes; auth database failure is 503.
- CSRF: unsafe methods need `Origin` (or `Referer`) equal to `SERVER_PUBLIC_URL`'s origin and `X-CSRF-Token` equal to the session secret in constant time; JSON bodies only; 403 `csrf_rejected`.
- A session never authorizes `/mcp`; a bearer token never authorizes `/api/v1`. Account routes act only on the caller's rows; another account's id is 404.
- Limits: per-account `search` and `read` buckets shared with `/mcp`; login routes 20 per minute per IP; body 64 KB; web document pages 2 MB of serialized units with 1 MB parts by the `read` rules. Expired and claimed login transactions are deleted every hour.
- Errors `{error:{code,message}}` with the spec's codes and fixed messages. Never log or return query text, returned legal text beyond the response, tokens, cookies, handles, codes or email. Go files at most 400 lines; Markdown at most 300 lines, split by topic without dropping facts.

## Review Focus

- A login callback replayed, raced or used with another browser's state must fail and must not create two sessions: Task 2 `TestCallback_ReplayAndConcurrentClaim`.
- An open redirect through `next` (absolute URL, `//host`, `/\host`, backslash, encoded backslash or control characters, malformed escapes, dot segments): Task 2 `TestNext_RejectsOffsitePaths` on raw and decoded forms.
- A cross-site POST with the session cookie but without a valid Origin and token, including `null` Origin and a missing header: Task 3 `TestCSRF_Matrix`.
- A session or personal token revoked by the server process working at all afterwards (warm cache, lookup in flight), or one revoked by another process working past the 10-second window or past `expires_at`: Task 3 `TestSession_CacheBoundedByWindowAndExpiry` and `TestSession_LocalRevokeEvictsAtOnce`, Task 4 `TestTokens_RevokeEvictsCredentialCache`.
- An auth database failure during the callback issuing a session or redirecting as a rejection: Task 2 `TestCallback_DatabaseFailureIs503AtEachStep`.
- An account route given another account's session or token id: Task 4 `TestAccount_CrossAccountIDsAre404`.

---

### Task 1: Auth schema additions

**Files:** Create `apps/server/migrations/00002_web.sql`, queries in `apps/server/sql/`, regenerated store, integration tests.

**Interfaces:** Tables of `data.md` for `web_session` and `login_transaction`; `personal_token` gains `id bigint GENERATED ALWAYS AS IDENTITY UNIQUE` (token hash stays the primary key). Queries: `CreateLoginTransaction`, `ClaimLoginTransaction` (single statement, returns the row only when unclaimed and unexpired), `DeleteExpiredLoginTransactions`; `CreateSession`, `GetSession` (joined to account: blocked, delete requested, epochs), `TouchSession` (conditional, at most hourly, moves `expires_at`), `RevokeSession`, `ListSessions(account)`, `RevokeOwnSession(account, id)`; `UpsertGoogleAccount(sub, email)` (insert or update email and `last_login_at`), `RequestDeletion(account)` (block, bump epoch, revoke sessions and tokens, set `delete_requested_at` in one statement), `PurgeDeletedAccounts(now)` (rows requested more than 7 days ago); `ListTokens(account)`, `RevokeOwnToken(account, id)`.

- [ ] **Step 1: Failing tests** (integration): `TestMigrate_00002UpTwice`; `TestStore_ClaimOnce` (two concurrent claims, one wins); `TestStore_SessionJoin`; `TestStore_TouchAtMostHourly`; `TestStore_RequestDeletionRevokesAll`; `TestStore_PurgeAfterSevenDays`; `TestStore_OwnRowsOnly`.
- [ ] **Step 2: Implement** the migration with hash format checks, foreign keys with `ON DELETE CASCADE` from account for sessions and tokens, and indexes for every lookup.
- [ ] **Step 3: Run** `make generate`, `make sqlc-check`, integration tests; commit `feat(server): web session and login schema`.

### Task 2: Google login

**Files:** Create `apps/server/internal/weblogin/{login.go,callback.go,transaction.go,next.go}` with tests, and a test OIDC provider in `apps/server/internal/weblogin/oidctest/`.

**Interfaces:** `weblogin.Config{ClientID, ClientSecret, PublicURL string; Issuer string (test override only); Store; Sessions SessionCreator; Clock; Rand io.Reader}`; `weblogin.New(ctx, cfg) (*Handler, error)` (discovers the provider at startup); routes `Start` and `Callback` as `http.Handler`s; `SessionCreator` is `CreateSession(ctx, w, r, accountID int64) error` from Task 3.

- [ ] **Step 1: Failing tests** with `oidctest`: `TestLogin_NewAndReturningAccount`; `TestLogin_EmailUpdatedBySub`; `TestCallback_RejectsBadStateNonceAudienceIssuerExpirySignature`; `TestCallback_RejectsUnverifiedEmail`; `TestCallback_ReplayAndConcurrentClaim`; `TestCallback_ExpiredTransaction`; `TestCallback_BlockedOrDeletedAccount`; `TestCallback_DatabaseFailureIs503AtEachStep` (claim, upsert, presented-session revoke, session create); `TestNext_RejectsOffsitePaths`; `TestLogin_ErrorsAreFixedCodes`; `TestLogin_NoSecretsInLogs`.
- [ ] **Step 2: Implement** per the spec; the redirect URI is built from `PublicURL` only.
- [ ] **Step 3: Run** unit and integration tests; commit `feat(server): Google login`.

### Task 3: Sessions, CSRF and logout

**Files:** Create `apps/server/internal/websession/{session.go,cookie.go,middleware.go,csrf.go,logout.go}` with tests.

**Interfaces:** `websession.Manager{Store; Clock; Rand; PublicURL; CacheTTL}` with `CreateSession(ctx, w, r, accountID) error` (revokes a presented session first), `Middleware(next) http.Handler` (principal in context, 401 `unauthenticated`), `CSRF(next) http.Handler`, `Logout` handler, `FromContext(ctx) (Principal, bool)` where `Principal{AccountID, SessionID int64; CSRFSecret string}`.

- [ ] **Step 1: Failing tests:** `TestCookie_Attributes`; `TestSession_HashedAtRest`; `TestSession_SlidingExpiryAtMostHourly`; `TestSession_AbsoluteNinetyDays` (expires despite recent activity; cookie `Max-Age` clamped); `TestSession_RejectsRevokedExpiredBlockedEpoch`; `TestSession_CacheBoundedByWindowAndExpiry`; `TestSession_LocalRevokeEvictsAtOnce` (warm cache and a lookup racing the revoke); `TestSession_AuthDownIs503`; `TestSession_LoginRevokesPresentedSession`; `TestCSRF_Matrix` (Origin equal, different, `null`, absent with and without Referer, token missing, wrong, duplicate headers, non-JSON body); `TestLogout_RevokesAndClears`; `TestBearerNeverAuthorizesWeb`.
- [ ] **Step 2: Implement** per the spec.
- [ ] **Step 3: Run** unit and integration tests; commit `feat(server): web sessions, CSRF and logout`.

### Task 4: Account endpoints

**Files:** Create `apps/server/internal/accountapi/{me.go,sessions.go,tokens.go,deletion.go}` with tests; the deletion purge runs from the server on a ticker (once an hour).

- [ ] **Step 1: Failing tests:** `TestMe_ReturnsAccountAndCSRF`; `TestSessions_ListAndRevokeOwn`; `TestTokens_CreateShowsOnceListRevoke` (lifetime default 90 days, maximum 365, name 1 to 64 characters); `TestTokens_RevokeEvictsCredentialCache` (the bearer credential cache of Plan 9a drops the token at once); `TestAccount_CrossAccountIDsAre404`; `TestDelete_BlocksRevokesAndPurgesAfterSevenDays`; `TestAccount_LimitsAndCSRF`.
- [ ] **Step 2: Implement**; tokens use Task 2 of Plan 9a (`credential.Issue`); responses never include hashes.
- [ ] **Step 3: Run** tests; commit `feat(server): account, sessions and personal tokens`.

### Task 5: Search, read, documents and status

**Files:** Create `apps/server/internal/webapi/{search.go,read.go,documents.go,status.go}` with tests; let `retrieval.ReadRequest` take the page cap and part size (defaults stay the MCP values) and an outline flag for the first page, with tests.

- [ ] **Step 1: Failing tests:** `TestWeb_SearchEqualsMCP` (same input gives the same JSON as the `search` tool, including cache hits); `TestWeb_ReadEqualsMCP`; `TestDocuments_IsReadWithWebCaps` (same units, parts and `next` as a `read` item over the whole document with a 2 MB page cap and 1 MB parts); `TestDocuments_OversizedLeadAndProvisionAdvance` (a lead or provision above 1 MB is split and every page advances; concatenated parts equal the source); `TestDocuments_FirstPageHasEntryAndOutline`; `TestDocuments_ReleaseExpired`; `TestWeb_StatusEqualsMCP`; `TestWeb_LimitsSharedWithMCP`.
- [ ] **Step 2: Implement**; share the tool code with `mcpapi` through functions both call rather than duplicating it.
- [ ] **Step 3: Run** unit and integration tests; commit `feat(server): web search, read, documents and status`.

### Task 6: OpenAPI contract and generated client

**Files:** Create `packages/schema/{openapi.yaml,package.json,package-lock.json,scripts/gen.sh}`, `packages/schema/gen/openapi.ts` (generated), Go contract tests beside each handler package. Modify `Makefile` (`schema-gen`, `schema-check`), `.github/workflows/ci.yml` (schema job: install pinned packages with `npm ci`, regenerate, `git diff --exit-code`).

- [ ] **Step 1: Failing tests:** one contract test per route asserting documented statuses, media types and response shapes against the real handlers (`gopkg.in/yaml.v3` parse, as aboutme's `health_contract_test.go`).
- [ ] **Step 2: Write** the document for every route and error of the spec; generate the client; CI drift check.
- [ ] **Step 3: Run** `make schema-check`, `make test`, `make pre-push`; commit `feat: OpenAPI contract for /api/v1 and generated client`.

### Task 7: Wiring, config and runbook

**Files:** Modify `apps/server/internal/app/` (routes, the hourly purge of login transactions and deleted accounts, config `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET`, `SERVER_PUBLIC_URL` required for web routes; per-IP login limiter), `docs/runbooks/server-local.md` (create a Google OAuth client for local use with redirect URI `http://localhost:8080/api/v1/auth/google/callback`; browsers treat `http://localhost` as a secure context, so `__Host-` cookies work there; Task 8 verifies this in Chrome and Firefox, and if a browser refuses, the runbook switches to Caddy with a local certificate), `docs/architecture.md`.

- [ ] **Step 1: Failing tests:** `TestConfig_WebRequiresGoogleAndPublicURL`; `TestRoutes_CredentialsNeverCross`.
- [ ] **Step 2: Implement**; commit `feat(server): web routes and config`.

### Task 8: End-to-end check (executing session; needs the owner)

- [ ] **Step 1:** The owner creates a Google OAuth client (Web application) for the local HTTPS origin and puts the id and secret in `.env`.
- [ ] **Step 2:** Log in through the browser, create and revoke a personal token, run search, read and a document page, log out; record shapes and sizes in the PR, never question text.
- [ ] **Step 3:** Open the PR; the coordinator merges after the final adversarial review and green CI.
