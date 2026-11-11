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
Handles never encode a release, a database id or a rank. Inside one document
context (a hit's `path`, a read item's units and outline) positions are bare
anchors; the document key appears once, as the hit's `doc` or the read item's
`doc`.

## Search response

Input adds `detail`: `compact` (default) or `full`. The default `limit` depends
on `detail`.

Top level: `release`, `abstain`, `reason`, `message`, `documents`, `hits`,
`non_current`, `gaps`, `outside_filter_count`, `next_cursor`. `documents` is an
object keyed by document key and holds every document that a hit in `hits` or
`non_current` names, once.

A document entry has `number`, `title`, `type`, `issuer`, `issued`,
`effective_from`, `effective_to`, `status` (the document part of the recipe
validity stamp: class, current flag, basis, status basis), `text_source` and
`text_complete` ([text extraction](text-extraction.md): whether the text is the
source's tree, a parsed transcription, PDF text or OCR, and whether it is known
complete), `source_url`, `related` and `related_counts`.

A compact hit has, in this order:

| Field       | Meaning                                                                                                                                                   |
| ----------- | --------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `handle`    | The hit's provision                                                                                                                                       |
| `doc`       | Key into `documents`                                                                                                                                      |
| `citation`  | Canonical citation from the recipe, number excluded                                                                                                       |
| `path`      | Ancestors from the top structural level down to and including the hit: `label`, `heading` when the source has one, `anchor` (read as `<doc>#<anchor>`)    |
| `snippet`   | Verbatim text under the snippet rule                                                                                                                      |
| `complete`  | True only when `snippet` is the whole hit text and the recipe marked the hit complete                                                                     |
| `in_filter` | Subject filter result, only when `nodes` were requested                                                                                                   |
| `nodes`     | Taxonomy node codes                                                                                                                                       |
| `warnings`  | Provision-level relations whose endpoint is this provision or a descendant, as pointers, under the recipe's validity rule, in the recipe's relation order |
| `gaps`      | The recipe's hit gaps                                                                                                                                     |

A full hit has the same fields except `snippet`, then the recipe's `text`,
`fragments`, `score` and complete `relations` with evidence; its `complete` is
the recipe's `Complete`, and an incomplete full hit carries a `next` read item
(see Read). Rank order, the document cap and the `non_current` group follow the
recipe in both modes.

## Snippet rule

The snippet is cut from the recipe's hit text only; it never joins lexical
tokens, paraphrases or adds text. The recipe exposes the best matched member as
`Hit.BestMember` and `Hit.BestTextStart`, the offset in `Hit.Text` where that
member's first body span begins.

1. If the hit text is at most the snippet cap, the snippet is the whole text.
2. Otherwise the snippet is the lead fragments in ancestor order, then the text
   from `BestTextStart`, until the snippet cap. Leads take at most half of the
   cap; a longer lead is cut.
3. Every cut ends at the last whitespace at or before its limit; when the span
   has none, at the last code point at or before the limit that does not
   separate a combining mark from its base, or at the limit when no such code
   point exists. Each omission, inside or between spans and at the end, is shown
   once as `[…]` separated by one space from adjacent text; it is the only text
   not taken from the source. Text from `BestTextStart` that does not start at
   the start of the hit's own text is preceded by the marker.

Count code points after the marker is inserted. `complete` is false whenever any
marker appears or the recipe marked the hit incomplete, including a cut or
missing lead.

## Related pointers

`related` lists relation rows in which the document is the source (`dir:"out"`)
or the target (`dir:"in"`), as stored by `relate`. A pointer has `dir`, `kind`,
`key`, `number` and `in_release`: `key` is the other document's key in this
release when `in_release` is true, otherwise its stored key when known. An
`in_release` pointer is read by its `key`. Pointer order is total: kind in the
order `repeals`, `replaces`, `suspends`, `amends_supplements`, `consolidates`,
`implements`, `guided_by`, `legal_basis`, `cites`, `references`, `unknown`; then
`in` before `out`; then the other document's issue date, newest first, unknown
dates last; then number; then the relation's unique
`(source_key, artifact_sha256, ordinal)`. Search returns the first pointers up
to the cap in budgets; `related_counts` maps `<dir>:<kind>` to the full count,
and `read` reaches every counted relation. Pointers carry no evidence text and
state no inverse legal effect. A pointer whose endpoint is not in the release is
never replaced by a number lookup.

## Read

Input: `country`, `release`, `items`. Handles, numbers and anchors must be valid
UTF-8 without NUL and within the lengths in budgets; otherwise the item fails
with `invalid_item` (or `invalid_next` inside a token) before any database
access. An item is one of:

- `{handle}`: a document handle returns the document entry and its outline; a
  provision handle returns that provision's text.
