# Web API and sessions

The website talks to the Go server through `/api/v1`, signed in with a Google
session cookie. It uses the same retrieval engine, presentation and per-account
limits as `/mcp` ([retrieval](retrieval.md), [search results](mcp-results.md)).
Login and account rules come from [auth](auth.md) and
[ADR 0005](../adr/0005-google-login-and-embedded-oauth.md); this note owns the
routes, the session and CSRF mechanics and the contract document. The flow
follows aboutme's `apps/server/internal/auth` at commit `187830407c46` (Google
login, sessions and CSRF), adapted to the rules below.

## Google login

`GET /api/v1/auth/google/start?next=<path>` starts the OpenID Connect
authorization code flow with PKCE (S256), scopes `openid email`, issuer
`https://accounts.google.com`, through `github.com/coreos/go-oidc/v3` and
`golang.org/x/oauth2`. The server creates a login transaction: a random handle,
`state`, `nonce` and PKCE verifier (32 random bytes each, base64url), the
validated `next` path and the redirect URI, stored in `login_transaction` keyed
by the SHA-256 of the handle, valid for 10 minutes. The server deletes expired
and claimed transactions every hour. The browser gets only the handle, in cookie
`__Host-login` (`Path=/`, `Max-Age=600`, `Secure`, `HttpOnly`, `SameSite=Lax`),
and a 302 to Google. The redirect URI is `SERVER_PUBLIC_URL` plus the callback
path; it is never taken from the request.

`GET /api/v1/auth/google/callback` claims the transaction for the cookie's
handle atomically and once, then requires: `state` equal to the stored value;
the code exchanged with the stored verifier; an ID token verified for signature,
issuer (`https://accounts.google.com`, or `accounts.google.com`, which Google
also documents as valid), audience (the client id) and expiry; `nonce` equal to
the stored value; `email_verified` true. A rejected login (bad state, nonce,
token or code, an unverified email, an expired or already claimed transaction, a
blocked account, a new account whose email sign-up does not allow) clears the
cookie and redirects to `/login?error=<code>` with a fixed code (`login_failed`,
`login_expired`, `account_blocked`, `signup_closed`). A failure of the auth
database or of Google's endpoints at any step (claiming the transaction,
upserting the account, revoking a presented session, creating the session) is
503 `unavailable` and issues no session; the transaction stays claimed, so the
user starts again. Provider error text is never shown or logged.

The account is found by `google_sub` only, never by email. The first login
creates it; every login stores the current email and `last_login_at`. Sign-up is
open unless `SERVER_SIGNUP_EMAILS` is set: a comma-separated list of emails,
compared after trimming and lowercasing. Then a Google subject with no account
is admitted only when its verified email is in the list; otherwise the callback
redirects with `signup_closed` and writes no account, session or login time (the
login transaction stays claimed). An existing account signs in whatever the list
says. A variable that is present but empty, or holds an entry that is not a
plain address (a non-empty local part, `@`, a domain with a dot, no whitespace
and no display name), stops the server at startup. A blocked account, or one
whose deletion was requested, cannot log in.

`next` is validated twice, on the raw value and after decoding percent-escapes
once (a malformed escape rejects it). Both forms must start with exactly one
`/`, must not start with `//` or `/\`, and must contain no backslash, no control
character (U+0000 to U+001F, U+007F) and no `.` or `..` path segment; the raw
value is at most 512 bytes. A rejected `next` becomes `/`. The redirect uses the
validated raw value. The `hd` claim is not checked: any Google account may sign
in.

### Test identity provider

Browser and end-to-end tests sign in through `apps/server/cmd/fakegoogle`, a
test-only OpenID provider; it never ships (see
[deployment](deployment.md#images-and-deploy)). It listens only on a loopback
address and serves discovery, JWKS, an authorization page and the token
endpoint. The authorization page is an HTML form (an email field and an "email
verified" box, checked by default) whose submit issues a single-use code bound
to the request's PKCE S256 challenge, redirect URI and nonce. It accepts only
its configured client id and the exact configured redirect URI and answers
anything else with 400, never a redirect. The subject is derived from the email
(`fake-` plus the first 20 hex digits of its SHA-256), so one email is one
account. ID tokens are RS256 with a key made at startup.

The server takes the provider from `SERVER_GOOGLE_ISSUER`; unset or empty means
`https://accounts.google.com`. Any other value is accepted only when all of
these hold, and otherwise `serve` exits at startup with
`SERVER_GOOGLE_ISSUER requires a loopback deployment`, never echoing values: the
host of `SERVER_PUBLIC_URL` and of `SERVER_LISTEN` is loopback, and the issuer
is exactly `http://<loopback host>:<port>` (explicit port 1 to 65535, no user
info, path, query or fragment, and equal to its own normalized form). A loopback
host is `localhost` in lowercase or an IP literal in 127.0.0.0/8 or `::1`, with
no zone, no IPv4-mapped form and no trailing dot. When the override is active,
`serve` writes one startup line saying so, and the callback accepts only that
exact issuer.

