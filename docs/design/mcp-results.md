# Search results and reading for agents

`search` returns compact hits an agent can scan; `read` returns the verbatim
parts the agent chooses, several per call. The decision and its trade-offs are
in [ADR 0013](../adr/0013-compact-search-and-batch-read.md). Ranking,
abstention, validity and hit text come from
[retrieval recipe 1](retrieval-recipe.md); this note owns how they are shaped
for `/mcp` and the matching web API. Numbers are in [budgets](budgets.md).

## Format

Each tool result carries `structuredContent` and the same JSON, serialized
without whitespace, in one text block, as the
[MCP tools specification](https://modelcontextprotocol.io/specification/2025-06-18/server/tools)
recommends for backward compatibility. Each tool declares an `outputSchema`.
Keys are English; legal text is verbatim. Null and empty fields are omitted;
booleans are always present. The web API returns the same objects through the
OpenAPI document.

## Handles

A handle names a document, `<key>`, or a provision, `<key>#<anchor>`, using the
public identity in [corpus schema](corpus-schema.md):
`vn:vbpl:03206030-6f7b-11f1-95c5-bd4e620e4897#dieu-1/khoan-1`. A handle is
stable while that identity exists; the text behind it is pinned by `release`.
Handles never encode a release, a database id or a rank.

## Search response

Input adds `detail`: `compact` (default) or `full`. The default `limit` depends
on `detail`.

Top level: `release`, `abstain`, `reason`, `message`, `documents`, `hits`,
`non_current`, `gaps`, `outside_filter_count`, `next_cursor`. `documents` is an
object keyed by document key and holds every document that a hit in `hits` or
`non_current` names, once.

A document entry has `number`, `title`, `type`, `issuer`, `issued`,
`effective_from`, `effective_to`, `status` (the document part of the recipe
validity stamp: class, current flag, basis, status basis), `source_url`,
`related` and `related_counts`.

A compact hit has, in this order:

| Field       | Meaning                                                                                                                     |
| ----------- | --------------------------------------------------------------------------------------------------------------------------- |
| `handle`    | The hit's provision                                                                                                         |
| `doc`       | Key into `documents`                                                                                                        |
| `citation`  | Canonical citation from the recipe, number excluded                                                                         |
| `path`      | Ancestors from the top structural level down to and including the hit: `label`, `heading` when the source has one, `handle` |
| `snippet`   | Verbatim text under the snippet rule                                                                                        |
| `complete`  | True only when `snippet` is the whole hit text and the recipe marked the hit complete                                       |
| `in_filter` | Subject filter result, only when `nodes` were requested                                                                     |
| `nodes`     | Taxonomy node codes                                                                                                         |
| `warnings`  | Provision-level relations whose endpoint is this provision or a descendant, as pointers, under the recipe's validity rule   |
| `gaps`      | The recipe's hit gaps                                                                                                       |

A full hit has the same fields, then the recipe's `text`, `fragments`, `score`
and complete `relations` with evidence, without `snippet`. Rank order, the
document cap and the `non_current` group follow the recipe in both modes.

## Snippet rule

The snippet is cut from the recipe's hit text and fragments only; it never joins
lexical tokens, paraphrases or adds text.

1. If the hit text is at most the snippet cap, the snippet is the whole text.
2. Otherwise the snippet is the lead fragments in ancestor order, then the body
   of the recipe's best matched member from the start of its first fragment,
   until the snippet cap. Leads take at most half of the cap; a longer lead is
   cut.
3. Every cut ends at the last whitespace at or before its limit. Each omission,
   inside or between spans and at the end, is shown once as `[…]` separated by
   one space from adjacent text; it is the only text not taken from the source.
   A body that does not start at the start of the hit's own text is preceded by
   the marker.

Count code points after the marker is inserted. `complete` is false whenever any
marker appears or the recipe marked the hit incomplete.

## Related pointers

`related` lists relation rows in which the document is the source (`dir:"out"`)
or the target (`dir:"in"`), as stored by `relate`. A pointer has `dir`, `kind`,
`number` of the other document, and `handle` only when that document is in the
release. Order by kind: `repeals`, `replaces`, `suspends`, `amends_supplements`,
`consolidates`, `implements`, `guided_by`, `legal_basis`, `cites`, `references`,
`unknown`; then `in` before `out`; then the other document's issue date, newest
first, unknown dates last; then number. The pointer cap per document is in
budgets. `related_counts` maps `<dir>:<kind>` to the full count, so an omission
is never silent. Pointers carry no evidence text and state no inverse legal
effect; evidence and raw labels are returned by `read`.

## Read

Input: `country`, `items` (each a `handle` or an exact document `number`, with
optional `from`, `to` and `part`), `release`. A number must resolve to exactly
one document; otherwise the item fails with `ambiguous_number` or `not_found`
and lists no guesses.

| Item                       | Returns                                                                                                                                                                                                                |
| -------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Document handle or number  | The document entry and its outline: `phan`, `chuong`, `muc` and `dieu` nodes in document order, or its top-level provisions when it has no `dieu`, each with `label`, `heading`, `handle` and text size in code points |
| Provision handle           | `citation`, `path`, the ancestors' lead fragments, then the provision's own text and descendants by [chunk rendering](chunk.md#tree-rendering), `complete`, `warnings`                                                 |
| `from` and `to` provisions | The provisions from `from` to `to` in document order, each as above                                                                                                                                                    |
| Any item                   | Relation pointers for the read scope, in pointer order, with `raw_label` and evidence, under the relation caps in budgets, and `related_counts` for the rest                                                           |

The read cap in budgets counts returned text across all items of one call. Items
are processed in order. When the cap is reached inside an item, that item stops
at a provision boundary with `next_from`, or inside a single provision above the
cap with `part`, `parts` and `next_part`; later items return
`not_read:"call_cap"`. The first item always returns at least one provision, one
outline page or one part, so every call makes progress. An outline longer than
the cap pages with `next_from` the same way. Per-item failures (`not_found`,
`ambiguous_number`, `release_expired`, `invalid_range`) do not fail the call.
Every result carries `release`.

## Guide

`guide` states the flow: `search`; scan `documents` and `hits`; read the chosen
handles in one `read` call, choosing a `path` handle to read a whole article or
chapter; check `status`, `warnings` and `in` pointers of kinds `repeals`,
`replaces`, `suspends` and `amends_supplements` before relying on a document;
read `in:implements` documents for detailed rules. It says that a snippet is not
the provision, that `[…]` marks omitted text, that pointer direction is as
recorded by the source, and that only text returned complete may be cited.

## Evaluation

Recall, MRR, current-law precision and abstention use the ranked identities and
are equal in both modes. Each trial also reports serialized response size in
code points (p50, p95) for compact at its default limit and full at its default
limit. Compact at limit 20 must not exceed full at limit 8 at p95 on the old
product cohort; otherwise the snippet cap or the compact default changes before
the server ships.

After the server exists, an agent-in-the-loop check runs one Claude and one
OpenAI client model on the development cohort with only the MCP tools, compact
against full. It records expected provisions the agent cites under the product
matcher, tool calls and input tokens. Compact stays the default only if cited
expected provisions do not fall. Sealed cases are never used, and no question
text is stored outside the evaluation files.