- `{number}`: an exact document number that resolves to one document, read like
  its handle; otherwise `ambiguous_number` or `not_found`, with no guesses.
- `{handle, from, to}`: the provisions of one document between the anchors
  `from` and `to`, both inclusive, in preorder.
- `{handle, relations:true}`: the relation list of that document or provision.
- `{next}`: a continuation token copied from an earlier result.

Units of text never overlap. A provision handle returns the provision with its
descendants by [chunk rendering](chunk.md#tree-rendering), preceded by its
ancestors' lead fragments. A range returns each provision in it once, with its
own text only, in preorder, and the first unit is preceded by the leads of
ancestors outside the range. An outline lists `phan`, `chuong`, `muc` and `dieu`
nodes in document order, or the top-level provisions when the document has no
`dieu`, each with `label`, `heading`, `anchor` and text size in code points.
Relation lists follow pointer order and add `raw_label` and evidence.

An item's output is a fixed sequence of units: the leads, then each provision,
outline entry or relation, in the order above. Each unit kind has one split
field: `text` for leads and provisions, `heading` for outline entries,
`evidence` for relations. Any unit whose serialized size exceeds the part size
in budgets is split into parts by cutting only that field; every other field is
copied to each part. If a part with an empty split field would already exceed
the part size, the item ends with `unit_too_large` and no `next`. If the
document entry and the first unit or part of its item would together exceed the
whole call cap, the item also ends with `unit_too_large`. One rule cuts the
split field: cut at the last whitespace at or before the part size; when the
remaining span has none, at the last code point at or before the part size that
does not separate a combining mark from its base, or at the part size when no
such code point exists, so every part is nonempty and the split field's parts
concatenate to the original field. Parts carry `part` and `parts`. Boundaries
depend only on the release and the item, never on the call.

The call cap in budgets counts the serialized size of returned units across
items, including a document entry on the first page of its item, and the part
size is below it. Items are processed in order and each returns whole units or
parts. When an item has more to return, its result carries `next`, an opaque
token that fixes the release, the item, its mode and the next unit or part. The
units and parts that follow a `next` are the same whatever else that call reads;
how many of them fit on one page depends on the cap left. When the next unit
does not fit in what remains of the cap, the item returns `not_read:"call_cap"`
and its own `next`. The first item always returns at least one unit or part, so
every call makes progress; the last page of an item returns no `next`.

`complete` belongs to an item's chain of pages. The page without `next` reports
`complete:true` when the chain, read in order at one release, delivered the
whole scope with its leads; every earlier page reports `complete:false`. Text
from a provision, a range or a part may be cited once its chain was read to that
last page, so range units share the leads at the start of their chain. Per-item
failures (`not_found`, `ambiguous_number`, `release_expired`, `invalid_range`,
`invalid_next`, `invalid_item` for an item that is not exactly one of the forms
above, `unit_too_large`) do not fail the call. Every result carries `release`,
and every item result that resolved a document carries its `doc` key.

## Guide

`guide` states the flow: `search`; scan `documents` and `hits`; read the chosen
handles in one `read` call, choosing a `path` anchor, read as `<doc>#<anchor>`,
to read a whole article or chapter; check `status`, `warnings` and `in` pointers
of kinds `repeals`, `replaces`, `suspends` and `amends_supplements` before
relying on a document; read `in:implements` documents for detailed rules; follow
`next` tokens to the last page before citing read text. It says that a snippet
is not the provision, that `[…]` marks omitted text, that pointer direction is
as recorded by the source, that a pointer outside the release must not be
replaced by a number lookup, and that only text returned complete may be cited.

## Evaluation

[ADR 0010](../adr/0010-retrieval-evaluation-for-agents.md) and
[evaluation](evaluation.md) product scoring, gates and promotion run on full
responses at limits 8 and 20 exactly as specified there; this note changes none
of them. Compact responses are scored separately with the same matcher on what
they return: own identities match, while child credit (no fragments) and
relation-backed credit (no edge evidence) earn nothing. That compact score is a
diagnostic, reported at limits 8 and 20. Each trial also reports serialized
response size in code points (p50, p95) for compact at limit 20 and full at
limit 8. Compact at limit 20 must not exceed full at limit 8 at p95 on the old
product cohort; otherwise the snippet cap or the compact default changes before
the server ships.

After the server exists, an agent-in-the-loop check runs on the development
cohort with only the MCP tools: one Claude and one OpenAI client model, model
versions recorded, the same system prompt, each case twice per mode per model.
Per model it records expected provisions cited under the product matcher, tool
calls, input tokens summed over every model request in the task as the provider
reports them, and tool-result size separately. Compact stays the default only
if, for each model, mean cited expected provisions do not fall and mean summed
input tokens are lower than full. No question text is stored outside the
evaluation files.
