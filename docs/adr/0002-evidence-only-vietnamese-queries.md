# ADR 0002: Evidence only; Vietnamese queries; no platform model

Status: accepted 2026-11-08.

## Context

Sending user questions to a model provider abroad is a cross-border transfer
under Vietnam's personal data law (91/2025/QH15, Điều 20). Machine translation
of Vietnamese legal terms is unreliable.

## Decision

Our servers run only the query embedder. The user's own agent reasons over
verbatim evidence. Queries must be Vietnamese with diacritics or a document
number; legal text is never translated. Questions are never stored.

## Consequences

No answer quality to defend; no question data to protect. Users without an AI
client get search and reading, not answers.

## Alternatives rejected

A platform-run answer model or reranker. Translating questions. Query history
features.

## History

- 2026-11-08: accepted in the brainstorming conversation.
