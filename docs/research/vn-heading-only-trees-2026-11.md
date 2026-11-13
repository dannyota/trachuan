# Source trees with heading-only articles

Observed 2026-11-13 on this MacBook against release `corpus_vn_20261112_2`
(3,842 documents; 2,396 read from a source tree, 415 of them without any
article). Evidence for [heading-only trees](../design/heading-only-trees.md).

## Method

An article is heading-only when it has no child provision and its text, after
whitespace collapse, is its label, the label and a period, or the label followed
(after a space, or after a period and a space) by its heading, and text that is
the label alone is not heading-only when the heading is nonempty: exactly the
per-row test of `headingOnly` in
`apps/corpus/internal/stage/extract/selection.go`, which applies it to a whole
tree, here applied per article (the measurement script implements the same two
conditions). Every source-tree document with at least one such article was
compared with its stored transcription (`<id>_content.html` in the fetch bundle
of `vn/20261112_2`), parsed by the production parser
(`textparse.ParagraphsFromHTML` and `Parse` at main 3cd06ed, run from an
uncommitted scratch test). The comparison takes the accepted scopes' articles:
their anchors in order against the tree's article anchors in order, and, for
tree articles that hold text and have no children, the two texts after
whitespace collapse.

## Documents

| Heading-only articles in the tree        | Documents | Articles (heading-only of all) |
| ---------------------------------------- | --------- | ------------------------------ |
| None                                     | 1,274     | 0 of 27,255                    |
| Some (467 primary, 175 relation context) | 642       | 3,909 of 11,344                |
| All (22 primary, 43 relation context)    | 65        | 3,873 of 3,873                 |

The 65 trees whose every article is heading-only are still read from the tree.
The whole-tree test also looks at structure nodes, and in at least 59 of them a
section (`muc`, 388 nodes) or part (`phan`, 9) keeps its title in its text with
an empty heading (`Tiểu mục 1 THẨM ĐỊNH ...` under label `Mục 1`), so the tree
does not count as heading-only.

## Against the transcription

| Result for the 707 documents                  | Some | All |
| --------------------------------------------- | ---- | --- |
| Transcription accepted, article anchors equal | 381  | 32  |
| Transcription accepted, tree anchors a subset | 9    | 1   |
| Transcription accepted, anchors differ        | 81   | 8   |
| Transcription not accepted by the parser      | 171  | 24  |

In the 413 documents with equal anchors, tree articles that hold text compare as
follows: 1,046 equal; 65 similar (sequence ratio at least 0.9); 167 differ only
because the tree article has clause children and holds its lead alone; 3 where
the tree's text is contained in the transcription's article; 8 that differ for
real (6 where the transcription's article is the shorter, 2 others), one in each
of 8 documents. The heading-only articles of the 413 documents gain 1,987,597
characters of article text from the transcription.

## Reading

A per-article test with the anchor and text checks reads 405 of the 707
documents from their transcription (the 413 less the 8 with a real text
difference), about 2.0 million characters that search does not see today. The
other 302 keep their tree and need a gap that says text is missing.

## Strict rule

A second pass applied the rule
[heading-only trees](../design/heading-only-trees.md) adopts after review: the
tree article's full text with its clauses must occur unchanged inside the
transcription's article (no similarity ratio), every tree clause and point
anchor must occur among the transcription's, every structure and `other` text
must occur in the emitted provisions, and manual-correction quotes found in the
tree must survive. The final numbers come from the implementation's
recorded-release test (`TestHeadingMeasurement_RecordedRelease`, tag
`measurement`, on the build inputs of `vn/20261112_2`), which uses the
production selection, parser and chunk rendering:

| Result for the 707 documents                   | Documents |
| ---------------------------------------------- | --------- |
| Read from the transcription                    | 338       |
| Kept: transcription not accepted by the parser | 197       |
| Kept: article anchors differ                   | 97        |
| Kept: article text differs                     | 26        |
| Kept: a tree clause or point anchor missing    | 49        |

The 338 are 266 primary and 72 relation context documents and gain 1,660,734
characters net of the heading lines the tree already held. No document failed
the structure-text rule. The 1,689 source-tree documents without a heading-only
article keep byte-identical extract artifacts.

A Python prototype of the same rule gave 339 and 368. Each of the four documents
where they differ was traced: `vn:vbpl:5636` and `vn:vbpl:8667` have a main
scope that fails the article sequence, which the prototype skipped while the
rule requires it (anchors differ becomes not accepted); in `vn:vbpl:139039` a
clause's points are missing from the clause text, and chunk's rendering, unlike
the prototype's, shows the article holds more than the transcription (switch
becomes kept); in `vn:vbpl:8706` a no-break space after one point makes the
rendered clause differ (clause anchor missing becomes text differs).

One correction draws its evidence from a switching document (`vn:vbpl:2145`,
evidence in `vn:vbpl:10761` `dieu-20`), and its quote survives verbatim.
