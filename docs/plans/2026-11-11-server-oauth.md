# Server OAuth for MCP Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Routes (`instructions/workflow.md`): Tasks 1, 2 and 7 go to Codex `implementer` on `gpt-6.1-terra`, reviewed by `gpt-6.1-sol`; Tasks 3 to 6 (clients and CIMD, authorize and consent, tokens, bearer check and revocation) go to `implementer` with `--model gpt-6.1-sol`, reviewed by `reviewer` with `--model gpt-6-astra` with an adversarial pass; the final whole-branch review is `gpt-6-astra` adversarial. Task 8 is the executing session's. Executor: trachuan-dev2; coordinator: the trachuan session, which merges. Starts after Plan 9b merges.

**Goal:** MCP clients connect to `/mcp` through OAuth: CIMD, pre-registered and dynamically registered public clients, Google-login consent, PKCE codes, rotating refresh tokens bound to the `/mcp` resource, revocation, and the account's list of authorized clients.

**Architecture:** New package `apps/server/internal/oauth` (one concern per file: metadata, clients, cimd, registration, authorize, consent, token, revoke, retention) started from aboutme `apps/server/internal/oauthsrv` at `1b1ece3d` and changed to the spec; the consent page is a Go template in the same package. `apps/server/internal/credential` gains OAuth tokens and the RFC 6750 challenges. `accountapi` from Plan 9b gains the grant routes. Storage is new tables in the auth database through sqlc.

