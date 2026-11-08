# ADR 0003: PostgreSQL with one database per corpus release

Status: accepted 2026-11-08.

## Context

The corpus is 142k chunks with dense and sparse vectors, a structure tree, relations and validity dates. banhmi measured 93.8% recall on PostgreSQL with pgvector HNSW, sparsevec BM25 and reciprocal rank fusion.

## Decision

PostgreSQL with pgvector in a container on the host. Each corpus release is its own database (corpus_<cc>_<rel>) activated by a pointer; accounts and tokens live in a separate auth database that is never reloaded. The server never writes corpus data; the pipeline never touches auth.

## Consequences

Releases are atomic and reversible. Vector dimensions and model belong to the release recipe. Two databases to back up; pgBackRest handles both.

## Alternatives rejected

SQLite with sqlite-vec (no ANN index, re-measure everything). A separate search engine such as Meilisearch or Qdrant (second service, duplicated data). Managed RDS (cost, less control).

## History

- 2026-11-08: accepted in the brainstorming conversation; recorded with the
  design spec.
