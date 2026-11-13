# Sibling clause merge

The sibling clause merge of retrieval recipe 3, adopted on 2026-11-13 after ADR
0010 item 4 passed; the numbers are in ADR 0010 History.
[Retrieval recipe](retrieval-recipe.md) owns grouping, capping and hit text;
this note owns only the merge of sibling clause groups.

## Why

Clause and point structure in parsed text (extract `textparse/7`) turns an
article of a transcription document into clause groups, as source trees already
do. Two clauses of one article then take two of the 20 slots. On run
vn/20261113_1 this pushed an expected citation of development case dev-018 out
of the first 20 (vn:vbpl:139874 Article 1 returned as clauses 14 and 15 at ranks
19 and 20), and ADR 0010 item 4 failed. A per-article cap (research override
`--article-cap`) is a trade, not a fix: cap 1 gained 3 development citations
within 20 and lost 1 old product citation, because the expected clause
(infosys-level3-09-2020: 09/2020/TT-NHNN Article 5 clause 4) ranked behind a
sibling clause and was demoted.

## Rule

The merge runs inside the shaping window of the
[retrieval recipe](retrieval-recipe.md): the first `hydrate multiplier × limit`
fused chunks, separately for the primary and the non-current partition, in both
`Rank` (calibration) and `Shape`. Within that window, clause groups (groups
whose identity is a `khoan`, including those that absorbed their points) that
share their nearest `dieu` ancestor merge into one article group when there are
two or more of them. If any group of that article in the window is the
article-owned group itself, nothing of that article merges: a second article hit
beside it would duplicate or split the article's evidence. Membership is frozen
before the document cap; the document cap, the non-current tail and any backfill
use the merged groups as they are and never add members later.

- Position and score: the best member's position and the maximum member fused
  score. Best cosine: the maximum over every chunk of every member, as for any
  group. The abstention statistic is the best cosine of the first group of the
  resulting primary ranking before the document cap, merged or not, exactly as
  without the rule. A research trial evaluates with the build's frozen
  threshold; a build that adopts the rule calibrates with it, and calibration
  provenance records the recipe.
- Identity and citation: the article. `BestMember` is the winning constituent's
  own `BestMember`, a point when a point supplied the best chunk;
  `BestTextStart` is recomputed for it in the final merged text, and the
  existing rule applies when the best member's body is not in it.
- Hit text: the article's substantive lead spans as for a clause hit, then each
  member clause's representation in source order. Each member has a fragment,
  `Complete:true` only when its full text is included, so the hit earns credit
  for complete member clauses only, never for other clauses of the article
  ([product matching](evaluation.md)). Above the evidence cap: leads first, then
  members in rank order; a cut member is `Complete:false`.
- `Hit.Complete` is true only when the merged text covers every clause and the
  substantive text of the article; otherwise it is false and `NextPart:1` points
  to a read of the article, so a partial article is never presented as whole.
- The document cap and the non-current tail count a merged group once. Relations
  and warnings attach as for the article group, so a member keeps the warnings
  whose target is the member or its ancestor.

## Measurement

The rule was measured with the research override `corpus eval --merge-siblings`
against release vn/20261112_2 on both cohorts, with the compact size gate. It
passed ADR 0010 item 4 and became recipe 3. The release recipe now decides: the
override is rejected with a message naming the release recipe, while
`--article-cap` stays a research tool on both recipes.
