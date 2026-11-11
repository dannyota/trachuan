# OAuth for MCP

The authorization server that lets MCP clients call `/mcp` on behalf of an
account. Guarantees come from [auth](auth.md); limits from
[budgets](budgets.md); tables from [data](data.md). Decision:
[ADR 0005](../adr/0005-google-login-and-embedded-oauth.md). Protocol baseline:
MCP authorization 2026-07-28, OAuth 2.1 (draft 17) with OAuth 2.0 clients
accepted, RFCs 7009, 7591, 8252, 8414, 8707, 9207, 9700 and 9728, and Client ID
Metadata Documents (draft 03).

## Source

The code starts from aboutme `oauthsrv` and the bearer check in `mcpapi` at
commit `1b1ece3d` (2026-11-11, after the refactor; no behavior change since the
reviewed `e2c1d79ff576`). The import records that commit and then trachuan owns
the code. A review of that commit against [auth](auth.md) found these gaps, each
closed here with a test before the import counts as done:

| Gap in aboutme                                                         | Rule here                                                            |
| ---------------------------------------------------------------------- | -------------------------------------------------------------------- |
| `resource` not stored; codes and tokens carry no audience              | [Resource and scope](#resource-and-scope)                            |
| Refresh reuse revokes one token family; other families of a grant live | Reuse revokes the whole grant                                        |
| Epoch kept on the grant; consent again re-stamps it                    | Each code and token stores the account epoch it was issued under     |
| No blocked-account check in the bearer path                            | The shared credential check (auth.md) applies to OAuth tokens        |
| Client table unbounded; `last_used_at` never written                   | [Clients](#clients) cap, protection and eviction                     |
| Auth database errors return 500                                        | [Errors](#errors): 503                                               |
| Consent shows the self-chosen client name only                         | [Consent](#consent) shows hosts and warnings                         |
| Validation errors redirect to any registered callback                  | Errors before consent stay on a server page                          |
| CIMD requires the singular `token_endpoint_auth_method`                | [Clients](#clients) method selection                                 |
| Code exchange requires `redirect_uri`; refresh rejects `scope`         | [Token requests](#token-requests)                                    |
| Revocation ignores a client mismatch; metadata defaults to secrets     | [Revocation](#revocation)                                            |
| Registration response omits granted metadata; one generic error        | [Registration](#registration)                                        |
| No cleanup of expired requests, codes and tokens                       | [Retention](#retention)                                              |
| Scopes `resumes:read` and `resumes:write`                              | One scope, `mcp:read`                                                |
| Sessions, second factor, SPA consent and envelope from aboutme         | trachuan's web session ([web API](web-api.md)), Go consent page here |

## Endpoints

| Method and path                                 | Purpose                                                                                                                          |
| ----------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------- |
| `GET /.well-known/oauth-protected-resource/mcp` | RFC 9728 metadata: `resource` `<base>/mcp`, the issuer, `scopes_supported` `["mcp:read"]`; also served without the `/mcp` suffix |
| `GET /.well-known/oauth-authorization-server`   | RFC 8414 metadata: issuer `<base>`, PKCE `S256`, token and revocation endpoint auth methods `["none"]`, CIMD and `iss` supported |
| `GET /oauth/authorize`                          | Validates the request and starts consent                                                                                         |
| `GET /oauth/consent/{handle}`, `POST` same path | Consent page and decision                                                                                                        |
| `POST /oauth/token`                             | `authorization_code` and `refresh_token` grants                                                                                  |
| `POST /oauth/register`                          | RFC 7591 registration of public clients                                                                                          |
| `POST /oauth/revoke`                            | RFC 7009 revocation                                                                                                              |

`<base>` is `SERVER_PUBLIC_URL`. Every endpoint here shares the login and OAuth
per-IP bucket in budgets.

## Clients

A client is identified, in this order: an `https` `client_id` URL is a Client ID
Metadata Document (CIMD); an ID listed in the pre-registered file
(`SERVER_OAUTH_CLIENTS`, JSON: ID, name, redirect URIs) is pre-registered; any
other ID must come from [registration](#registration). All clients are public.
For a CIMD document the method is `none` when `token_endpoint_auth_method` is
absent or `none` and `token_endpoint_auth_methods_supported`, when present,
contains `none`; any other value or combination rejects the client.

CIMD fetching follows auth.md (HTTPS, DNS name, global unicast addresses only,
validated dial, no redirects, `client_id` equal to the fetched URL) with the
limits in budgets. An IPv4-mapped IPv6 answer is rejected, not unmapped.

Redirect URIs match byte for byte. The one exception is loopback: a registered
`http://127.0.0.1`, `http://[::1]` or `http://localhost` URI matches a requested
URI that is byte-for-byte equal after removing only the port (RFC 8252 §7.3).
`localhost` is accepted for compatibility with native clients that register it.
No other `http` URI is accepted. Redirect URI counts are in budgets.

The client table holds at most the number in budgets. A client is in use while
it has an unexpired pending request, an unconsumed unexpired code or an
unexpired token family. Admitting a new client (registration or first CIMD use)
runs in one transaction that takes a table-wide advisory lock: when the cap is
reached it deletes the least recently used client not in use, or fails with 503
`temporarily_unavailable` when every client is in use. `last_used_at` is written
by authorization and token requests at the interval in budgets.

## Registration

`POST /oauth/register` accepts `redirect_uris` (required), `client_name`,
`grant_types` (subset of `authorization_code`, `refresh_token`),
`response_types` (`code`), `scope` (`mcp:read`) and `token_endpoint_auth_method`
(`none` or absent); other known fields are ignored and not echoed. The 201
response returns `client_id`, `client_id_issued_at` and the effective
`client_name`, `redirect_uris`, `grant_types`
`["authorization_code","refresh_token"]`, `response_types` `["code"]`,
`token_endpoint_auth_method` `none` and `scope` `mcp:read`. A bad redirect URI
returns `invalid_redirect_uri`; any other bad field returns
`invalid_client_metadata`.

## Resource and scope

The resource is the exact string `<base>/mcp`. The authorization request and
both token grants require `resource` equal to it (RFC 8707); anything else
returns `invalid_target`. The resource is stored on the pending request, the
code, the grant and every token; the bearer check accepts a token only when its
stored resource equals the server's. `scope` is optional at authorize and
defaults to `mcp:read`; any other value returns `invalid_scope`.

## Consent

`/oauth/authorize` validates the client, the redirect URI, `response_type`
`code`, the PKCE `S256` challenge, `state`, resource and scope (sizes in
budgets). Any error, before or after the redirect URI is known, renders a server
page with the error and, when the redirect URI is valid, a button that returns
to the client with `error` and `state`; the server never redirects to a client
without that click or a consent decision (RFC 9700 §4.11.2).

A valid request is stored as a pending request under a random handle and bound
to the browser: the response sets cookie `__Host-oauth` (`Path=/`, `Secure`,
`HttpOnly`, `SameSite=Lax`) holding a second random value, and the request row
keeps the SHA-256 of both. The browser is redirected (303) to
`/oauth/consent/{handle}`. The consent page and its `POST` require the cookie to
match the handle's row; the page needs a web session and, without one, redirects
to the Google login with `next` set to the consent path (the `__Host-oauth`
cookie survives the login round trip).

The page (Go `html/template`, Vietnamese with an English toggle) shows the
client name in quotes, the client ID host for CIMD, the literal redirect host,
and these warnings: for DCR clients, that the name is unverified; for loopback
redirects, that an application on this computer will receive access and its
identity is not verified. The form carries
`HMAC-SHA256(session CSRF secret, "oauth-consent:" || handle)`, never the secret
itself; `POST` is `application/x-www-form-urlencoded`, requires that value, an
`Origin` equal to `<base>` and the browser cookie. Responses carry
`Cache-Control: no-store`, `X-Frame-Options: DENY` and a CSP with
`frame-ancestors 'none'` and `form-action` limited to `'self'` and the redirect
origin.

The decision claims the pending request in one statement (unexpired, unclaimed,
matching browser) and binds it to the approving session's account. Approval
creates or reuses the live grant for (account, client) within the per-account
cap in budgets (beyond it the page asks the user to revoke a grant first) and
issues a code; denial returns `access_denied`. Both leave with a 303 carrying
`state` and `iss` (RFC 9207) and clear `__Host-oauth`.

## Token requests

Both grants take `application/x-www-form-urlencoded`, ignore unknown parameters
and reject a repeated parameter with `invalid_request`. `client_id` and
`resource` are required in both.

- `authorization_code`: `code` and `code_verifier` required; `redirect_uri` is
  optional (OAuth 2.1) and, when present, must equal the code's. The code must
  belong to the client, be unexpired and unconsumed, match the PKCE verifier,
  the resource and the account epoch. A second use of a code revokes its grant.
- `refresh_token`: `refresh_token` required; `scope` optional and, when present,
  must be `mcp:read`. A superseded refresh token revokes the grant: every
  family, code and token of it.

Access tokens are `tra_` and refresh tokens `trr_`, each followed by 43
base64url characters, stored as SHA-256 hashes; lifetimes and the families per
grant are in budgets. A refresh token rotates on every use within a family whose
absolute life starts at the code exchange; a code exchange beyond the family cap
revokes the grant's oldest family.

## Bearer check

OAuth tokens pass the auth.md credential check: hash exists, not revoked, not
expired, grant not revoked, account not blocked, token epoch equal to the
account epoch, resource and scope; the 10-second credential cache applies. An
account block, deletion request or auth restore increments the epoch and revokes
every grant in the same transaction. Approving consent again creates new codes
and tokens; it never changes the epoch stored on old ones.

Challenges on `/mcp`: no credential is 401 with
`WWW-Authenticate: Bearer resource_metadata="<base>/.well-known/oauth-protected-resource/mcp", scope="mcp:read"`;
an invalid, expired or revoked one adds `error="invalid_token"`; a token without
the scope is 403 with `error="insufficient_scope"`, the scope and
`resource_metadata`. Personal tokens get the same challenges.

## Revocation

`POST /oauth/revoke` takes `token`, optional `token_type_hint` and required
`client_id`. A token issued to another client is refused with 400
`invalid_request` and nothing changes. An unknown token returns 200. Revoking a
refresh token revokes its grant; revoking an access token revokes that token.
The process drops its cached credential entries at once. The account routes
`GET /api/v1/me/grants` (client name, client ID host, created, last used) and
`DELETE /api/v1/me/grants/{id}` (revoke one grant with its tokens) follow the
[web API](web-api.md) rules for account routes.

## Retention

A cleanup loop in the server deletes, in batches, at the interval in budgets:
pending requests after expiry; codes one day after expiry (kept for replay
detection until then); tokens one day after their family expires or their grant
is revoked; grants with no unexpired family and no pending code for the
retirement period in budgets. Unexpired pending requests are capped in budgets;
at the cap `/oauth/authorize` returns the 503 server page.

## Errors

OAuth endpoints use RFC 6749 error codes in JSON (token, registration,
revocation) or the server page (authorize, consent). Auth database errors,
including the query deadline in budgets, return 503 `temporarily_unavailable` at
OAuth endpoints and 503 `unavailable` at `/mcp`, and change nothing.

## Acceptance

Before the plan lands: Claude Code connects over CIMD and over DCR against a
local server and runs the [MCP tools](mcp-results.md); refresh, revocation
through the account routes and a blocked account behave as above. Claude.ai and
ChatGPT connectors need a public HTTPS URL and run against the first deployment.
