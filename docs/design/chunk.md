# Chunk recipe 1

Proposal for implementation and independent review. This is the baseline from
the [study](../research/retrieval-hybrid-2026-11.md), adapted to the extracted
tree. It preserves source identities; it does not import banhmi code. Numeric
limits live in [budgets](budgets.md#indexing).

## Stage and artifact

`apps/corpus/internal/stage/chunk/` owns planning, splitting, prefix rendering,
artifact decoding and workshop rows in separate files. Register after `relate`.
Plan one item per done `extract` item with a done `fetch` item, keyed by the
document key. Empty extracted trees still get an item and an empty artifact.

Use
`stage.RecipeHash("chunk-input", "1", document_key, fetch_sha256, extract_sha256)`
for the input hash. Both hashes are artifact manifest hashes, verified before
reading. This prevents a changed title, number or date from reusing stale
prefixes when the tree is unchanged. Require both artifacts to name the same
document and the extract input to reference that fetch hash; mismatched lineage
is an infrastructure error.

Recipe hash components, in order: `chunk`, `vn-article/1`, the canonical recipe
JSON. JSON fields are `version`, `rough_count`, `body_limit`, `minimum_runes`,
`traversal`, `parent_lead`, `sharding`, `dedup`, `outside_articles`,
`prefix_fields`, `prefix_field_limit`, `prefix_number_limit`, `date_normalizer`.
Values are the algorithms below and the limits in budgets. Serialize a
fixed-field Go struct without whitespace; never hash map iteration.
`date_normalizer` records the shared build normalizer's `normalizer_version`
from [normalization](normalization.md). Hash that version in this recipe; a
normalizer change requires re-chunking and re-embedding affected inputs.

Artifact files: `chunks.json`, `coverage.json`, `MANIFEST.sha256`. JSON uses
UTF-8, snake_case, no timestamps. `chunks.json` is:

```json
{
  "version": 1,
  "document_key": "vn:vbpl:123",
  "fetch_sha256": "<sha256>",
  "extract_sha256": "<sha256>",
  "chunks": [
    {
      "ordinal": 0,
      "node_id": "source-node-id",
      "anchor": "dieu-1/khoan-1",
      "context_prefix": "<retrieval hint>",
      "content": "<rendered source spans>",
      "rough_tokens": 100,
      "spans": [
        {
          "node_id": "source-node-id",
          "start": 0,
          "end": 200,
          "content_start": 0,
          "content_end": 200,
          "role": "body"
        }
      ]
    }
  ]
}
```

Source offsets count Unicode code points in the extracted node's `text`;
`content_start/end` address the rendered chunk. Intervals are half-open and
their substrings must be identical. Split spans around rewritten whitespace;
only inserted whitespace may lack a source span. Containment aliases may map
several source nodes to the same content interval without duplicating text.
Roles are `body` or `lead`. Every chunk has exactly one owning `node_id` and its
unchanged extraction `path` as `anchor`. Spans can identify ancestor lead-ins
and descendants without creating multiple owners. Ordinals are zero-based,
contiguous after omission and deduplication. Build identity is
`(document_key, ordinal)` within the release, never an inferred citation.

`coverage.json` contains `version`, `document_key`, `gaps[]` of `{kind,detail}`,
and `omissions[]` of `{node_id,start,end,reason,kept_ordinal}`; the last field
is null except for duplicates. It also records `covered_ranges[]` and
`whitespace_ranges[]`, each with `{node_id,start,end}` in source coordinates.
Reasons: `label_only`, `short`, `duplicate`, `structural_heading`. Every
non-whitespace source interval must be covered or explicitly omitted. Coverage
is the union of body and lead source intervals; repeating a lead never increases
coverage. Record containment aliases too.

## Tree rendering

Follow `parent_node_id`, not slash parsing: extraction paths are public anchors
and do not encode every structural ancestor. Order siblings by `ordinal`, then
UTF-8 `node_id`. Invalid parents, cycles or repeated node IDs are source gaps;
do not repair them by guessing. Traverse roots depth first.

For each node form a source representation from its trimmed `text`. If a
parent's text contains each nonempty child representation exactly once, in child
order, use the parent text once and map those child intervals into it.
Otherwise, if none occurs, append each child representation with a newline.
Partial, repeated or out-of-order containment yields `ambiguous_tree_text`: emit
the parent text as shards owned by the parent, then process children separately.
Record the overlap gap; never delete unmatched parent text. Comparisons here are
byte-exact after edge trimming, not accent folding.

A heading line is label-only when, after NFC, lowercase, trimming and removal of
trailing `.`, `:` and `)`, it equals the node's similarly normalized label. It
is not substantive. Heading text beyond a label remains substantive. This rule
does not classify arbitrary digits or short legal duties as labels.

## Splitting

1. Start at each `dieu`, skipping article descendants until that article is
   processed. Render the article subtree. If its rough size fits, emit it as one
   article-owned candidate; do not emit its children again.
2. Rough size is zero for empty text, otherwise `max(1, rune_count / 2)` with
   integer division. Prefixes are outside this rough limit. This reproduces the
   historical approximation, not a tokenizer guarantee.
3. Above the limit, recurse into immediate legal children (`khoan`, `diem`, or
   nested `dieu`) when their spans are unambiguous. Emit every substantive
   parent-only interval separately, except the lead before the first child:
   prepend that lead to each child's representation. Do not carry a label-only
   lead. Include inherited leads in size calculation. A carried lead is never
   removed from provenance. If it alone exceeds the limit, shard it once under
   the parent and do not repeat it on children. Nonlegal child subtrees are
   rendered as parent-owned intervals, not skipped. If all child candidates are
   omitted, emit the unused lead once under its parent using the same minimum
   rule; record any resulting omission.
4. When no legal split is available, shard the rendered text by lines. Trim line
   edges, skip blank lines, greedily pack lines separated by `\n` while the next
   addition fits. Flush before an oversized line. Split that line on Unicode
   whitespace and greedily pack whole words separated by one space. An oversized
   single word stays intact and can fail the later tokenizer cap. Shards retain
   the original owning provision; never invent a provision or anchor for a
   shard. No sliding overlap is added. Repeat carried leads on each resulting
   shard and count them in its size. If the lead leaves no room for one body
   word, emit it once under its parent and shard the body without it. Preserve
   source order within each shard.
5. Minimum-content and dedup comparisons use NFC plus collapsed Unicode
   whitespace, with case and accents retained. Drop label-only candidates and
   candidates below the minimum rune count after removing carried leads. Record
   the exact omitted spans. Do not pad short children with parent text merely to
   pass the minimum. Exact normalized content duplicates within one document
   keep the first candidate in traversal/shard order, even across provisions;
   record the lost citation and retained ordinal. No cross-document
   deduplication. This deliberate baseline loss is measured by evaluation.
6. For nodes outside any article, process substantive own text of `phan`,
   `chuong`, `muc`, `khoan`, `diem` and `other`, using the same shard/minimum
   rules, but without absorbing child articles. Label-only structural nodes are
   omitted. This keeps preambles and unclassified appendix text searchable
   without treating the whole document as an article.

Own text excludes unambiguously contained child representations. Apply that rule
outside articles too, so a chapter containing an article is not emitted again as
a chapter chunk. In ambiguous containment, shard only the node's own stored text
before visiting children; do not append them a second time. Every subtree is
visited once. Invalid ancestry ends the item as `invalid_source_data` without
chunks; build preserves the gap rather than accepting a repaired tree.

If the tree is empty, preserve `no_tree`. If a nonempty tree produces no chunk,
emit `no_chunks`. Every short or duplicate omission appears in the coverage
report and a summarized `chunk_omission` gap. A done artifact may have gaps;
`Resolve` recreates them for each run. A failed invariant is not a valid empty
artifact.

## Context prefix

Join nonempty fields with `\n` in this order: trimmed document number; title;
nearest `chuong` ancestor's label plus heading; nearest `muc` ancestor's label
plus heading; `Hiệu lực: YYYY-MM-DD`, else `Ban hành: YYYY-MM-DD` using the
[date normalizer](normalization.md), the same implementation build uses, not a
separate parser or layout list. Collapse whitespace inside fields. Cap title and
each heading separately by Unicode code points; cap the number separately. Cut
at that boundary with no ellipsis. No issuer, taxonomy, article text, query
instruction or inferred dates enter the prefix.

Only retrieval input uses the prefix. Evidence is read from provision text and
source spans, never by stripping a guessed prefix from chunk text. The
[embedder](embedder.md) counts actual tokens on prefix plus content and rejects
oversize input instead of silently changing this recipe.

## Workshop projection and checks

Add workshop `chunk`: `artifact_id bigint` FK artifact, `ordinal integer >= 0`,
`extract_artifact_id bigint`, `node_id text`, `anchor text`,
`context_prefix text`, `content text`, `rough_tokens integer > 0`, `spans jsonb`
(array). All columns are non-null. PK `(artifact_id,ordinal)`; composite FK
`(extract_artifact_id,node_id)` references provision `(artifact_id,node_id)`.
Index that FK. `Rows` loads only from artifact files, resolving the recorded
extract manifest hash to an artifact row; `Resolve` recreates coverage gaps.
There is no workshop vector table.

Required cases for the primary's implementation checks: metadata-only prefix
change, small article, large clause/point tree, substantive lead, parent text
containing children, ambiguous containment, blank/label/short leaves, duplicate
citations, long line, oversized word, non-article text, empty tree, exact span
coverage and repeatable ordinals. Lost short duties and duplicate citations are
visible baseline limitations, not evidence that extraction succeeded fully.
