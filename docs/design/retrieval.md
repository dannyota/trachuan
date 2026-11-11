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
first recipe is a baseline, with a provisional old-product threshold; the
development cohort precedes recipe changes under ADR 0010 item 4.

## Validity

`in_force_only` defaults to true. Document dates, missing-date gaps, historical
filtering and the separate non-current group follow
[recipe 1](retrieval-recipe.md#validity-and-evidence). Provision-level repeal
relations appear as warnings, never as a changed stamp.

## Filter contract

Filters reorder, never hide. `nodes[]`, `doc_types[]`, `issuers[]` and
`documents[]` form one predicate: a hit is inside when it matches every kind the
request names, and a kind matches when any of its values does. A node matches
when the hit provision has an effective assignment in that node's subtree
([taxonomy](taxonomy.md#serving)); a node code that is not served is
`invalid_request`. `doc_types` and `issuers` match the hit document's `type` and
`issuer` after trimming and NFC normalization; `documents` match document keys.
`corpus_status` lists the release's document types and issuers with counts, so
callers pick values that exist.

Retrieval always runs over the whole country corpus. When any filter is given,
the response carries one ranked list where each hit has `in_filter: true|false`:
inside hits come first in rank order, then outside hits in rank order, up to
`limit`; `outside_filter_count` gives the total outside. `next_cursor` pages
through the rest of the same list, so every hit the unfiltered search would
return stays reachable. The cursor is opaque and holds the release, the offset
and a SHA-256 digest of the request without `cursor` and `limit`, never query
text; a cursor whose digest or release does not match the request is
`invalid_request` or `release_expired`. Without filters the list is the
unfiltered ranking and the same cursor pages it.

The filter changes order, so the recall guarantee is stated per case: for a
golden case whose expected citations lie inside the selected nodes, filtered
recall@8 must be at least the unfiltered recall@8 of that case; for every other
case, every expected citation must appear in the paged list within the first 100
hits. Unfiltered recall@8 on the full golden set is unchanged by the feature.
Evaluation uses this exact response.

## Release pinning

Every response carries `release`. `search` and `read` accept an optional
`release`. The server serves only the active release of each country, so a
pinned release that is no longer active returns an explicit `release_expired`
error, never silently different text.

## MCP tools

| Tool            | Input                                                                                                                                      | Output                                                                                                                                                                                                                                                                                                                                                                                     |
| --------------- | ------------------------------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `guide`         | `country`                                                                                                                                  | Usage, Vietnamese rule, citation format, the reading flow in [search results](mcp-results.md#guide), suggested flow `taxonomy` then `search` with `nodes` then `read`                                                                                                                                                                                                                      |
| `corpus_status` | `country`                                                                                                                                  | Release, counts of documents, provisions, served nodes and gaps, newest source date, document types and issuers with counts                                                                                                                                                                                                                                                                |
| `taxonomy`      | `country`, `node` code or empty, `depth`, optional `q`                                                                                     | Served subtree in sibling order with `code`, `name`, `description`, `level`, `origin`, provision and document counts, to `depth` levels below `node` (roots when empty); `q` instead returns served nodes whose name or description contains it, case and diacritics folded                                                                                                                |
| `search`        | `country`, `query`, `nodes[]`, `as_of`, `in_force_only`, `doc_types[]`, `issuers[]`, `documents[]`, `detail`, `limit`, `cursor`, `release` | `documents` once per document with metadata, status and related pointers; ranked hits with handle, citation path, verbatim snippet and `complete`, nodes, warnings, `in_filter`; `detail:"full"` returns recipe text, fragments, score and relations instead of the snippet; plus `outside_filter_count`, `gaps[]`, `abstain`, `release`, `next_cursor` ([search results](mcp-results.md)) |
| `read`          | `country`, `items[]` of `handle`, number, `from`/`to` range, `relations`, or `next` token; `release`                                       | Per item: a document's metadata and outline, a provision's verbatim text with ancestor lead-ins and warnings, or its relation list; a per-call cap with fixed parts and opaque `next` tokens, so every call makes progress ([search results](mcp-results.md#read))                                                                                                                         |
| `quality_gaps`  | `country`, `kind`, `node`, `cursor`                                                                                                        | Paged gap list                                                                                                                                                                                                                                                                                                                                                                             |

Tool schemas grow only additively. `/mcp` requires scope `mcp:read` and serves
RFC 9728 metadata at `/.well-known/oauth-protected-resource`.

## Web API

`POST /api/v1/search`, `GET /api/v1/public/taxonomy`,
`GET /api/v1/documents/{key}`, auth and account endpoints
([web API](web-api.md)). The OpenAPI document in `packages/schema` is the
contract; the Nuxt client is generated from it and CI fails on drift. Limits are
in [budgets](budgets.md).
