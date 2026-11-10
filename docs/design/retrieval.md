# Retrieval and the MCP interface

One engine, two faces: `/api/v1/*` for the website (cookie session and CSRF
token) and `/mcp` for agents (bearer token). Same functions, rules and limits.

## Query validation

Accept Vietnamese text with diacritics or a document-number pattern such as
`50/2024/TT-NHNN`. The exact character gate, bounded number grammar and
rejection message are in [recipe 1](retrieval-recipe.md#validation-and-routing).
A character gate does not establish the language or subject of a question;
evidence-based abstention handles unsupported accepted text. Never translate.

## Ranking

The shared Go engine implements [retrieval recipe 1](retrieval-recipe.md): dense
cosine and BM25, weighted reciprocal rank fusion, provision grouping, document
diversity and explicit abstention. [Budgets](budgets.md#retrieval) owns
parameters; [evaluation](evaluation.md) owns calibration and measurement. The
first recipe is a baseline, with a provisional old-product threshold; new
development and sealed cohorts precede tuning or superiority claims.

## Validity

`in_force_only` defaults to true. Document dates, missing-date gaps, historical
filtering and the separate non-current group follow
[recipe 1](retrieval-recipe.md#validity-and-evidence). Provision-level repeal
relations appear as warnings, never as a changed stamp.

## Subject filter contract

This contract lands with taxonomy. The index and evaluation work implements
unfiltered search only and reports filtered evaluation as out of scope.

Retrieval always runs over the whole country corpus. The response carries one
ranked list where each hit has `in_filter: true|false`. Hits inside the
requested `nodes` come first in rank order, then outside hits in rank order, up
to `limit`; `outside_filter_count` gives the total outside. A cursor pages
through the rest, so every hit the unfiltered search would return stays
reachable.

The filter changes order, so the recall guarantee is stated per case: for a
golden case whose expected citations lie inside the selected nodes, filtered
recall@8 must be at least the unfiltered recall@8 of that case; for every other
case, every expected citation must appear in the paged list within the first 100
hits. Unfiltered recall@8 on the full golden set is unchanged by the feature.
Evaluation uses this exact response.

## Release pinning

Every response carries `release`. `search` and `document` accept an optional
`release`; the server keeps the previous release database for 7 days after
activation so a follow-up read can pin it. A pinned release that is gone returns
an explicit `release_expired` error, never silently different text.

## MCP tools

| Tool            | Input                                                                                                                            | Output                                                                                                                                                                                                                                                                                                                                                                                                                              |
| --------------- | -------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `guide`         | `country`                                                                                                                        | Usage, Vietnamese rule, citation format, suggested flow `taxonomy` then `search` with `nodes` then `document`                                                                                                                                                                                                                                                                                                                       |
| `corpus_status` | `country`                                                                                                                        | Release, counts of documents, provisions, nodes and gaps, newest source date                                                                                                                                                                                                                                                                                                                                                        |
| `taxonomy`      | `country`, `node` code or empty, `depth`, optional `q`                                                                           | Subtree with `code`, `name`, `description`, provision and document counts; `q` searches names and descriptions                                                                                                                                                                                                                                                                                                                      |
| `search`        | `country`, `query`, `nodes[]`, `as_of`, `in_force_only`, `doc_types[]`, `issuers[]`, `documents[]`, `limit`, `cursor`, `release` | Hits with canonical citation, stamp, verbatim text, nodes, relation summary, source URL, score, `in_filter`; plus `outside_filter_count`, `gaps[]`, `abstain`, `release`, `next_cursor`                                                                                                                                                                                                                                             |
| `document`      | `country`, `key` or number, optional `anchor`, `from`, `to`, `part`, `release`                                                   | Metadata, table of contents, the requested provision, or the provisions from `from` to `to` in document order; when the text cap in [budgets](budgets.md) is reached the response stops at a provision boundary and returns `next_from`; a single provision above the cap is returned in numbered `part`s with `parts` and `next_part`, so every read makes progress; relations with affected provisions, sources and hashes, nodes |
| `quality_gaps`  | `country`, `kind`, `node`, `cursor`                                                                                              | Paged gap list                                                                                                                                                                                                                                                                                                                                                                                                                      |

Tool schemas grow only additively. `/mcp` requires scope `mcp:read` and serves
RFC 9728 metadata at `/.well-known/oauth-protected-resource`.

## Web API

`POST /api/v1/search`, `GET /api/v1/taxonomy`, `GET /api/v1/documents/{key}`,
auth and account endpoints. The OpenAPI document in `packages/schema` is the
contract; the Nuxt client is generated from it and CI fails on drift. Limits are
in [budgets](budgets.md).
