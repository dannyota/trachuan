# ADR 0003: PostgreSQL with one database per corpus release

Status: accepted 2026-11-08.

## Context

The corpus is 142k chunks with dense and sparse vectors, a structure tree,
relations and validity dates. banhmi measured 93.8% recall on PostgreSQL with
pgvector HNSW, sparsevec BM25 and reciprocal rank fusion.

## Decision

PostgreSQL with pgvector in a container on the host. Each corpus release is its
own database (`corpus_<cc>_<rel>`) activated by a pointer; accounts and tokens
live in a separate auth database that is never reloaded. The server never writes
corpus data; the pipeline never connects to the host. Release activation uses a
dedicated `activator` role limited to creating, restoring and dropping corpus
and frameworks databases; in `auth` it has only the activation lock and catalog
reads. The updater reads and writes the activation tables in `auth` as
`auth_owner`, the role that owns them.

## Consequences

Releases are atomic and reversible. Vector dimensions and model belong to the
release recipe. Two databases to back up; pgBackRest handles both.

## Alternatives rejected

SQLite with sqlite-vec (no ANN index, re-measure everything). A separate search
engine such as Meilisearch or Qdrant (second service, duplicated data). Managed
RDS (cost, less control).

## History

- 2026-11-08: accepted in the brainstorming conversation.
- 2026-11-08: added the `activator` role after design review finding S-01, which
  showed activation could not both write the pointer and obey "the pipeline
  never touches auth".
- 2026-11-08: widened the activator to the journal and retirement tables after
  finding S-25, so the recovery procedure can run under its own role.
- 2026-11-18: took the activation tables away from `activator`. `auth_owner`,
  the role the internet-facing server uses, owns them, so a compromised server
  could plant a trigger or view that ran as `activator`, which owns every corpus
  database. The updater now uses `auth_owner` for them and keeps each
  operation's record on the host, so a table row cannot drive it.
