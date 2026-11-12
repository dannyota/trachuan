# Clause and point parsing of article text

Observed 2026-11-12 on this MacBook against the local release
`corpus_vn_20261111_4` (3,841 documents, 298,500 provisions, 57,176 articles).
Evidence for [clauses and points in parsed text](../design/text-structure.md).

## Method

All provisions were exported as JSON lines (id, document, parent, kind, anchor,
ordinal, text, document text source). A Python prototype of the design's grammar
and checks ran on each article's stored `text`, split at newlines; in parsed
text a newline separates the parser's paragraphs, so the lines are the
paragraphs. Differences from the real parser: the quoted-text state starts at
each article instead of running through the document, and only the stored
article text is read, not the source bundle. The prototype was not committed;
the design's acceptance list adds a comparison test over recorded bundles that
replaces it.

The validation set is every source-tree article whose children are all `khoan`
and whose child texts each occur in the article text, in order (21,223 articles;
6,028 more fail that containment and were skipped). For those, the prototype
read the article text alone and its result was compared with the tree: clause
start offsets, then clause spans against the tree clause text with its
descendants, then point counts, then point spans, all after whitespace collapse.

## Leaf articles

| Text source and result                              | Articles |
| --------------------------------------------------- | -------- |
| Parsed text, split                                  | 10,676   |
| Parsed text, no clause or point paragraph           | 3,778    |
| Parsed text, rejected `point_sequence`              | 101      |
| Parsed text, rejected `clause_sequence`             | 75       |
| Parsed text, rejected `clause_candidate`            | 44       |
| Parsed text, rejected `point_candidate`             | 18       |
| Parsed text, rejected `single_clause`               | 15       |
| Parsed text, rejected `clause_style`, `point_style` | 4        |
| Source tree, heading line only (686 documents)      | 7,324    |
| Source tree, no clause or point paragraph           | 7,648    |
| Source tree, split                                  | 46       |
| Source tree, rejected                               | 94       |

The 10,676 split parsed articles hold 40,651 clauses and 26,608 points. Six of
them fail unique containment (a child text occurs twice in its parent); in 308
of the 7,112 parsed parents with points, the last point is followed by a
paragraph that is not a sub-item.

## Validation against source trees

| Result                                                                                                                                                                | Articles |
| --------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------- |
| Accepted, clause starts equal                                                                                                                                         | 20,436   |
| of those, point counts equal                                                                                                                                          | 20,333   |
| Accepted, clause starts differ                                                                                                                                        | 117      |
| Rejected (`point_candidate` 132, `clause_sequence` 170, `point_sequence` 122, `quote_unbalanced` 112, `clause_candidate` 78, `none` 29, `single_clause` 21, styles 6) | 670      |

Spans where clause starts are equal: 78,505 of 78,556 clause spans equal the
tree's clause text with its descendants. The 51 others were sampled (7 read): in
6 the tree article's own text stops early, so the article text holds less than
its children; one is quoted amendment text that the grammar does not read as
structure. Point spans where counts are equal: 54,498 of 54,500 equal; of the
last points, 974 are followed by non-sub-item paragraphs and the tree puts those
paragraphs in the last point in all 974.

## Differences read by hand

Clause starts differ (19 read): 148/2025/QH15 Điều 8, 23, 24, 29, 33 and 35,
59/2015/TT-BCT Điều 17, 39/2016/TT-NHNN Điều 14, 803/2007/QĐ-BKH Điều 7,
09/2018/NĐ-CP Điều 38 and 43, 15/2008/QH12 Điều 19, 36/2016/TT-NHNN Điều 17 and
101/2017/TT-BTC Điều 11: the tree lacks a clause the text has. 184/2015/TT-BTC
Điều 18 and 03/2008/QĐ-NHNN Điều 3: the tree records a continuation paragraph as
a clause. 41/2016/TT-NHNN Điều 13 and 05/2017/QĐ-TTg Điều 17: the tree records
points as clauses. 05/2007/TTLT-NHNN-BKH&ĐT-BTC-BTM Điều 2 uses sub-numbering
`2.1.` with points under each; an earlier prototype read the points as the
article's own and ran the last one to the article end, so sub-numbering became a
failed candidate. An old `1-` style article was missed by the first prototype
and is accepted now.

Point counts differ (8 read): 103/2026/NĐ-CP Điều 12 khoản 1, 148/2025/QH15 Điều
32 khoản 3, 06/2008/QĐ-NHNN Điều 5 khoản 1 and 62/2016/TT-BTC Điều 11 khoản 8:
the tree lacks points. 104/2016/QH13 Điều 8 khoản 2: the tree clause text stops
early. 80/2016/NĐ-CP Điều 1 khoản 8: the points are quoted amendment text; the
grammar is right not to read them. 184/2015/TT-BTC Điều 7 khoản 1: `c1)` to
`c3)` nested under `c)` in the tree, siblings in the grammar. 38-HĐBT Điều 10
khoản 2: an uppercase `C)` the first prototype read as running text; it is a
failed candidate now.
