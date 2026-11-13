# Website

Decision: [ADR 0007](../adr/0007-nuxt-frontend.md). Routes are English with a
country segment; content defaults to Vietnamese with optional English. Legal
text is never translated.

## Public, no account

| Route                             | Content                                                                                                                                                                                     |
| --------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `/`                               | Purpose, one real pre-rendered sample result, coverage facts from `GET /api/v1/public/status`, Google sign-in, connect link                                                                 |
| `/vn/topics`, `/vn/topics/{code}` | Subject tree from `GET /api/v1/public/taxonomy`: served nodes in sibling order with name, description and counts; a node page with its path, children and main documents; no provision text |
| `/connect`                        | Setup for Claude.ai, ChatGPT, Claude Code, Cursor; no secrets                                                                                                                               |
| `/sources`                        | Sources, current release, known gaps (from `GET /api/v1/public/status`)                                                                                                                     |
| `/privacy`, `/terms`, `/llms.txt` | Legal pages ("evidence, not legal advice"; account email stored in Singapore; Google is the identity provider; no questions stored) and agent guidance                                      |

Public pages are rendered from release data at build or cached by Nitro route
rules with a 10-minute window; they make no per-visitor corpus query. Only Nitro
holds that copy. Browsers revalidate public responses by ETag only: every
response, 304 included, carries `Cache-Control: no-cache` and `Vary: Cookie` and
no `Last-Modified`, and Nitro ignores `If-Modified-Since` on these routes, so a
copy in one language never revalidates as the other. Error responses (the fixed
404 and the unavailable page) are never cached, carry no `ETag` and are never
answered with 304.

Nitro serves only known routes, matched exactly (case-sensitive, no trailing
slash): a known path with a trailing slash or repeated slashes is a 308 to the
canonical path without the query, and any other unknown path is a fixed
plain-text 404 without a render. Admission covers every page request, not only
public ones (static assets excepted): 120 per minute per client address, and at
most 4 concurrent server renders of any page, error pages included; a cache hit
renders nothing, and a rejection is a fixed plain-text 429 without a render.

## Signed in

| Route                 | Content                                                                                                                                                                                                                                                                                          |
| --------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `/vn/search`          | Vietnamese query box; filters: nodes picked on the tree, applies-on date, type, issuer. Cards: citation, stamp, verbatim text, relation chips, nodes. Summary bar by validity and gaps. Outside-filter hits follow inside hits with a divider and the total count; paging continues through them |
| `/vn/documents/{key}` | Reader: table of contents, text scrolled to the anchor, side rail with validity, relations, nodes, sources and hashes, release id                                                                                                                                                                |
| `/account`            | Email, sessions, personal tokens (create, name, revoke), authorized MCP clients (revoke), delete account                                                                                                                                                                                         |

A signed-out visit to a signed-in route starts Google login with `next` set to
the path plus only allowlisted query parameters: `release` on the reader and
`node` (a served node code, from a topic page link) on search; every other
parameter is dropped, so a query or form state never reaches a URL
([web API](web-api.md#google-login) states the `next` rules).

## Warnings

Provision-level warnings ([search results](mcp-results.md#warnings)) render as
one sentence each with the acting document as subject, never as a stamp:
Vietnamese `{number} {verb} {target}` with the verbs `bãi bỏ`, `thay thế`,
`ngưng hiệu lực` and `sửa đổi, bổ sung`, English `{number} {verb} {target}` with
repeals, replaces, suspends, amends or supplements. `{target}` is the warning's
`citation` for an exact warning; for an inexact one it is the act's `named` path
written the same way, article first, with the server's labels `Điều`, `Khoản`
and `Điểm` joined by ", " (`dieu-8/khoan-9` as `Điều 8, Khoản 9`,
`dieu-8/khoan-9/diem-a` as `Điều 8, Khoản 9, Điểm a`), followed by "(khoản hoặc
điểm này chưa có trong văn bản đã tải)" / "(this clause or point is not in the
loaded text)". Legal labels are not translated, so the English sentence uses the
same `{target}`. A `vbpl_reference` warning adds "(theo liên kết của VBPL)" /
"(per a VBPL link)". The number links to the acting provision in the reader when
it is in the release; `warnings_omitted` shows as "và {n} cảnh báo khác" / "and
{n} more warnings" linking to the reader. On search cards the warning lines sit
under the stamp and document-level relations stay chips; in the reader each
warning sits beside its target provision's label and the rail lists them under
"Cảnh báo" / "Warnings", grouped by target in document order.

## Routing

The website and the API share one origin, because the session cookie is
`__Host-session` and unsafe requests are checked against `SERVER_PUBLIC_URL`.
The edge sends `/api/`, `/mcp`, `/oauth/` and `/.well-known/oauth-` paths to the
Go server and every other path to Nuxt; `/healthz` and `/readyz` are not
exposed. In local development Nuxt runs on `http://localhost:3000` and its dev
proxy sends `/api/*` to the Go server, which runs with
`SERVER_PUBLIC_URL=http://localhost:3000`.

## Visual system

The product display name is "Tra Chuẩn" in both languages (header, page titles,
login, legal pages, the OAuth consent page, agent guidance), matching the Google
consent branding; code, routes, identifiers, the MCP server name and setup
commands keep `trachuan`. Be Vietnam Pro for interface text, a serif for
verbatim law, monospace for document numbers. The CSP is a static header with
`script-src 'self'` (no `'unsafe-inline'`, no `'unsafe-eval'`) and the hydration
payload externalized into a same-origin script; per-request nonces are not used
because public HTML is cached for ten minutes and a cached page would replay an
old nonce. Four validity inks: in force, partly in force, not yet in force,
expired. Theme follows the device. Self-hosted fonts; CSP with no third-party
host. Phase 2: dossiers, watch lists, exports.
