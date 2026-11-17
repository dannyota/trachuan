# ADR 0004: Subject taxonomy seeded from Bộ pháp điển

Status: accepted 2026-11-08.

## Context

Agents need to narrow a search by subject before retrieving. VBPL classifies
only at document level. Bộ pháp điển (45 chủ đề, 271 đề mục, with
phần/chương/mục inside each đề mục) places every in-force article in a tree, but
its public portal now requires authentication; a 2023 official offline package
exists with 86,388 placed articles linked to VBPL ItemIDs.

## Decision

Levels 1 to 4 of the subject tree come from the 2023 codification package,
keeping names and codes. Custom nodes are declared in a YAML file for subjects
the codification lacks. Articles matching the package receive seed labels; an
article of a consolidated document whose corresponding source article is seeded
inherits those labels; the rest are classified by an LLM in batches (document,
then chapter with all its articles), each article choosing only within the
level-2 families its document request returned; manual overrides win. The
subject filter never hides results: search runs over the whole corpus and
reports the count outside the filter.

## Consequences

Official structure with little hand work; a ground-truth set to measure the
classifier. The package is from 2023, so post-2023 law depends on the LLM layer.
Pháp điển data is never served to users.

## Alternatives rejected

A hand-written tree. Pure embedding zero-shot classification. A fine-tuned
classifier. Hard-cut subject filtering.

## History

- 2026-11-08: accepted in the brainstorming conversation.
- 2026-11-08: after design review findings S-08 to S-11, fixed the precedence
  (manual over seed over llm), moved classification after embedding, and defined
  the import mapping and code rules in design/taxonomy.md.
- 2026-11-09: join spike ([research note](../research/phapdien-join-2026-11.md))
  found seeds for 25.3% of primary corpus articles (24.7% through the VBPL
  ItemID alone) and 91.6% of chủ đề 22 package articles; added Tiểu mục
  flattening, STT collision and empty đề mục rules to the import mapping.
- 2026-11-15: added consolidation inheritance (method `inherited`, below seeds,
  above `llm`) with its own evaluation rows after the classification design
  review; see design/taxonomy.md.
- 2026-11-16: replaced embedding-selected candidate nodes with the subtrees of
  the document request's level-2 families. On the 300 masked seeded articles of
  `vn/20261115_2`, whole-tree cosine top 10 reached candidate recall 0.05, 0.24
  and 0.37 at levels 2, 3 and deepest; the best variant (5 families of 20 nodes
  with ancestor closure) 0.82, 0.53 and 0.47; lexical and fused selection less.
  None came near the 0.95 floor, which now applies to family recall.
- 2026-11-16: corrected the seeded slice's precision after the Haiku pilot on
  the 300 masked articles failed every floor (all-pairs precision 0.569, 0.353
  and 0.260; recall 0.850, 0.653 and 0.560 at levels 2, 3 and deepest). Seeds
  place each article once while the design lets an article take several nodes,
  so all-pairs precision counted correct additional nodes as errors. The slice
  now gates recall and top-1 precision at the same floors; precision of
  additional nodes is gated on the model-adjudicated multi-family set (0.80).
  This is a correction, not a relaxation: recall alone already fails the pilot.
  The document menu now shows `<level-1> > <level-2>` names and omits level-2
  nodes with no provisions and no children (54 of 282), which removes all four
  duplicated level-2 names, including the empty 34.10 that took 34.11's
  documents.
- 2026-11-17: reduced the LLM layer's scope to level 2. On the 300 masked seeded
  articles (prompt v2), deepest-level recall was 0.583 for Haiku, 0.633 for
  Sonnet and 0.650 for a two-step request that chose within one family's
  subtree, all below 0.70; it reached 0.704 only when the family was already
  right. This is a scope change, not a floor change: the floors are unchanged,
  level 3 joins only when it passes them on every applicable slice, and deeper
  nodes come only from seeds and inheritance. The two-step request is retired:
  its family step lost families (recall 0.923 against the document request's
  0.970) and chose several for 241 of 300 articles.
