# ADR 0001: Reuse banhmi data, not code

Status: accepted 2026-11-08.

## Context

banhmi (archived 2026-11-01) built six country corpora, a hybrid retrieval
recipe with measured recall, and golden evaluation sets. Its code grew into 20+
specifications and a Temporal pipeline that never shipped the redesigned
product.

## Decision

traquydinh is a new codebase. It imports banhmi's corpus dumps, source lists,
golden cases and extraction knowledge. It copies no banhmi Go packages.

## Consequences

Faster to hold in one head; retrieval baselines stay comparable because the data
and evaluation are the same. Any banhmi behavior we want must be rewritten.

## Alternatives rejected

Reuse the Go engine (inherits the complexity). Greenfield including data (loses
months of corpus and evaluation work).

## History

- 2026-11-08: accepted in the brainstorming conversation.