## Sessions

A successful callback creates a new `web_session` row and sets cookie
`__Host-session` (`Path=/`, `Secure`, `HttpOnly`, `SameSite=Lax`, `Max-Age` 30
days). The token is 32 random bytes, base64url, stored only as its SHA-256. A
session lives at most 90 days after its creation, whatever its activity. A
session cookie present at login is revoked first, so a session is never carried
across a login. Expiry slides: a request on a session whose `last_seen_at` is
more than one hour old sets `last_seen_at` to now and `expires_at` to the
earlier of now plus 30 days and creation plus 90 days, and sets the cookie's
`Max-Age` to the time left until that `expires_at`; the cookie set at login is
clamped the same way. Other requests write nothing.

Each request on `/api/v1` with a session verifies: the hash exists, `revoked_at`
is null, `expires_at` is in the future, the account is not blocked and the
session's `auth_epoch` equals the account's. The lookup uses the same 10-second
cache as bearer credentials ([auth](auth.md)), never beyond `expires_at`; a
failure of the auth database is 503. Every revocation made by the server process
(logout, revoking a session or personal token on the account page, a deletion
request) drops the cache entries of the credentials it revokes before it
responds, so they stop working at once, including a lookup that was in flight;
revocations made by another process, such as the admin command, take effect
within the 10-second window. A session cookie never authorizes `/mcp` and a
bearer token never authorizes `/api/v1`.

`POST /api/v1/auth/logout` revokes the session, clears the cookie and sends
`Clear-Site-Data: "cookies"`. Sessions are listed and revoked on the account
page.

## CSRF

