# Heading-only trees

Proposed. Some VBPL source trees give articles only their heading line while the
body sits in the bundle's HTML transcription. This note decides, per document,
when `extract` reads such a document from its transcription instead of its tree,
and how a document that keeps a partial tree is marked.
[Text extraction](text-extraction.md#text-source-per-document) owns text source
selection; [clauses and points](text-structure.md) owns the structure parsed
text gains. Evidence:
[source trees with heading-only articles](../research/vn-heading-only-trees-2026-11.md).

## Measured

On release `vn/20261112_2`, 707 of 2,396 source-tree documents have at least one
heading-only article: 642 have some (3,909 of their 11,344 articles), 65 have
all. The 65 still read from the tree because today's whole-tree test also
inspects structure nodes, whose titles sit in their text. Under the rule below,
338 of the 707 are read from their transcription (266 primary, 72 relation
context), adding about 1.66 million characters of article text, net of the
heading lines the tree already had, that search does not see today; 369 keep
their tree.

## Rule

An article is heading-only when it has no child provision and its text, after
whitespace collapse, is its label, the label and a period, the label and its
heading joined by a space, or the label, a period, a space and its heading; text
that is the label alone (with or without a period) is not heading-only when the
heading is nonempty. This is the test `headingOnly` in `extract` applies to a
whole tree today, applied to one article.

A source-tree document with at least one heading-only article is read from its
transcription when all of these hold:

1. The transcription is the bundle's `<id>_content.html`, selected and limited
   as text extraction already selects it, and the parser accepts its main scope.
2. The accepted scopes' article anchors equal the tree's article anchors, in
   order. A tree anchor with a `~k` suffix never matches.
3. Every tree article that is not heading-only agrees with the transcription's
   article of the same anchor: the tree article's full text (its own text and
   its descendants, joined as chunk renders a subtree) occurs, after whitespace
   collapse, exactly as a contiguous part of the transcription's article text.
   The transcription may add text; it may not change any. Any other difference,
   a changed number or a dropped word included, keeps the tree.
4. Every clause and point anchor of the tree occurs among the transcription's
   clause and point anchors (with [clauses and points](text-structure.md)).
5. The text of every other tree provision (parts, chapters, sections, `other`)
   occurs, after whitespace collapse, in the text of the provisions the parser
   emits from the transcription, not merely in its input paragraphs: text the
   parser discards (a signature block, a preamble table) does not count.

Then the document's text source is `html_transcription`, and `text.json` records
that the tree was consulted and set aside, with the counts behind each rule.

Otherwise the tree stays. Then:

- When the tree passes today's whole-tree test (every node, structure nodes
  included, holds only its label and heading), the existing order of later
  sources applies unchanged: the transcription when it is usable, then
  `pdf_text` and checked OCR as text extraction orders them, and only when none
  is usable the tree with the gap `tree_headings_only`. Frozen selection replays
  the same choice. On the measured release no document is in this case: such
  trees were already read from a later source.
- When every article is heading-only but the tree fails that test (a section
  keeps its title in its text, 65 documents measured), later sources are not
  used: the tree stays with `tree_headings_only`.
- When some articles hold text, later sources are not used either: they cannot
  be checked against the tree's text. The tree stays with the new gap
  `tree_headings_partial` (detail: the count of heading-only articles and the
  first rule that failed).

`tree_headings_partial` joins every list that `tree_headings_only` is on: the
gaps that set `text_complete` false
([text extraction](text-extraction.md#what-is-recorded)), and the gaps for which
relations from text are not read ([relations text](relations-text.md)). Like
`tree_headings_only` it is a coverage gap that release acceptance reports and
that the owner may accept.

Manual corrections are not an extract input: they stay pinned at build planning
([validity corrections](validity-status.md)). A switch can still invalidate the
evidence of any correction whose evidence document (its `evidence.key`, else its
own key) switches, and build fails closed on a quote it cannot find verbatim at
the evidence anchor. So the implementation checks every such correction against
the transcription before the run, and a correction that would fail is updated
first. On the measured release one correction qualifies (`vn:vbpl:2145`,
evidence in `vn:vbpl:10761` `dieu-20`), and its quote survives verbatim.

## Effects

- Extract: the 338 switched documents change source; the 369 kept documents gain
  a gap, which changes their extract manifest too. So extract, relate, chunk and
  embed repeat for all 707 documents: chunk binds the extract manifest hash, and
  embed binds the chunk manifest hash, even where the embedded text of a kept
  document is unchanged. The extract recipe changes; this joins the run that
  carries `textparse/7`. Source-tree documents without a heading-only article
  keep byte-identical artifacts.
- Citations: article anchors are unchanged by rule 2 and clause anchors by
  rule 4. Structure anchors may change (a source `chuong-i/muc-1` can become a
  parsed `muc-1`); node ids change from source ids to `tx:` ids. The run lists
  both, and the identity map's node check re-adjudicates entries on these
  documents.
- Validity: effect articles of a switched document become review evidence, not
  rule input ([text extraction](text-extraction.md#validity-rules)). That
  withdraws its automatic effects on other documents as well. The run therefore
  diffs every validity value and decision across the whole corpus against the
  previous release, not only on the switched documents, and each change is read
  before the release is accepted.
- Relations from text: switched documents' articles that were heading-only now
  hold text, so more relations can be read; kept documents with
  `tree_headings_partial` stop yielding text relations.

## Checks

The primary runs these; they are requirements, not results.

- Fixtures from recorded bundles, one per rule: a matching partial tree (read
  from the transcription); anchors that differ; a `~k` anchor; a rejected
  transcription; a changed number, a dropped word and a reordered clause in an
  article that holds text; a tree clause anchor the transcription lacks; a
  section title missing from the emitted provisions; each kept with
  `tree_headings_partial`.
- A correction on another document whose evidence quote sits in a switching
  document: the pre-run check finds it, and build fails closed if it is missed.
- A tree whose every article is heading-only and whose sections hold titles:
  read from a transcription that passes rules 1 to 5; with none, it keeps the
  tree with `tree_headings_only` and uses no later source. A tree that passes
  today's whole-tree test still falls to `pdf_text` or checked OCR as today. A
  frozen selection replays the same choice.
- `tree_headings_partial` sets `text_complete` false in build, suppresses text
  relations in relate, appears in coverage and status, and blocks release until
  accepted.
- The live run reports switched and kept documents by rule, characters gained,
  changed structure anchors and node ids, identity-map entries re-adjudicated,
  and the corpus-wide validity diff, and is gated by
  [ADR 0010](../adr/0010-retrieval-evaluation-for-agents.md) item 4.
