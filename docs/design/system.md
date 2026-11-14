# System

trachuan serves a citable corpus of Vietnamese law to AI agents over MCP and to
people on the web, behind Google login. Evidence only; see
[ADR 0002](../adr/0002-evidence-only-vietnamese-queries.md). This note holds the
component shape and the data-flow rules. Decisions and alternatives live in the
[ADRs](../adr/README.md).

## Success criteria

- An agent in Claude.ai, ChatGPT or Claude Code browses the subject tree,
  searches Vietnamese provisions filtered by subject, and reads a provision with
  validity and amendment context, behind Google login.
- A signed-in person does the same on the website.
- Vietnam retrieval holds the golden-set floors in
  [testing](../../instructions/testing.md).
- A subject filter never lowers recall on the golden set under the per-case
  contract in [retrieval](retrieval.md) "Subject filter contract".
- Recurring hosting stays within the [budget](budgets-host.md).

## Serving host

The host selected in [ADR 0006](../adr/0006-single-host-oci-podman.md) runs five
containers under rootless Podman Quadlet:

```text
Browser / MCP client
        │ HTTPS 443
      caddy      TLS and reverse proxy only
        ├── /                        → web       Nuxt 4 SSR
        ├── /api/v1/*                → server    Go
        ├── /mcp                     → server    Go, MCP Streamable HTTP
        └── /oauth/*, /.well-known/* → server    Go, OAuth 2.1 AS, RFC 9728
      server ──► embedder   query embedding, container network only
      server ──► postgres
                   ├── auth                accounts, sessions, tokens, release pointer
                   └── corpus_<cc>_<rel>   one database per corpus release, read only
```

Caddy has no rate limiter and gets none. Authentication and the limits for
`/api/v1` and `/mcp` live in `server`. Admission for public pages lives in
`web`: Nitro middleware applies the per-IP limit from [budgets](budgets.md),
serves cached pages, bounds the cache to the known public routes and node codes,
and caps concurrent renders at the value in budgets; an uncached request above
the cap receives 429 at once, with no queue.

`server` listens on loopback behind Caddy, so every `/mcp` request arrives with
the public Host. The MCP SDK's localhost Host check (DNS rebinding protection
for unauthenticated local servers) is therefore off: it would reject every
proxied request with 403. Caddy serves only the configured site names, and
`/mcp` requires a bearer token, so a rebinding page holds no credential.

## Write path

Corpus releases are built on the owner's MacBook by the `corpus` worker under a
self-hosted Temporal server ([pipeline](pipeline.md),
[ADR 0008](../adr/0008-write-path-temporal-self-hosted.md)). A release is a
signed manifest plus a PostgreSQL dump in OCI Object Storage. A host timer
activates it ([corpus release](corpus-release.md)).

## Database roles

| Role             | Where             | May                                                                                                                    | May not                             |
| ---------------- | ----------------- | ---------------------------------------------------------------------------------------------------------------------- | ----------------------------------- |
| `server`         | host              | read any `corpus_*`; read and write `auth` tables                                                                      | create or drop databases            |
| `activator`      | host, CLI         | create and drop `corpus_*` databases; write `auth.corpus_release`, `auth.release_journal`, `auth.retired_release` only | read or write other `auth` tables   |
| `restorer`       | host, CLI         | restore into one new `corpus_*` database                                                                               | touch `auth`; superuser rights      |
| `pipeline`       | MacBook           | read and write the local workshop database                                                                             | connect to the host                 |
| `corpus_builder` | MacBook, proposed | create and own local corpus build databases; separate connection                                                       | read auth/workshop; connect to host |

The server never writes corpus data. The pipeline never connects to the host.
The proposed local builder needs owner approval for CREATEDB; its actual scope
and name guards are in [index construction](index.md#database-lifecycle).
Activation has its own narrow identity; this amends the earlier wording that
"the pipeline never touches auth"
([ADR 0003](../adr/0003-postgres-per-release-corpus.md) History).

## Repository layout

```text
apps/server      Go: API, MCP, OAuth, auth; imports shared retrieval
apps/web         Nuxt 4: public pages, search, reader, account
apps/corpus      Go: Temporal worker, stage activities, release and activation commands
apps/queryembed  Go: query embedding service, ONNX Runtime CPU (read path, host)
apps/docembed    Python, uv: document embedding on the MacBook GPU (write path)
internal/retrieval  Go: shared search and lexical encoding
internal/corpusdb   Go: embedded corpus schema template and schema version
packages/schema  OpenAPI document and generated TypeScript client
deploy/oci       OpenTofu root and modules
deploy/host      cloud-init, install.sh, Quadlet units, pgBackRest, alerts
deploy/local     native local development: PostgreSQL 18, Temporal dev server
docs             design, ADRs, plans, runbooks, standards, research
```
