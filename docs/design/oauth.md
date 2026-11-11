# OAuth for MCP

The authorization server that lets MCP clients call `/mcp` on behalf of an
account. Guarantees come from [auth](auth.md); limits from
[budgets](budgets.md); tables from [data](data.md). Decision:
[ADR 0005](../adr/0005-google-login-and-embedded-oauth.md).

## Source

The code starts from aboutme `oauthsrv` and the bearer check in `mcpapi` at
commit `1b1ece3d` (2026-11-11, after the refactor; no behavior change since the
reviewed `e2c1d79ff576`). The import records that commit and then trachuan owns
the code. A review of that commit against [auth](auth.md) found these gaps, each
closed here with a test before the import counts as done:

| Gap in aboutme                                                         | Rule here                                                             |
| ---------------------------------------------------------------------- | --------------------------------------------------------------------- |
| `resource` not stored; codes and tokens carry no audience              | [Resource and audience](#resource-and-audience)                       |
| Refresh reuse revokes one token family; other families of a grant live | Reuse revokes the whole grant                                         |
| Epoch kept on the grant; consent again re-stamps it                    | Each code and token stores the account epoch it was issued under      |
| No blocked-account check in the bearer path                            | The shared credential check (auth.md) applies to OAuth tokens         |
| Client table unbounded; `last_used_at` never written                   | [Clients](#clients) cap and eviction                                  |
| Auth database errors return 500                                        | 503 `unavailable` on every endpoint                                   |
| DCR consent shows only the self-chosen client name                     | [Consent](#consent) shows the redirect host and marks DCR unverified  |
| Scopes `resumes:read` and `resumes:write`                              | One scope, `mcp:read`                                                 |
| Sessions, second factor, SPA consent and envelope from aboutme         | trachuan's web session ([web API](web-api.md)) and a server page here |

## Endpoints

| Method and path                                 | Purpose                                                                                |
| ----------------------------------------------- | -------------------------------------------------------------------------------------- |
| `GET /.well-known/oauth-protected-resource/mcp` | RFC 9728 metadata for the resource `<base>/mcp`; also served without the `/mcp` suffix |
| `GET /.well-known/oauth-authorization-server`   | RFC 8414 metadata: issuer `<base>`, PKCE `S256`, auth method `none`, CIMD supported    |
| `GET /oauth/authorize`                          | Validates the request, stores it as a pending request, redirects to the consent page   |
| `GET /oauth/consent/{handle}`, `POST` same path | Consent page and decision ([Consent](#consent))                                        |
| `POST /oauth/token`                             | `authorization_code` and `refresh_token` grants                                        |
| `POST /oauth/register`                          | RFC 7591 registration for public clients only                                          |
| `POST /oauth/revoke`                            | RFC 7009 revocation of a refresh or access token                                       |

`<base>` is `SERVER_PUBLIC_URL`. A request to `/mcp` without a valid token gets
401 with
`WWW-Authenticate: Bearer resource_metadata="<base>/.well-known/oauth-protected-resource/mcp"`;
a valid token without the scope gets 403 with `error="insufficient_scope"`.
Every endpoint here shares the login and OAuth per-IP bucket in budgets.

## Clients

A client is identified, in this order: an `https` `client_id` URL is a Client ID
Metadata Document (CIMD); an ID listed in the pre-registered file
(`SERVER_OAUTH_CLIENTS`, JSON: ID, name, redirect URIs) is pre-registered; any
other ID must come from `/oauth/register`. All clients are public:
`token_endpoint_auth_method` is `none`, and a registration asking for a secret
is refused with `invalid_client_metadata`.

CIMD fetching follows auth.md (HTTPS, DNS name, global unicast addresses only,
validated dial, no redirects, `client_id` equal to the fetched URL) with the
limits in budgets. An IPv4-mapped IPv6 answer is rejected, not unmapped.

Redirect URIs match byte for byte, except loopback: a registered
`http://127.0.0.1/<path>`, `http://[::1]/<path>` or `http://localhost/<path>`
matches the same host and path on any port (RFC 8252). No other `http` URI is
accepted. The number of redirect URIs per client is in budgets.

The client table holds at most the number in budgets. Use of a client in an
authorization or token request writes `last_used_at`, at most once an hour. When
a new client would exceed the cap, the least recently used client with no live
grant is deleted with its expired codes; when every client has a live grant,
registration and first CIMD use fail with 503 `temporarily_unavailable`.

## Resource and audience

The resource is the exact string `<base>/mcp`. `/oauth/authorize` and both token
grants require `resource` equal to it (RFC 8707); anything else returns
`invalid_target`. The resource is stored on the pending request, the code, the
grant and every token; the bearer check accepts a token only when its stored
resource equals the server's resource. `scope` is optional at authorize and
defaults to `mcp:read`; any other value returns `invalid_scope`.

## Consent

`/oauth/authorize` validates the client, redirect URI, PKCE `S256` challenge,
`state`, resource and scope (sizes in budgets), then stores a pending request
under a random handle (hashed at rest, single use, lifetime in budgets) and
redirects to `/oauth/consent/{handle}`. Errors before the redirect URI is
validated are shown on a server page, never sent to the client.

The consent page needs a web session; without one it redirects to the Google
login with `next` set to the consent path (within the 512-byte rule of the web
API). The server renders the page itself (Go `html/template`, Vietnamese with an
English toggle) until the web app exists. It shows the client name in quotes,
the client ID host for CIMD, the redirect host (or "this computer" for loopback)
and, for DCR clients, a warning that the name is unverified. The form carries
the session's CSRF secret; `POST` requires it and an `Origin` equal to `<base>`.
Responses carry `Cache-Control: no-store`, `X-Frame-Options: DENY` and a CSP
with `frame-ancestors 'none'` and `form-action` limited to `'self'` and the
redirect origin.

Approval creates or reuses the live grant for (account, client), within the
per-account grant cap in budgets, and issues a code; a consent beyond the cap
shows an error page asking the user to revoke a grant first; denial redirects
with `access_denied`. Both responses include `state` and `iss` (RFC 9207).

## Codes and tokens

Codes are single use and are bound to the client, redirect URI, resource, PKCE
challenge, scope, grant and the account epoch. Replaying a used code revokes the
grant.

Access tokens are `tra_` and refresh tokens `trr_`, each followed by 43
base64url characters; lifetimes are in budgets. A refresh token rotates on every
use within a family whose absolute life starts at the code exchange. All are
stored as SHA-256 hashes. Presenting a superseded refresh token revokes the
grant: every family, code and token of it.

The bearer check for OAuth tokens is the auth.md credential check: hash exists,
not revoked, not expired, grant not revoked, account not blocked, token epoch
equal to the account epoch, resource and scope; the 10-second credential cache
applies. An account block, deletion request or auth restore increments the epoch
and revokes every grant in the same transaction. Approving consent again creates
new codes and tokens; it never changes the epoch stored on old ones.

The account routes `GET /api/v1/me/grants` (client name, client ID host,
created, last used) and `DELETE /api/v1/me/grants/{id}` (revoke one grant with
its tokens) follow the [web API](web-api.md) rules for account routes.

Auth database errors, including the query deadline in budgets, return 503
`temporarily_unavailable` at the OAuth endpoints and 503 at `/mcp`.

## Acceptance

Before the plan lands: Claude Code connects over CIMD and over DCR against a
local server and runs the [MCP tools](mcp-results.md); refresh, revocation on
the account routes and a blocked account behave as above. Claude.ai and ChatGPT
connectors need a public HTTPS URL and run against the first deployment.
