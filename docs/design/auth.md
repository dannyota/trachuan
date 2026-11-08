# Login and agent authorization

Decision: [ADR 0005](../adr/0005-google-login-and-embedded-oauth.md).

## Web login

Google OpenID Connect handled by the Go server, scopes `openid email`, consent
screen in Production mode. Identity is `google_sub`; the email is kept only in
its current value. First login creates the account. Session cookie `HttpOnly`,
`Secure`, `SameSite=Lax`, stored hashed, rotated on login, 30-day sliding
expiry. Logout revokes the server-side session.

## MCP authorization

The OAuth 2.1 authorization server is copied from aboutme's `oauthsrv`,
`mcpapi` and Google login, then owned here. The reference revision for review
is aboutme `e2c1d79ff576` (2026-11-08, before the requested refactor); the import
plan records the final commit, and the import is blocked until the Claude
reviewer has reviewed that revision against the guarantees below with
evidence of the tests. Required guarantees, each with a test: PKCE S256 only; single-use
authorization codes bound to client, redirect URI, `resource` and PKCE; `state`
and `iss` in authorization responses; refresh token rotation with reuse
detection that revokes the grant; access tokens bound to `aud` equal to the
canonical `/mcp` URL; scope `mcp:read`.

Client registration order: Client ID Metadata Documents, then pre-registered
clients, then dynamic registration for compatibility. Loopback redirects
`http://127.0.0.1:<any port>` are allowed for native clients (RFC 8252); all
other redirect URIs match byte for byte.

CIMD fetching: HTTPS only; the host must be a DNS name, never an IP literal;
resolve it, accept only globally routable unicast addresses (reject private,
loopback, link-local, multicast, unspecified, IPv4-mapped IPv6 and other
transition forms), then dial the validated address; no redirects; metadata
`client_id` must equal the fetched URL. Timeouts, body size, concurrent
fetches, cache size and total client storage are in [budgets](budgets.md);
when the client table is full, the least recently used unused client is
evicted.

Personal tokens for CLI clients are random, shown once, stored hashed with an
expiry chosen at creation (default 90 days, at most one year), and pass the
same middleware as OAuth tokens; the expiry check reads `personal_token.expires_at`.

## Token checks and revocation

Each request to `/mcp` or `/api/v1` verifies: token or session hash exists,
`revoked_at` is null and `expires_at` is in the future; grant not revoked;
account not blocked; credential issued after `account.auth_epoch`; `aud` and
scope. The lookup is cached per credential hash for the duration in
[budgets](budgets.md) and never beyond the credential's expiry; the entry is
dropped on revoke. If the
auth database is unavailable, requests fail closed with 503.

A web cookie never authorizes `/mcp`; a bearer token never authorizes web
APIs. Account endpoints (`/account`) act only on rows whose `account_id` is the
caller's; tests cover cross-account identifiers.

## Account lifecycle

Block: set `blocked_at`, increment `auth_epoch`, revoke sessions and tokens.
Delete: as block, then remove the row after 7 days. Auth restore from backup:
increment every `auth_epoch` and revoke all sessions and tokens before traffic
resumes, so credentials revoked after the backup cannot return.
