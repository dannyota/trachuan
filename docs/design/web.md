# Website

Decision: [ADR 0007](../adr/0007-nuxt-frontend.md). Routes are English with a
country segment; content defaults to Vietnamese with optional English. Legal
text is never translated.

## Public, no account

| Route                             | Content                                                                                                                                                |
| --------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `/`                               | Purpose, one real pre-rendered sample result, coverage facts, Google sign-in, connect link                                                             |
| `/vn/topics`, `/vn/topics/{code}` | Subject tree: name, description, counts, main documents per node; no provision text                                                                    |
| `/connect`                        | Setup for Claude.ai, ChatGPT, Claude Code, Cursor; no secrets                                                                                          |
| `/sources`                        | Sources, current release, known gaps                                                                                                                   |
| `/privacy`, `/terms`, `/llms.txt` | Legal pages ("evidence, not legal advice"; account email stored in Singapore; Google is the identity provider; no questions stored) and agent guidance |

Public pages are rendered from release data at build or cached by Nitro route
rules with a 10-minute window; they make no per-visitor corpus query.

## Signed in

| Route                 | Content                                                                                                                                                                                                                                                                                          |
| --------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `/vn/search`          | Vietnamese query box; filters: nodes picked on the tree, applies-on date, type, issuer. Cards: citation, stamp, verbatim text, relation chips, nodes. Summary bar by validity and gaps. Outside-filter hits follow inside hits with a divider and the total count; paging continues through them |
| `/vn/documents/{key}` | Reader: table of contents, text scrolled to the anchor, side rail with validity, relations, nodes, sources and hashes, release id                                                                                                                                                                |
| `/account`            | Email, sessions, personal tokens (create, name, revoke), authorized MCP clients (revoke), delete account                                                                                                                                                                                         |

## Visual system

Be Vietnam Pro for interface text, a serif for verbatim law, monospace for
document numbers. Four validity inks: in force, partly in force, not yet in
force, expired. Theme follows the device. Self-hosted fonts; CSP with no
third-party host. Phase 2: dossiers, watch lists, exports.
