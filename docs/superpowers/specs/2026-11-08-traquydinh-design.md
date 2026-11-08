# traquydinh design

Status: approved in conversation on 2026-11-08, pending written review.
Language: documents, code, commits in English; product interface in Vietnamese with optional English.

## 1. Purpose

traquydinh is a citable corpus of Vietnamese law served to two kinds of readers:

- AI agents over the Model Context Protocol (MCP), who need to narrow a search before
  retrieving evidence.
- People through a website, who need to find a provision, see whether it applies on a date,
  and read the source.

It serves evidence, never answers. No answer model, generative model, or reranker runs on our
servers. The user's own agent reads and judges the evidence. The only model we serve is the
query embedder.

traquydinh succeeds [banhmi](https://github.com/dannyota/banhmi) (archived 2026-11-01). It reuses
banhmi's data (corpus dumps, source lists, golden evaluation cases) and none of its code.
banhmi's code grew to 20+ specifications and a Temporal pipeline that never shipped the
redesigned product. traquydinh keeps a codebase one person can hold in their head.

### Success criteria

- An agent connected through Claude.ai, ChatGPT or Claude Code can browse the subject tree,
  search Vietnamese provisions filtered by subject, and read a provision with its validity and
  amendment context, all behind Google login.
- A signed-in person can do the same on the website.
- Vietnam retrieval holds banhmi's accepted floors on the golden set: recall@8 at least 0.90,
  MRR@8 at least 0.66, current-law precision at least 0.99, abstention at least 0.95.
- Adding a subject filter never lowers recall on the golden set.
- Hosting cost stays near USD 60 per month.

## 2. Decisions

| Area | Decision | Rejected |
|---|---|---|
| Reuse from banhmi | Corpus dumps, source lists, golden cases, extraction knowledge | Go engine, Temporal, bronze/silver/gold schema |
| Countries | Vietnam first; taxonomy, keys and routes carry a country code from day one | Six countries at launch |
| Domain scope | Launch with banking, finance, payments, securities, personal data, cybersecurity; tree designed so adding a domain is data, not code | All Vietnamese law at launch |
| Subject taxonomy | Two axes. Subject tree from Bộ pháp điển (levels 1 to 4) plus custom nodes; structure tree from the documents themselves | Hand-written tree; pure embedding zero-shot classification |
| Login | Google only, first login creates the account | Passwords, other providers, approval queue |
| MCP authorization | OAuth 2.1 server embedded in the Go binary, copied from aboutme `internal/oauthsrv` after refactor; CIMD first, pre-registration second, DCR for compatibility | Keycloak (about 1 GB RAM, experimental MCP flags); writing a new server; preregistered-only clients |
| Database | PostgreSQL + pgvector; one database per corpus release; separate auth database | SQLite + sqlite-vec; Meilisearch or Qdrant alongside |
| Hosting | Danny's personal AWS account, ap-southeast-1, ARM64 Graviton, one host | VNGCloud; Vietnamese VPS |
| Containers | Rootless Podman with systemd Quadlet on the host; compose for local dev | Docker and compose on the host; Ansible |
| Frontend | Nuxt 4 SSR + Vue 3, same stack as aboutme.vn | Go templates + htmx; separate Next.js |
| Query language | Vietnamese with diacritics or a document number; no translation | Machine translation of questions |
| Embedding model | Start with Qwen3-Embedding-0.6B to reuse vectors; bake off INT8 Qwen vs Vietnamese_Embedding_v2 locally at the retrieval step | Granite-Embedding-311M (its advantage was multi-language coverage) |
| Classification | Claude Batch API over public legal text, seeded by Bộ pháp điển labels | Fine-tuned classifier; sending user questions anywhere |

Data residency: the only personal data is the Google subject and current email of each account.
Questions are never stored. Public legal text may be processed by cloud batch services. The
privacy notice states that account data is stored in Singapore and that Google is the identity
provider.

## 3. Components

One host runs five containers under systemd Quadlet, rootless, user `app`:

```
Browser / MCP client
        │ HTTPS 443
      caddy   TLS (Let's Encrypt), reverse proxy only
        ├── /                      → web      Nuxt 4 SSR (Node)
        ├── /api/v1/*              → server   Go
        ├── /mcp                   → server   Go, MCP Streamable HTTP
        └── /oauth/*, /.well-known → server   Go, OAuth 2.1 AS + RFC 9728 metadata
      server ──► embedder   Qwen3 ONNX, own process, container network only
      server ──► postgres
                   ├── auth                accounts, sessions, tokens; never reloaded
                   └── corpus_<cc>_<rel>   one database per corpus release, read only
```

Rate limiting, request limits and authentication live in `server`, not in Caddy. Caddy has no
built-in rate limiter.

Outside the host, a Go CLI `corpus` builds releases on the MacBook (see section 7). The server
never writes to a corpus database. The pipeline never touches the auth database.

### Repository layout

```
apps/server      Go: API, MCP, OAuth, auth, retrieval
apps/web         Nuxt 4: public pages, signed-in search and reader, account
apps/corpus      Go CLI: fetch, extract, relate, classify, chunk, embed, build, eval, release
packages/schema  OpenAPI document; generated TypeScript client for web
deploy/aws       OpenTofu root and modules (adapted from aboutme)
deploy/host      cloud-init, install.sh, Quadlet units, pgBackRest, alert scripts
deploy/compose.yml  local development and pipeline services
docs             specs, plans, ADRs, runbooks
```

## 4. Data model

The schema is stated here as tables, keys and principles. Exact DDL belongs to the
implementation plan.

### Corpus database `corpus_<cc>_<rel>` (read only when serving)

| Table | Content |
|---|---|
| `document` | `id`, durable `key` (`vn:vbpl:<ItemID>`), number, title, document type, issuer, issue date, `eff_from`, `eff_to`, validity status from the source, `is_consolidated` |
| `source_file` | `document_id`, `source` (vbpl, congbao, sbv), `url`, `hash`, `role` (main, appendix, scan) |
| `provision` | Structure tree: `id`, `document_id`, `parent_id`, `kind` (text, per-country list: phần, chương, mục, điều, khoản, điểm), `label` (native, e.g. "Điều 5"), `ordinal`, `path` (ltree), `text` verbatim, `anchor` |
| `provision_validity` | Provision-level validity when evidence exists; empty at launch |
| `chunk` | Search unit: `id`, `provision_id`, `text` with context prefix (document title, chapter, article), `embedding` (dimensions from the release recipe, HNSW cosine), `sparse` (BM25 `sparsevec`), `token_count` |
| `relation` | `source_document_id`, `target_document_id`, `kind` (amends, replaces, repeals, basis, guides, consolidates), optional `source_provision_id` and `target_provision_id`, `evidence` verbatim, `method` |
| `taxonomy_node` | `id`, `country`, `parent_id`, `level`, public `code` ("22", "22.3", "22.3.1"), `name`, `description` for agents, `origin` (phapdien, custom), `path` (ltree) |
| `taxonomy_assignment` | `provision_id`, `node_id`, `method` (phapdien_seed, llm, manual), `confidence`, `taxonomy_version` |
| `coverage_gap` | `kind`, `document_id` or `node_id`, message |
| `release` | One row: `rel`, `taxonomy_version`, `embedding_recipe` (model, dims, pooling, prompt, quantization), `source_manifest_hash`, evaluation results, `built_at` |

Principles:

- Search results are provisions, never chunks. Citation and validity come from `document` and
  `provision`.
- A provision can carry several taxonomy assignments.
- Public identity of a taxonomy node is its `code`, stable across versions. Integer ids are
  internal.
- Provision-level repeal is shown as "a repeal relation targets this provision", not as a
  changed validity stamp, until `provision_validity` has evidence.
- Everything that changes over time (model, taxonomy version, source list, evaluation) is a
  property of the release. The server holds no corpus configuration except the active release
  pointer.

### Auth database `auth` (written when serving)

| Table | Content |
|---|---|
| `account` | `id`, `google_sub` unique, current `email`, `blocked_at`, `created_at`, `last_login_at`, `delete_requested_at` |
| `web_session` | rotated session id, `account_id`, expiry |
| `oauth_client`, `oauth_grant`, `oauth_code`, `oauth_token` | as defined by the copied `oauthsrv` store: CIMD or registered clients, consents, codes, access and refresh tokens with `resource`, `scope`, `account_id` |
| `personal_token` | `hash`, `account_id`, `name`, `last_used_at`, `revoked_at` |
| `corpus_release` | `country`, `db_name`, `activated_at` |

No table stores question text. Operational logs record `account_id`, error code and latency,
never the question or the returned text. Logs are kept 30 days.

### Subject taxonomy

Levels 1 to 4 come from the 2023 Bộ pháp điển offline snapshot: 45 chủ đề, 271 đề mục, and the
phần/chương/mục nodes inside each đề mục (90,316 tree nodes). Names and codes are kept. Custom
nodes for subjects the codification lacks (digital payments, personal data, cloud) are declared in
a YAML file in the repository with a one-sentence description each. Changing the tree bumps
`taxonomy_version` and reruns classification.

Verified facts about Bộ pháp điển (2026-11-08): the portal `phapdien.moj.gov.vn` is now an
internal codification tool whose API requires authentication; the public viewer pages return 404.
The 2023 official offline package (`BoPhapDienDienTu.zip`, 33 MB, from github.com/definev/phapdien)
holds 86,388 articles placed in the tree, each linked to a VBPL ItemID and article anchor, across
6,523 distinct documents. Those links join directly to banhmi's `vbpl:<ItemID>` keys.

## 5. Retrieval and the MCP interface

One engine serves two faces: `/api/v1/*` for the website (cookie session, CSRF) and `/mcp` for
agents (bearer token). Same functions, same rules, same per-account limits.

### Retrieval

1. Validate the query: Vietnamese letters with diacritics, or a document number pattern.
   Otherwise return `abstain: true` with a reason. Vietnamese without diacritics is rejected with
   a request to add them.
2. Dense arm: embed the query, HNSW cosine top 50. Lexical arm: BM25 over `sparse`, top 50.
   Fuse with reciprocal rank fusion, k = 60. Group by provision. These are banhmi's measured
   settings and the starting point.
3. Validity: `in_force_only` defaults to true. `as_of` filters document `eff_from`/`eff_to`.
   Documents without an effective date are kept and flagged with a gap rather than dropped.
   Expired results group separately when `in_force_only` is false.
4. Subject filter never hides evidence. Retrieval always runs over the whole country corpus.
   Results are split into inside and outside the requested `nodes`. The response returns the
   inside list, `outside_filter_count`, and the three best outside hits labelled as such. There
   is no hard-cut mode.
5. Abstain when the top cosine score is below the calibrated threshold. Never return an empty
   list without a reason.

### MCP tools

| Tool | Input | Output |
|---|---|---|
| `guide` | `country` | Usage, Vietnamese-only rule, citation format, suggested flow: `taxonomy` → `search` with `nodes` → `document` |
| `corpus_status` | `country` | Release id, counts of documents, provisions, nodes, gaps, newest source date |
| `taxonomy` | `country`, `node` (code or empty for roots), `depth`, optional `q` | Subtree: `code`, `name`, `description`, provision count, document count; `q` searches node names and descriptions |
| `search` | `country`, `query`, `nodes[]`, `as_of`, `in_force_only`, `doc_types[]`, `issuers[]`, `documents[]`, `limit` | Provisions with canonical citation ("khoản 2 Điều 5 Thông tư 50/2024/TT-NHNN"), validity stamp, verbatim text, nodes, relation summary, source URL, score; plus `outside_filter_count`, `gaps[]`, `abstain` |
| `document` | `country`, `key` or number, optional `anchor` | Metadata, table of contents, requested provision or full text, incoming and outgoing relations with affected provisions, sources and hashes, nodes |
| `quality_gaps` | `country`, `kind`, `node` | Gap list so an agent knows which areas not to trust |

Tool schemas only grow additively. `/mcp` requires scope `mcp:read`. Protected resource
metadata is served at `/.well-known/oauth-protected-resource`. Personal tokens and OAuth tokens
pass through the same middleware. Limits: for example 60 searches per minute per account, HTTP 429
with `Retry-After`; request body 64 KB; query 2,000 characters.

### Web API

`POST /api/v1/search`, `GET /api/v1/taxonomy`, `GET /api/v1/documents/{key}`, plus auth and
account endpoints. The OpenAPI document in `packages/schema` is the contract; the Nuxt client is
generated from it and CI fails on drift, as in aboutme.

## 6. Website and login

Routes are English with a country segment. Content defaults to Vietnamese.

Public, no account:

| Route | Content |
|---|---|
| `/` | Purpose, one real pre-rendered sample result, coverage facts, Google sign-in, connect link |
| `/vn/topics`, `/vn/topics/{code}` | Browse the subject tree: name, description, counts, main documents per node; no provision text |
| `/connect` | Setup for Claude.ai, ChatGPT, Claude Code, Cursor; no secrets |
| `/sources` | Sources, current release, known gaps |
| `/privacy`, `/terms`, `/llms.txt` | Legal pages ("evidence, not legal advice") and agent guidance |

Signed in:

| Route | Content |
|---|---|
| `/vn/search` | Vietnamese query box; filters: subject nodes picked on the tree, applies-on date, document type, issuer. Result cards: citation, validity stamp, verbatim text, relation chips, nodes. Summary bar by validity and gaps. "N more outside the filter" expands |
| `/vn/documents/{key}` | Reader: chapter/article table of contents, text scrolled to the anchor, side rail with validity, relations, nodes, sources and hashes |
| `/account` | Email, active sessions, personal tokens (create, name, revoke), authorized MCP clients (revoke), delete account |

Login: Google OpenID Connect handled by the Go server; one Google client for both web and MCP
flows. Identity is `google_sub`; the email is stored only in its current value. First login
creates the account. Session cookie is `HttpOnly`, `Secure`, `SameSite=Lax`, rotated on login,
30-day sliding expiry. Logout destroys the server-side session. The Google consent screen must be
in Production mode with scopes `openid email` only.

Agent flow: the MCP client calls `/mcp` without a token, receives 401 with resource metadata,
opens `/oauth/authorize`; the user signs in with Google if needed; the consent page shows the
client name and domain from its CIMD document and the requested scope; the client exchanges the
code. Loopback redirects `http://127.0.0.1:<any port>` are allowed for native clients per RFC
8252; all other redirect URIs match byte for byte.

Visual direction follows aboutme and the earlier tracuuquydinh design: Be Vietnam Pro for
interface text, a serif for verbatim law, monospace for document numbers; four validity inks
(in force, partly in force, not yet in force, expired); theme follows the device; self-hosted
fonts; CSP with no third-party hosts. Dossiers, watch lists and exports are phase 2.

## 7. Corpus pipeline

`corpus` is a Go CLI with one subcommand per step. Steps exchange JSON files on disk under
`work/<country>/<step>/<doc-key>.json`, each with a `schema_version`. Any step can be rerun
alone and skips inputs whose hash is unchanged. It runs on the MacBook with a local Postgres from
`deploy/compose.yml`.

| Step | Input | Output | Notes |
|---|---|---|---|
| `fetch` | Source list YAML inherited from banhmi (VBPL by issuer and scope terms, SBV) | Raw VBPL JSON, original files, hashes | VBPL needs a Vietnamese IP; the MacBook in Vietnam suffices |
| `extract` | Original files | Structure tree with verbatim text per node | Prefer VBPL HTML, then DOCX; OCR for scans with a gap flag |
| `relate` | VBPL `references` plus text | Relations with kind, evidence, affected provision when a regex captures it | Never guess; leave empty and flag a gap |
| `classify` | Structure tree, taxonomy, seed labels | `taxonomy_assignment` rows | See below |
| `chunk`, `embed` | Structure tree | Article-level chunks (split by clause when too long) with context prefix; dense and sparse vectors | Model from the recipe in config; embed locally or on Kaggle |
| `build` | All of the above | Local `corpus_<cc>_<rel>` database | Schema template; vector dimensions from the recipe |
| `eval` | Local database, banhmi golden set plus new filtered cases | Recall, MRR, current-law precision, abstention, with and without `nodes`; diff report against the previous release | Below floor means `release` refuses |
| `release` | Evaluated database | `corpus_<cc>_<rel>.dump` and signed `manifest.json` in S3 | Manifest: source hashes, recipe, taxonomy version, eval results |

Delta mode: `fetch` asks VBPL for documents new or changed since the previous release; only those
pass through `extract`, `classify`, `embed`; `relate` reruns for referenced documents too;
`build` always produces a full database from the local workshop.

### Classification

Three layers, highest confidence wins:

1. Seed: an article whose VBPL ItemID and article number match the 2023 codification receives
   that node down to level 4, `method = phapdien_seed`, `confidence = 1.0`.
2. LLM, document → chapter → article:
   - One call per document (title, scope article, table of contents, VBPL field) yields its
     level-2 đề mục.
   - One call per chapter with all its articles as context yields assignments for every
     article at once. Candidate nodes come from cosine similarity between the articles' existing
     embeddings and node description embeddings, top 10 across the whole tree, so a data
     protection article inside a banking law can still reach the data node.
   - Output is JSON with codes and confidence; below threshold, no assignment and a gap.
   - Claude Batch API; input is public legal text only.
3. Manual overrides in a YAML file, `method = manual`.

Quality gate: mask 300 seeded articles, reclassify, measure agreement at level 2 and level 4.
Below 90% at level 2, fix the prompt or node descriptions before the full run. The result is
recorded in the manifest.

### Release activation on the host

`release activate <rel>`: download the dump and manifest from S3, verify the signature and that
the manifest's embedding recipe equals the running embedder's recipe (otherwise refuse; a recipe
change is a code deploy), check free memory, `pg_restore` into a new database with a dedicated
`maintenance_work_mem` for the HNSW build, run smoke queries, switch `auth.corpus_release`, and
drop the previous database after 24 hours. The server picks up the pointer on the next request.

## 8. Deployment, security, operations

Host: one EC2 Graviton instance, estimated `t4g.large` (2 vCPU, 8 GB), sized by measurement; gp3
volume; Amazon Linux 2023; rootless Podman; sysctl `net.ipv4.ip_unprivileged_port_start=80`. One
Elastic IP, DNS in Route 53 managed by OpenTofu. No CloudFront or ALB at launch.

Infrastructure code: adapt aboutme's `deploy/aws/modules` (network, host, dns, identity, ops) and
`deploy/vn/host` bundle (cloud-init template, `install.sh`, Quadlet `.container.in` templates,
systemd timers, pgBackRest, alert scripts). No Ansible.

Images: built for linux/arm64 on GitHub Actions, pushed to GHCR, pinned by digest, with a signed
build record (aboutme `provenance.sh`). Base images from Chainguard or distroless.

Deploy: GitHub Actions assumes an IAM role through OIDC and runs `aws ssm send-command`; the host
script updates image references in the Quadlet units, `systemctl daemon-reload`, restarts.
Rollback reverts the tag. No inbound SSH. Corpus release activation is separate from code
deploys.

Secrets: Google client secret, token signing keys, database passwords in SSM Parameter Store,
fetched at deploy into Podman secrets. Signing keys rotate: the server verifies with all current
keys and signs with the newest.

Backups: pgBackRest to a private S3 bucket with continuous WAL archiving, weekly full, daily
differential, weekly verify, 30-day retention, SSE-KMS. Alarm when the newest backup is older
than 26 hours. Monthly automated restore into a scratch database with smoke queries. Accepted
targets: RPO minutes (WAL), RTO about one hour manual on a single host.

Security rules:

- Token checks are local: signature, `aud` equals the canonical `/mcp` URL, expiry, and
  `account.blocked_at` is null; database check cached 10 seconds, so revocation and blocking
  take effect within that window.
- A web cookie never authorizes `/mcp`; a bearer token never authorizes web APIs.
- CIMD fetches go over HTTPS only, refuse private address ranges, time out at 5 seconds, cache
  by URL.
- Limits per account for search and embedding; per IP for login and OAuth endpoints.
- Account deletion revokes all OAuth tokens, personal tokens and sessions in one transaction and
  removes the account row after 7 days.
- `govulncheck`, `npm audit`, Dependabot in CI.

Observability: journald with 30-day retention; health and readiness endpoints; Prometheus metrics
scraped locally; a `watch` systemd timer and Telegram alerts (from aboutme) plus an external
uptime check and CloudWatch alarms on disk, memory and instance status. Public SSR pages are
cached with Nitro route rules so Node does not compete with the embedder under load.

## 9. Testing

| Layer | When | Content |
|---|---|---|
| Unit (Go, Vitest) | every push | Citation formatter, Vietnamese query gate, RRF, inside/outside split, OAuth validators, CIMD resolver |
| Integration with real Postgres | every push, container | Migrations, search over a 50-document sample corpus committed to the repo, end-to-end OAuth flow with a fake client, token and cookie boundaries |
| Corpus evaluation | before every `release`, on the MacBook | Golden set with floors; filtered cases; release diff report; classification agreement |
| Browser (Playwright) | every pull request | Public pages without a session, fake Google login (aboutme dev-https-browser harness), search, reader, personal tokens, both themes, phone and desktop widths |

## 10. Spikes before implementation

1. OAuth clients: run a minimal server with the copied `oauthsrv` and connect for real from
   Claude.ai, ChatGPT connector and Claude Code. aboutme verified only Claude Code. If a client
   fails, fall back to `github.com/giantswarm/mcp-oauth`, then AuthPlane.
2. Embedder on ARM64: Qwen3-Embedding-0.6B INT8 ONNX on Graviton; record RSS, cold start, p95 for
   queries up to 256 tokens. Compare with Vietnamese_Embedding_v2 on the golden set locally.
3. Bộ pháp điển join: parse the 2023 package, join to banhmi's VBPL keys, report coverage of the
   current corpus by seeded articles.

## 11. Out of scope for the first release

Other countries' corpora; dossiers, watch lists, exports; organizations and team accounts; paid
tier; a platform-run model or reranker; translation; query history; Coraza/CrowdSec edge
protection; deployment transparency observer; Pháp điển data served to users (it is only a
taxonomy skeleton and an evaluation seed).

## 12. Open items

- Confirm the Bộ pháp điển coverage after the join spike; decide how many custom nodes the
  finance subjects need.
- Ask Cục Kiểm tra văn bản QPPL (Bộ Tư pháp, 024.62739660) about API or dump access to the
  current codification.
- Wait for the aboutme refactor of `oauthsrv`, `mcpapi` and Google login before copying them.
- Measure the host size and cost; consider a savings plan once the instance type is fixed.

## 13. References

- banhmi: `/Users/danny/src/dannyota/banhmi` (archived), especially `docs/design/website.md`,
  `docs/design/rag.md`, `docs/specs/embedding-models.md`, `deploy/eval/golden.json`.
- aboutme: `/Users/danny/src/dannyota/aboutme`, especially `apps/server/internal/oauthsrv`,
  `docs/design/mcp-cimd-verification.md`, `deploy/vn/host`, `deploy/aws/modules`.
- MCP authorization spec 2026-07-28: https://modelcontextprotocol.io/specification/2026-07-28/basic/authorization
- Keycloak MCP guide (used to reject Keycloak): https://www.keycloak.org/securing-apps/mcp-authz-server
- Bộ pháp điển announcement, 45 chủ đề and 271 đề mục: https://xaydungchinhsach.chinhphu.vn/cong-bo-bo-phap-dien-viet-nam-119241105183017898.htm
- 2023 offline package: https://github.com/definev/phapdien (data/BoPhapDienDienTu.zip)