Every request to `/api/v1` with a method other than `GET`, `HEAD` or `OPTIONS`
requires: `Origin` equal to the origin of `SERVER_PUBLIC_URL` (the `Referer`
origin when `Origin` is absent; both absent fails); header `X-CSRF-Token` equal
to the session's CSRF secret, compared in constant time; and
`Content-Type: application/json` when a body is sent. The secret is 32 random
bytes created with the session and returned only in the body of
`GET /api/v1/me`. Failures return 403 `csrf_rejected`. `GET` handlers change no
account or corpus state; the only writes made by `GET` requests are the login
start and callback (protected by the login transaction) and the session's own
`last_seen_at` and `expires_at` maintenance. Search and read are `POST` and
follow the same rule. The OAuth consent form has its own rule in
[OAuth](oauth.md#consent).

## Routes

| Method and path                      | Credential     | Limit bucket  | Purpose                                                                                                                                                                             |
| ------------------------------------ | -------------- | ------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `GET /api/v1/auth/google/start`      | none           | login per IP  | Start Google login                                                                                                                                                                  |
| `GET /api/v1/auth/google/callback`   | login cookie   | login per IP  | Finish Google login                                                                                                                                                                 |
| `POST /api/v1/auth/logout`           | session + CSRF | read          | Revoke this session                                                                                                                                                                 |
| `GET /api/v1/me`                     | session        | read          | Account id, email, created date, CSRF token                                                                                                                                         |
| `DELETE /api/v1/me`                  | session + CSRF | read          | Request deletion: block now, remove after 7 days ([auth](auth.md))                                                                                                                  |
| `GET /api/v1/me/sessions`            | session        | read          | Own live sessions (not revoked, not expired): id, created, last seen, current flag                                                                                                  |
| `DELETE /api/v1/me/sessions/{id}`    | session + CSRF | read          | Revoke one own session                                                                                                                                                              |
| `GET /api/v1/me/tokens`              | session        | read          | Own personal tokens: id, name, created, expires, last used, revoked                                                                                                                 |
| `POST /api/v1/me/tokens`             | session + CSRF | read          | Create a personal token `{name, days}`; the token is returned once                                                                                                                  |
| `DELETE /api/v1/me/tokens/{id}`      | session + CSRF | read          | Revoke one own personal token                                                                                                                                                       |
| `GET /api/v1/me/grants`              | session        | read          | Own OAuth grants ([OAuth](oauth.md))                                                                                                                                                |
| `DELETE /api/v1/me/grants/{id}`      | session + CSRF | read          | Revoke one own OAuth grant and its tokens                                                                                                                                           |
| `POST /api/v1/reports`               | session + CSRF | read          | Report a corpus defect ([error reports](error-reports.md))                                                                                                                          |
| `GET /api/v1/me/reports`             | session        | read          | Own error reports                                                                                                                                                                   |
| `DELETE /api/v1/me/reports/{id}`     | session + CSRF | read          | Withdraw an own open report                                                                                                                                                         |
| `POST /api/v1/search`                | session + CSRF | search        | The `search` tool's input and result                                                                                                                                                |
| `POST /api/v1/read`                  | session + CSRF | read          | The `read` tool's input and result                                                                                                                                                  |
| `GET /api/v1/documents/{key}`        | session        | read          | Reader page: document entry, outline and text from `from`                                                                                                                           |
| `GET /api/v1/status`                 | session        | read          | The `corpus_status` tool's result                                                                                                                                                   |
| `GET /api/v1/public/status`          | none           | public per IP | Release, counts, newest source date and gap counts for the public pages; same fields as `corpus_status`, never provision text or account data; `Cache-Control: public, max-age=600` |
| `GET /api/v1/public/taxonomy`        | none           | public per IP | The `taxonomy` tool without `q`: `node` and `depth` as query parameters; same result; `Cache-Control: public, max-age=600`; typed lookup text never goes to this route              |
| `GET /api/v1/public/taxonomy/{code}` | none           | public per IP | A served node with its path, children and its 20 documents with the most effective assignments in the subtree (number, title, type, status); never provision text; cacheable 600 s  |

Account routes list and create within the per-account credential bounds in
[budgets](budgets.md#request-limits). Account routes act only on rows whose
`account_id` is the caller's; an id that belongs to another account is 404, the
same as an unknown id. Personal tokens created here follow [auth](auth.md):
default 90 days, at most one year.

`GET /api/v1/documents/{key}?release=<rel>&next=<token>` is the reader's
whole-document read: the `read` item `{handle: key, from: first, to: last}` of
[search results](mcp-results.md#read) with the web caps of
[budgets](budgets.md#request-limits): units (ancestor leads, provisions) and
parts by the same cutting rule, a page cap of 2 MB of serialized units and a
part size of 1 MB, so a long lead or provision is split into parts and every
page makes progress. The unit sequence starts with the document entry with its
status, then the outline entries, then the leads and provisions; outline entries
are ordinary units of that sequence, so a long outline continues across pages
through `next` like any other units. The response is
`{release, fetch_sha256, item, nodes}`: `release` is the release the page was
read from, `item` is the `read` item result object, and `fetch_sha256` is the
document's fetch hash in that release, read with the item from the same release
(directly or through the release-keyed read cache). It is present on every page
that carries the document, first or continuation, and absent when `item.error`
is set. `nodes` names (`name`, `level`) every node code in `item`. Continuation
is `item.next`, passed back as `next`. The website links search results to the
reader with `?release=`, so the reader shows the release the result came from;
the reader shows the document's single `source_url` only when present.

Search, read and status take the same inputs and return the same objects as the
MCP tools. Documents wraps one `read` item result with the web caps in the
object above; MCP result shapes do not change. All four keep `release` pinning
and `release_expired`; only the transport differs.

## Errors and limits

Errors are `{error:{code,message}}` with fixed messages: 400 `invalid_request`,
401 `unauthenticated`, 403 `csrf_rejected` or `account_blocked`, 404
`not_found`, 409 `release_expired`, 413 `body_too_large`, 415
`unsupported_media_type`, 429 `rate_limited` with `Retry-After`, 503 `busy` or
`unavailable`, 500 `internal`. Request bodies are at most 64 KB. Per-IP limits
use the client address: the connection peer, or, when the peer is in
`SERVER_TRUSTED_PROXIES` (CIDRs, empty by default; Caddy on the host in
production), the rightmost `X-Forwarded-For` address not in that list. The
per-account `search` and `read` buckets are shared with `/mcp`; login routes use
the per-IP limit in [budgets](budgets.md#request-limits). Logs carry account id,
route, code and latency, never query text, returned text, tokens, cookies or
email.

## Contract

`packages/schema/openapi.yaml` (OpenAPI 3.1) is the single contract for
`/api/v1`. Go contract tests parse it and drive the real handlers, asserting
every documented status, media type and response shape, as aboutme does. The
TypeScript client for the website is generated from it with `openapi-typescript`
7.13.0 into `packages/schema/gen/openapi.ts` and used through `openapi-fetch`
0.17.0, both pinned; the generated file is committed and CI regenerates it and
fails on any difference.
