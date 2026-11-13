# Heading-only trees

Proposed. Some VBPL source trees give articles only their heading line while the
body sits in the bundle's HTML transcription. This note decides, per document,
when `extract` reads such a document from its transcription instead of its tree,
and how a document that keeps a partial tree is marked.
[Text extraction](text-extraction.md#text-source-per-document) owns text source
selection and the existing whole-tree rule;
[clauses and points](text-structure.md) owns the structure parsed text gains.
Evidence:
[source trees with heading-only articles](../research/vn-heading-only-trees-2026-11.md).

## Measured

On release `vn/20261112_2`, 707 of 2,396 source-tree documents have at least one
heading-only article: 642 have some (3,909 of their 11,344 articles), 65 have
all. The 65 still read from the tree because today's whole-tree test also
inspects structure nodes, whose titles sit in their text (59 documents). In 413
of the 707 the parser accepts the transcription and its articles have exactly
the tree's anchors in order; in 405 of those, every tree article that holds text
agrees with the transcription. Reading those 405 from the transcription adds
about 2.0 million characters of article text that search does not see today.

## Rule

An article is heading-only when it has no child provision and its text, after
whitespace collapse, is its label, the label and a period, or the label and its
heading joined by a space or by a period and a space. Structure provisions
(`phan`, `chuong`, `muc`) are not tested: their title is not body text.

A source-tree document with at least one heading-only article is read from its
transcription when all of these hold:

1. The transcription is the bundle's `<id>_content.html`, selected and limited
   as text extraction already selects it, and the parser accepts its main scope.
2. The accepted scopes' article anchors equal the tree's article anchors, in
   order. A tree anchor with a `~k` suffix never matches: duplicate articles are
   not resolved here.
3. Every tree article that holds text and has no child provision agrees with the
   transcription's article of the same anchor: equal or contained after
   whitespace collapse, or a sequence ratio of at least 0.9. Articles with child
   provisions are not compared (their tree text is the lead only).

Then the document's text source is `html_transcription`; the parsed text gains
clauses and points as [clauses and points](text-structure.md) says, and
`text.json` records that the tree was consulted and why it was set aside (the
counts of heading-only and agreeing articles).

Otherwise the tree stays. A document whose every article is heading-only keeps
the gap `tree_headings_only`; one with some gets the new gap
`tree_headings_partial`, with the count of heading-only articles as detail, and
`text_complete` false. Both are reasons a search hit may lack text.

Reason: the anchor check keeps citations stable (an article cited as `dieu-5`
before is `dieu-5` after), and the text check refuses a transcription that
disagrees with what the tree does hold. On the measured release the rule reads
405 documents from their transcription; the other 302 keep their tree with one
of the two gaps.

## Effects

- Extract repeats for the documents whose selection changes; its recipe changes
  with the selection rule. It joins the run that carries `textparse/7`.
- Validity: a document read from its transcription has its effect articles as
  review evidence, not rule input
  ([text extraction](text-extraction.md#validity-rules)). The run reports, for
  the 405 documents, every validity decision that changes against the previous
  release, and each change is read before the release is accepted.
- Identity map and manual corrections: anchors are unchanged by rule 2, node ids
  change (`tx:` ids replace source ids), so the identity map's node check
  re-adjudicates entries on these documents; the run lists them.
- Relations from text: articles that were heading-only now hold text, so more
  relations can be read; relate repeats for these documents.

## Checks

The primary runs these; they are requirements, not results.

- Fixtures from recorded bundles: a partial tree whose transcription matches
  (read from the transcription, anchors equal, clauses present); anchors that
  differ; a tree anchor with `~k`; a transcription the parser rejects; a text
  disagreement on one article; each keeps the tree with `tree_headings_partial`.
- A tree whose every article is heading-only but whose sections hold titles in
  their text is now recognized and read from a matching transcription.
- Source-tree documents without heading-only articles keep byte-identical
  extract artifacts.
- The live run reports documents switched, kept partial and kept whole, the
  characters gained, validity decisions changed and identity-map entries
  re-adjudicated, and is gated by
  [ADR 0010](../adr/0010-retrieval-evaluation-for-agents.md) item 4 like any
  change to chunks.
