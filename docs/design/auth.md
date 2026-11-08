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
`mcpapi` and Google login at one pinned commit after their refactor, then owned
here. Required guarantees, each with a test: PKCE S256 only; single-use
authorization codes bound to client, redirect URI, `resource` and PKCE; `state`
and `iss` in authorization responses; refresh token rotation with reuse
detection that revokes the grant; access tokens bound to `aud` equal to the
canonical `/mcp` URL; scope `mcp:read`.

Client registration order: Client ID Metadata Documents, then pre-registered
clients, then dynamic registration for compatibility. Loopback redirects
`http://127.0.0.1:<any port>` are allowed for native clients (RFC 8252); all
other redirect URIs match byte for byte.

CIMD fetching: HTTPS only; resolve the host, reject private and link-local
addresses, then dial the validated address; no redirects; 5-second timeout;
64 KB body limit; metadata `client_id` must equal the fetched URL; cache at most
1,000 documents for at most 24 hours; at most 10 registrations per client URL.

Personal tokens for CLI clients are random, shown once, stored hashed, and
pass the same middleware as OAuth tokens.

## Token checks and revocation

Each request to `/mcp` or `/api/v1` verifies: token or session hash exists and
`revoked_at` is null; grant not revoked; account not blocked; token issued after
`account.auth_epoch`; `aud` and scope. The lookup is cached 10 seconds per token
hash and the cache entry is dropped on revoke, so revocation takes effect at
once on the revoking node and within 10 seconds elsewhere. If the auth database
is unavailable, requests fail closed with 503.

A web cookie never authorizes `/mcp`; a bearer token never authorizes web
APIs. Account endpoints (`/account`) act only on rows whose `account_id` is the
caller's; tests cover cross-account identifiers.

## Account lifecycle

Block: set `blocked_at`, increment `auth_epoch`, revoke sessions and tokens.
Delete: as block, then remove the row after 7 days. Auth restore from backup:
increment every `auth_epoch` and revoke all sessions and tokens before traffic
resumes, so credentials revoked after the backup cannot return.
