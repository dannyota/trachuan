# System

traquydinh serves a citable corpus of Vietnamese law to AI agents over MCP and
to people on the web, behind Google login. Evidence only; see
[ADR 0002](../adr/0002-evidence-only-vietnamese-queries.md). This note holds the
component shape and the data-flow rules. Decisions and alternatives live in the
[ADRs](../adr/README.md).

## Success criteria

- An agent in Claude.ai, ChatGPT or Claude Code browses the subject tree,
  searches Vietnamese provisions filtered by subject, and reads a provision
  with validity and amendment context, behind Google login.
- A signed-in person does the same on the website.
- Vietnam retrieval holds the golden-set floors in [testing](../../instructions/testing.md).
- A subject filter never lowers recall on the golden set under the per-case
  contract in [retrieval](retrieval.md) "Subject filter contract".
- Recurring hosting stays within the [budget](budgets.md).

## Serving host

One AWS Graviton host runs five containers under rootless Podman Quadlet
([ADR 0006](../adr/0006-single-host-aws-podman.md)):

```
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
serves cached pages, bounds the cache to the known public routes and node
codes, and renders at most four pages concurrently; everything else waits or
receives 429.

## Write path

Corpus releases are built on the owner's MacBook by the `corpus` worker under a
self-hosted Temporal server ([pipeline](pipeline.md),
[ADR 0008](../adr/0008-write-path-temporal-self-hosted.md)). A release is a
signed manifest plus a PostgreSQL dump in S3. Activation on the host is a
separate command run by the owner ([deployment](deployment.md)).

## Database roles

| Role         | Where        | May                                                   | May not                         |
| ------------ | ------------ | ----------------------------------------------------- | ------------------------------- |
| `server`     | host         | read any `corpus_*`; read and write `auth` tables     | create or drop databases        |
| `activator`  | host, CLI    | create and drop `corpus_*` databases; write `auth.corpus_release` only | read other `auth` tables |
| `restorer`   | host, CLI    | restore into one new `corpus_*` database              | touch `auth`; superuser rights  |
| `pipeline`   | MacBook      | read and write the local workshop database            | connect to the host             |

The server never writes corpus data. The pipeline never connects to the host.
Activation has its own narrow identity; this amends the earlier wording that
"the pipeline never touches auth" ([ADR 0003](../adr/0003-postgres-per-release-corpus.md) History).

## Repository layout

```
apps/server      Go: API, MCP, OAuth, auth, retrieval
apps/web         Nuxt 4: public pages, search, reader, account
apps/corpus      Go: Temporal worker, stage activities, release and activation commands
packages/schema  OpenAPI document and generated TypeScript client
deploy/aws       OpenTofu root and modules
deploy/host      cloud-init, install.sh, Quadlet units, pgBackRest, alerts
deploy/compose.yml  local development: PostgreSQL, Temporal, UI
docs             design, ADRs, plans, runbooks, standards, research
```
