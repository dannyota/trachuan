# Website

Decision: [ADR 0007](../adr/0007-nuxt-frontend.md). Routes are English with a
country segment; content defaults to Vietnamese with optional English. Legal
text is never translated.

## Public, no account

| Route                             | Content                                                                                                                                                |
| --------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `/`                               | Purpose, one real pre-rendered sample result, coverage facts from `GET /api/v1/public/status`, Google sign-in, connect link                            |
| `/vn/topics`, `/vn/topics/{code}` | Subject tree: name, description, counts, main documents per node; no provision text                                                                    |
| `/connect`                        | Setup for Claude.ai, ChatGPT, Claude Code, Cursor; no secrets                                                                                          |
| `/sources`                        | Sources, current release, known gaps (from `GET /api/v1/public/status`)                                                                                |
| `/privacy`, `/terms`, `/llms.txt` | Legal pages ("evidence, not legal advice"; account email stored in Singapore; Google is the identity provider; no questions stored) and agent guidance |

Public pages are rendered from release data at build or cached by Nitro route
rules with a 10-minute window; they make no per-visitor corpus query. Only Nitro
holds that copy. Browsers revalidate public responses by ETag only: every
response, 304 included, carries `Cache-Control: no-cache` and `Vary: Cookie` and
no `Last-Modified`, and Nitro ignores `If-Modified-Since` on these routes, so a
copy in one language never revalidates as the other.

## Signed in

| Route                 | Content                                                                                                                                                                                                                                                                                          |
| --------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `/vn/search`          | Vietnamese query box; filters: nodes picked on the tree, applies-on date, type, issuer. Cards: citation, stamp, verbatim text, relation chips, nodes. Summary bar by validity and gaps. Outside-filter hits follow inside hits with a divider and the total count; paging continues through them |
| `/vn/documents/{key}` | Reader: table of contents, text scrolled to the anchor, side rail with validity, relations, nodes, sources and hashes, release id                                                                                                                                                                |
| `/account`            | Email, sessions, personal tokens (create, name, revoke), authorized MCP clients (revoke), delete account                                                                                                                                                                                         |

## Deferred to taxonomy

The topics pages (`/vn/topics`, `/vn/topics/{code}`), the node filter, node
chips, outside-filter hits and the type and issuer filters need the taxonomy API
and arrive with it ([plans](../plans/README.md), Plan 6). Until then search
offers the query, the applies-on date and the in-force switch.

## Routing

The website and the API share one origin, because the session cookie is
`__Host-session` and unsafe requests are checked against `SERVER_PUBLIC_URL`.
The edge sends `/api/`, `/mcp`, `/oauth/` and `/.well-known/oauth-` paths to the
Go server and every other path to Nuxt; `/healthz` and `/readyz` are not
exposed. In local development Nuxt runs on `http://localhost:3000` and its dev
proxy sends `/api/*` to the Go server, which runs with
`SERVER_PUBLIC_URL=http://localhost:3000`.

## Visual system

Be Vietnam Pro for interface text, a serif for verbatim law, monospace for
document numbers. The CSP is a static header with `script-src 'self'` (no
`'unsafe-inline'`, no `'unsafe-eval'`) and the hydration payload externalized
into a same-origin script; per-request nonces are not used because public HTML
is cached for ten minutes and a cached page would replay an old nonce. Four
validity inks: in force, partly in force, not yet in force, expired. Theme
follows the device. Self-hosted fonts; CSP with no third-party host. Phase 2:
dossiers, watch lists, exports.
