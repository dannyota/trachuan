# Retrieval and the MCP interface

One engine, two faces: `/api/v1/*` for the website (cookie session and CSRF
token) and `/mcp` for agents (bearer token). Same functions, rules and limits.

## Query validation

Accept Vietnamese text with diacritics or a document-number pattern such as
`50/2024/TT-NHNN`. Reject Vietnamese without diacritics with a request to add
them, and other text with `abstain: true` and a reason. Never translate.

## Ranking

1. Dense arm: embed the query with the release's dense recipe, HNSW cosine
   candidates.
2. Lexical arm: BM25 over `sparse` using the release's lexical recipe.
3. Reciprocal rank fusion. Group by provision. Candidate pools and k are in
   [budgets](budgets.md).
4. Abstain when the top cosine score is below the threshold calibrated for the
   recipe and recorded in the release.

These are banhmi's measured settings and the starting point.

## Validity

`in_force_only` defaults to true. `as_of` filters document `eff_from` and
`eff_to`. Documents without an effective date stay in the results with a gap
"missing effective date". When `in_force_only` is false, expired results form a
separate group after current ones. Provision-level repeal relations appear as
warnings on the card, never as a changed stamp.

## Subject filter contract

Retrieval always runs over the whole country corpus. The response carries one
ranked list where each hit has `in_filter: true|false`. Hits inside the
requested `nodes` come first in rank order, then outside hits in rank order,
up to `limit`; `outside_filter_count` gives the total outside. A cursor pages
through the rest, so every hit the unfiltered search would return stays
reachable.

The filter changes order, so the recall guarantee is stated per case: for a
golden case whose expected citations lie inside the selected nodes, filtered
recall@8 must be at least the unfiltered recall@8 of that case; for every
other case, every expected citation must appear in the paged list within the
first 100 hits. Unfiltered recall@8 on the full golden set is unchanged by the
feature. Evaluation uses this exact response.

## Release pinning

Every response carries `release`. `search` and `document` accept an optional
`release`; the server keeps the previous release database for 7 days after
activation so a follow-up read can pin it. A pinned release that is gone
returns an explicit `release_expired` error, never silently different text.

## MCP tools

| Tool            | Input                                                                                                       | Output                                                                                                                        |
| --------------- | ----------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------- |
| `guide`         | `country`                                                                                                   | Usage, Vietnamese rule, citation format, suggested flow `taxonomy` then `search` with `nodes` then `document`                  |
| `corpus_status` | `country`                                                                                                   | Release, counts of documents, provisions, nodes and gaps, newest source date                                                  |
| `taxonomy`      | `country`, `node` code or empty, `depth`, optional `q`                                                      | Subtree with `code`, `name`, `description`, provision and document counts; `q` searches names and descriptions                |
| `search`        | `country`, `query`, `nodes[]`, `as_of`, `in_force_only`, `doc_types[]`, `issuers[]`, `documents[]`, `limit`, `cursor`, `release` | Hits with canonical citation, stamp, verbatim text, nodes, relation summary, source URL, score, `in_filter`; plus `outside_filter_count`, `gaps[]`, `abstain`, `release`, `next_cursor` |
| `document`      | `country`, `key` or number, optional `anchor`, `from`, `to`, `release`                                      | Metadata, table of contents, the requested provision, or the provisions between two anchors when the full text exceeds the response cap, relations with affected provisions, sources and hashes, nodes |
| `quality_gaps`  | `country`, `kind`, `node`, `cursor`                                                                         | Paged gap list                                                                                                                |

Tool schemas grow only additively. `/mcp` requires scope `mcp:read` and serves
RFC 9728 metadata at `/.well-known/oauth-protected-resource`.

## Web API

`POST /api/v1/search`, `GET /api/v1/taxonomy`, `GET /api/v1/documents/{key}`,
auth and account endpoints. The OpenAPI document in `packages/schema` is the
contract; the Nuxt client is generated from it and CI fails on drift.
Limits are in [budgets](budgets.md).