**Tech Stack:** Go 1.27.3, `html/template`, pgx v5 and sqlc 1.31.1 as in server-core; no new modules (aboutme's `github.com/google/uuid` is replaced by random base64url client IDs and bigint identities).

**Spec:** [OAuth](../design/oauth.md) (all), [auth](../design/auth.md), [web API](../design/web-api.md) "Sessions", "CSRF" and the grant routes, [data](../design/data.md) "Auth database", [budgets](../design/budgets.md) "Request limits", ADR 0005. Reference implementation: aboutme `apps/server/internal/oauthsrv` and `apps/server/internal/mcpapi/bearer.go` at `1b1ece3d`; review notes in the design's Source table. The spec wins over this plan and over the reference.

## Scope

|In|Out (later plans)|
|-|-|
|migration for the five OAuth tables; RFC 8414 and RFC 9728 metadata; CIMD, pre-registered file and DCR; authorize, browser-bound pending requests, Go consent page in Vietnamese and English; token endpoint; RFC 6750 challenges on `/mcp`; `/oauth/revoke`; `GET` and `DELETE /api/v1/me/grants` with OpenAPI and generated client; retention loop; local acceptance with Claude Code; the ADR 0013 agent-in-the-loop check|Claude.ai and ChatGPT connectors against a public deployment (Plan 11); the Nuxt account page (Plan 10)|

## Global Constraints

- Values come from budgets: client cap 10,000; 20 live grants per account; 1 to 5 redirect URIs; `state` 512 bytes; pending request 10 min; code 60 s; access token 1 h; refresh family 30 days; 10 families per grant; 10,000 unexpired pending requests; `last_used_at` at most hourly; cleanup every 10 min in batches of 1,000; replay retention 1 day; grant retirement 30 days; CIMD fetch 5 s, 64 KB, no redirects, 4 concurrent, 1,000 cached for 24 h; OAuth endpoints share the login per-IP bucket of 20 per minute; auth query deadline 2 s.
- Secrets: handles, browser values, codes and tokens are 32 random bytes base64url without padding; access tokens `tra_` and refresh tokens `trr_` plus 43 characters; all stored only as SHA-256 hex. Consent form token is `HMAC-SHA256(session CSRF secret, "oauth-consent:" || handle)`.
- Cookie `__Host-oauth` (`Path=/`, `Secure`, `HttpOnly`, `SameSite=Lax`, `Max-Age=600`), cleared on decision.
- Resource is exactly `SERVER_PUBLIC_URL + "/mcp"`; scope only `mcp:read`; PKCE `S256` only; every client public (`none`).
- Every redirect to a client is a 303 after a consent decision or a click on the error page, with `state` and `iss`.
- Auth database errors: 503 `temporarily_unavailable` at OAuth endpoints, 503 `unavailable` at `/mcp`; unknown commit outcomes fail closed as the spec says.
- Never log tokens, codes, handles, browser values, `state`, PKCE values, cookies or email. Go files at most 400 lines; Markdown at most 300 lines.

## Review Focus

- A pending request approved from another browser, or a consent `POST` without the browser cookie, the form token or a matching `Origin`: Task 4 `TestConsent_BrowserBindingAndForgery`.
- Two concurrent refreshes with the same refresh token: exactly one succeeds and the other revokes the grant: Task 5 `TestRefresh_ConcurrentReuseRevokesGrant`.
- A token whose stored resource differs from `<base>/mcp`, or one issued before an account block or epoch change, accepted at `/mcp` (warm cache included): Task 6 `TestBearer_ResourceEpochBlockAndCache`.
- A client evicted while its user is between authorize and token exchange, under concurrent admissions at the cap: Task 3 `TestAdmission_NeverEvictsInUse`.
- A CIMD URL whose host resolves to a private, mapped or rebinding address, or whose document redirects or exceeds 64 KB: Task 3 `TestCIMD_FetchPolicy`.
- An error at authorize that sends the browser to a registered phishing URL without a click: Task 4 `TestAuthorize_ErrorsStayOnServerPage`.

---

### Task 1: OAuth schema

**Files:** Create `apps/server/migrations/00003_oauth.sql`, queries in `apps/server/sql/`, regenerated store, integration tests.

**Interfaces:** Tables of data.md (`oauth_client`, `oauth_request`, `oauth_grant`, `oauth_code`, `oauth_token`) with foreign keys from grants, codes and tokens to `account` and `oauth_client` (`ON DELETE CASCADE` from account), unique live grant per (account, client), hash format checks. Queries: `CreateClient`, `GetClient`, `TouchClient` (hourly), `EvictClient` (least recently used not in use), `CountClients`; `CreateRequest`, `ClaimRequest` (single statement: unexpired, unclaimed, browser hash; sets account); `UpsertGrant`, `CountLiveGrants`, `RevokeGrant` (grant, codes, tokens in one statement); `CreateCode`, `ConsumeCode` (single statement returning the row or the replay marker); `CreateTokenFamily`, `RotateRefresh` (conditional on not superseded), `GetToken`, `RevokeToken`, `ListGrants(account)`, `RevokeOwnGrant(account, id)`; retention deletes.

- [ ] **Step 1: Failing tests** (integration): `TestMigrate_00003UpTwice`; `TestStore_ClaimRequestOnce`; `TestStore_ConsumeCodeOnce`; `TestStore_RotateRefreshOnce`; `TestStore_RevokeGrantRevokesAll`; `TestStore_OwnGrantsOnly`; `TestStore_RetentionDeletes`.
- [ ] **Step 2: Implement** the migration and queries.
- [ ] **Step 3: Run** `make generate`, `make sqlc-check`, integration tests; commit `feat(server): OAuth schema`.

### Task 2: Import, metadata and challenges

**Files:** Create `apps/server/internal/oauth/{metadata.go,pkce.go,scope.go,redirect.go,entropy.go,errors.go}` with tests, from the reference with aboutme dependencies removed; modify `apps/server/internal/credential` for the three `WWW-Authenticate` challenges on `/mcp` (personal tokens included); `docs/architecture.md` records the reference commit.

- [ ] **Step 1: Failing tests:** `TestMetadata_AuthorizationServer` (issuer, endpoints, `S256`, `["none"]` for token and revocation auth, CIMD and `iss` flags); `TestMetadata_ProtectedResourceBothPaths`; `TestPKCE_S256Only`; `TestScope_DefaultAndInvalid`; `TestRedirect_LoopbackPortOnly` (127.0.0.1, `[::1]`, localhost; query or path difference rejected); `TestRedirect_ExactOtherwise`; `TestChallenge_MissingInvalidInsufficient`.
- [ ] **Step 2: Implement**; commit `feat(server): OAuth metadata and protocol helpers`.

### Task 3: Clients, CIMD and registration

**Files:** Create `apps/server/internal/oauth/{clients.go,admission.go,cimd.go,cimd_fetch.go,register.go,preregistered.go}` with tests.

**Interfaces:** `Clients.Resolve(ctx, clientID) (Client, error)` in the order CIMD, pre-registered (`SERVER_OAUTH_CLIENTS` file), registered; `Admit(ctx, tx, client)` under the exclusive advisory lock; `InUse(ctx, tx, clientID)` taken under the shared lock by Tasks 4 and 5.

- [ ] **Step 1: Failing tests:** `TestCIMD_FetchPolicy` (IP literal, private, loopback, link-local, multicast, IPv4-mapped, NAT64, 6to4 and Teredo answers rejected; dial uses the validated address; redirect, oversize, timeout, `client_id` mismatch rejected); `TestCIMD_MethodSelection` (absent, singular, plural, forbidden method anywhere); `TestCIMD_CacheBound`; `TestRegister_FieldsResponseAndErrors` (`invalid_redirect_uri` against `invalid_client_metadata`; secret methods refused); `TestPreregistered_FileLoad`; `TestAdmission_CapEvictsLeastRecentNotInUse`; `TestAdmission_NeverEvictsInUse` (concurrent admissions and authorizations at the cap); `TestAdmission_AllInUseIs503`.
- [ ] **Step 2: Implement**; commit `feat(server): OAuth clients, CIMD and registration`.

### Task 4: Authorize and consent

**Files:** Create `apps/server/internal/oauth/{authorize.go,consent.go,consent_page.go,templates/consent.html,templates/error.html}` with tests.

**Interfaces:** Consumes Plan 9b's session lookup (`websession` current account and CSRF secret) and login start URL; Task 3 `Clients`; Task 1 queries.

- [ ] **Step 1: Failing tests:** `TestAuthorize_ValidatesAll` (client, redirect, `response_type`, PKCE, `state` size, resource, scope); `TestAuthorize_ErrorsStayOnServerPage` (no `Location` to the client; the button carries `error`, `state`, `iss`); `TestAuthorize_PendingCap503`; `TestConsent_RequiresSessionAndRoundTripsLogin`; `TestConsent_BrowserBindingAndForgery`; `TestConsent_ShowsHostsAndWarnings` (CIMD host, literal redirect host, DCR and loopback warnings, Vietnamese and English); `TestConsent_HeadersAndCSP`; `TestConsent_ApproveDeny303WithStateIss`; `TestConsent_GrantCap`; `TestConsent_ClaimOnceUnderRace`.
- [ ] **Step 2: Implement**; commit `feat(server): OAuth authorize and consent`.

### Task 5: Token endpoint

**Files:** Create `apps/server/internal/oauth/{token.go,token_form.go,code_exchange.go,refresh.go}` with tests.

- [ ] **Step 1: Failing tests:** `TestTokenForm_UnknownIgnoredRepeatRejected`; `TestCode_ExchangeBindings` (client, redirect when present, PKCE, resource, epoch, expiry); `TestCode_ReplayRevokesGrant`; `TestRefresh_RotatesAndKeepsFamilyLife`; `TestRefresh_ScopeOptional`; `TestRefresh_ConcurrentReuseRevokesGrant`; `TestFamilies_CapRevokesOldest`; `TestToken_DatabaseErrors503AndUnknownCommit`.
- [ ] **Step 2: Implement**; commit `feat(server): OAuth token endpoint`.

### Task 6: Bearer check, revocation and grant routes

**Files:** Modify `apps/server/internal/credential` (OAuth tokens through the shared check: grant, block, epoch, resource, scope, cache), create `apps/server/internal/oauth/revoke.go`, modify `apps/server/internal/accountapi` and `packages/schema/openapi.yaml` (grant routes), regenerate the client.

- [ ] **Step 1: Failing tests:** `TestBearer_ResourceEpochBlockAndCache`; `TestBearer_OAuthAndPersonalTokensShareChecks`; `TestRevoke_ClientMismatchRefused`; `TestRevoke_UnknownIs200`; `TestRevoke_RefreshRevokesGrantAndEvictsCache`; `TestGrants_ListAndRevokeOwnOnly`; contract tests for the two routes.
- [ ] **Step 2: Implement**; commit `feat(server): OAuth bearer check, revocation and grants`.

### Task 7: Retention, wiring, config and runbook

**Files:** Create `apps/server/internal/oauth/retention.go`; modify `apps/server/internal/app/` (routes, config `SERVER_OAUTH_CLIENTS` optional, the retention loop), `docs/runbooks/server-local.md` (connect Claude Code to the local server), `docs/architecture.md`.

- [ ] **Step 1: Failing tests:** `TestRetention_DeletesByRule` (requests, codes after replay retention, tokens after family expiry or revocation, retired grants); `TestRoutes_OAuthMounted`; `TestConfig_PreregisteredFileOptional`.
- [ ] **Step 2: Implement**; commit `feat(server): OAuth wiring and retention`.

### Task 8: Acceptance (executing session)

- [ ] **Step 1:** Local server with Google login (Plan 9b client); Claude Code connects over CIMD and over DCR; run `guide`, `corpus_status`, `search` and `read`; refresh after the access token expires (shortened lifetime in a test build only); revoke the grant through `/api/v1/me/grants`; block the account; record outcomes in the PR, never question text.
- [ ] **Step 2:** ADR 0013 agent-in-the-loop check: Claude Code answers five golden questions through the tools with no other context; record per question whether it searched compactly, read chosen handles in one batch, followed `next` and cited handles; findings go to the backlog or ADR 0013 History.
- [ ] **Step 3:** Add a backlog line for Claude.ai and ChatGPT connector acceptance on the first public deployment; open the PR; the coordinator merges after the final adversarial review and green CI.
