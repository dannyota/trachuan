# Retrieval and the MCP interface

One engine, two faces: `/api/v1/*` for the website (cookie session and CSRF
token) and `/mcp` for agents (bearer token). Same functions, rules and limits.

## Query validation

Accept Vietnamese text with diacritics or a document-number pattern such as
`50/2024/TT-NHNN`. The exact character gate, bounded number grammar and
rejection message are in the
[retrieval recipe](retrieval-recipe.md#validation-and-routing). A character gate
does not establish the language or subject of a question; evidence-based
abstention handles unsupported accepted text. Never translate.

## Ranking

The shared Go engine implements the [retrieval recipe](retrieval-recipe.md):
dense cosine and BM25, weighted reciprocal rank fusion, provision grouping,
document diversity and explicit abstention. [Budgets](budgets.md#retrieval) owns
parameters; [evaluation](evaluation.md) owns calibration and measurement. The
first recipe is a baseline, with a provisional old-product threshold; the
development cohort precedes recipe changes under ADR 0010 item 4.

## Validity

`in_force_only` defaults to true. Document dates, missing-date gaps, historical
filtering and the separate non-current group follow the
[retrieval recipe](retrieval-recipe.md#validity-and-evidence). Provision-level
repeal relations appear as warnings, never as a changed stamp.

## Filter contract

Filters reorder, never hide; `nodes[]`, `doc_types[]`, `issuers[]` and
`documents[]` are ordering preferences. Together they form one predicate: a hit
is inside when it matches every kind the request names, and a kind matches when
any of its values does. A node matches when the hit provision has an effective
node in that node's subtree ([taxonomy](taxonomy.md#serving)); a node code that
is not served is `invalid_request`. `doc_types` and `issuers` match the hit
document's `type` and `issuer` after trimming and NFC normalization; `documents`
match document keys. `corpus_status` lists the release's document types and
issuers, normalized the same way, with their document counts over the classes
search ranks by default (all but `relation_context`), so callers pick values
that exist.

Retrieval always runs over the whole country corpus. Paging walks one list in
two groups, primary and non-current. Each group's sequence is the unfiltered
walk: that group's hits on today's response of the first page's `limit`, then
the rest of the recipe's hits of that group shaped at the list cap of 100, in
rank order, without those already listed. Each page holds at most `limit` hits,
split into `hits` and `non_current` by group. Without filters the first page is
exactly today's response of that `limit`. Every other page fills from what
remains of each sequence: up to the reserved non-current count from the
non-current group, the effective non-current tail with `in_force_only` true and
half of `limit` rounded down with `in_force_only` false
([retrieval recipe](retrieval-recipe.md)), and the rest from primary; in both
modes either group then takes the slots the other cannot fill. When any filter
is given, each sequence is reordered stably, inside hits first, then outside
hits; the filtered first page takes as many hits from each group as today's
first page does and fills any slots left by the same rule, so every inside hit
of today's first page stays on it; every hit carries `in_filter: true|false`;
and `outside_filter_count` counts outside hits over both groups. Every page
carries the first page's `abstain`, `reason` and `message`; filters and paging
never change them. `next_cursor` continues while either sequence has hits left,
so every hit the unfiltered search would return stays reachable exactly once.
The cursor is opaque and holds the release, the first page's `limit`, the number
of hits taken so far from each sequence, the resolved `as_of` date and a SHA-256
digest of the request without `cursor` and `limit`, never query text; later
pages use the cursor's date and rebuild the sequences from it, and a cursor
whose digest or release does not match is `invalid_request` or
`release_expired`.

The filter changes order, so the recall guarantee is stated per case: for a
golden case whose expected citations lie inside the selected nodes, filtered
recall@8 must be at least the unfiltered recall@8 of that case; for every other
case, every expected citation must appear within the first 100 paged hits: the
hits of successive pages of `limit` 8 from the first, each page's `hits` before
its `non_current`. Unfiltered recall@8 on the full golden set is unchanged by
the feature. Evaluation uses this exact response.

## Release pinning

Every response carries `release`. `search` and `read` accept an optional
`release`. The server serves only the active release of each country, so a
pinned release that is no longer active returns an explicit `release_expired`
error, never silently different text.

## MCP tools

| Tool            | Input                                                                                                                                      | Output                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| --------------- | ------------------------------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `guide`         | `country`                                                                                                                                  | Usage, Vietnamese rule, citation format, the reading flow in [search results](mcp-results.md#guide), suggested flow `taxonomy` then `search` with `nodes` then `read`                                                                                                                                                                                                                                                                                                                             |
| `corpus_status` | `country`                                                                                                                                  | Release, counts of documents, provisions, served nodes and gaps, newest source date, document types and issuers with counts                                                                                                                                                                                                                                                                                                                                                                       |
| `taxonomy`      | `country`, `node` code or empty, `depth`, optional `q`                                                                                     | Served subtree in sibling order with `code`, `name`, `description`, `level`, `origin`, provision and document counts, to `depth` levels below `node` (roots when empty); `q` instead returns served nodes in `node`'s subtree (all when empty) whose name or description contains it, case and diacritics folded, as a flat list in tree order. Either way at most 500 nodes: the subtree fills breadth first in sibling order, the `q` list in tree order, and `truncated` marks that more exist |
| `search`        | `country`, `query`, `nodes[]`, `as_of`, `in_force_only`, `doc_types[]`, `issuers[]`, `documents[]`, `detail`, `limit`, `cursor`, `release` | `documents` once per document with metadata, status and relation counts (related pointers in full results); ranked hits with handle, citation path, verbatim snippet and `complete`, nodes, warnings, `in_filter`; `detail:"full"` returns recipe text, fragments, score and relations instead of the snippet; plus `outside_filter_count`, `gaps[]`, `abstain`, `release`, `next_cursor` ([search results](mcp-results.md))                                                                      |
| `read`          | `country`, `items[]` of `handle`, number, `from`/`to` range, `relations`, or `next` token; `release`                                       | Per item: a document's metadata and outline, a provision's verbatim text with ancestor lead-ins and warnings, or its relation list; a per-call cap with fixed parts and opaque `next` tokens, so every call makes progress ([search results](mcp-results.md#read))                                                                                                                                                                                                                                |
| `quality_gaps`  | `country`, `kind`, `node`, `cursor`                                                                                                        | Paged gap list                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    |

Tool schemas grow only additively. `/mcp` requires scope `mcp:read` and serves
RFC 9728 metadata at `/.well-known/oauth-protected-resource`.

## Web API

`POST /api/v1/search`, `GET /api/v1/public/taxonomy`,
`GET /api/v1/documents/{key}`, auth and account endpoints
([web API](web-api.md)). The OpenAPI document in `packages/schema` is the
contract; the Nuxt client is generated from it and CI fails on drift. Limits are
in [budgets](budgets.md).
