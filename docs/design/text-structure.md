# Clauses and points in parsed text

Proposed. Parsed text asserts articles only today
([text extraction](text-extraction.md#from-paragraphs-to-provisions)), so a
citation such as `khoản 2 Điều 5` cannot resolve in a document read from its
transcription, and a large article is chunked by lines instead of by clause.
This note adds clause (`khoan`) and point (`diem`) provisions inside accepted
articles of parsed text. It also records two neighbouring defects, with their
counts, that later changes handle in this order: source trees whose articles
hold headings only, then older article heading shapes.

## Evidence

Measured on release `vn/20261111_4` (57,176 articles) with a prototype of the
grammar below, run on each article's stored text line by line:

| Articles without child provisions                   | Count  |
| --------------------------------------------------- | ------ |
| Parsed text (`html_transcription`), all             | 14,711 |
| of those, clauses or points accepted by the grammar | 10,692 |
| of those, no clause or point lines                  | 3,790  |
| of those, rejected by a check below                 | 229    |
| Source tree, all                                    | 15,112 |
| of those, the text is the heading line only         | 7,324  |
| of those, clauses or points accepted by the grammar | 49     |

The 10,692 parsed articles gain 40,710 clauses and 26,674 points. Source trees
are left alone: 49 articles do not justify changing source-tree artifacts, which
stay byte-identical across this change, so their chunk and embed work is reused.

Validation set: 21,223 source-tree articles whose `khoan` children are each
contained in the article text, in order. The grammar, run on the article text
alone, was compared with the tree:

| Result                                             | Articles |
| -------------------------------------------------- | -------- |
| Accepted, clause starts equal the tree             | 20,590   |
| of those, point counts also equal                  | 20,443   |
| Accepted, clause starts differ                     | 122      |
| Rejected by a check (the article would stay whole) | 511      |

Of 79,270 clause spans in accepted articles with equal clause starts, 79,213
equal the tree's clause text including its points, after whitespace collapse.
Sixteen differing articles were read by hand: in 12 the tree is wrong (a clause
or point node missing, points recorded as clauses, a continuation paragraph
recorded as a clause, a clause whose stored text stops early); one used the old
dash style, which the grammar below accepts; one is quoted amendment text, which
the grammar correctly does not read as structure; one is `c1)` to `c3)` that the
tree nests under `c)`; one is an uppercase point letter `C)` that the prototype
missed, now a failed candidate below.

## Where it applies

Inside every accepted article of the main scope or an instrument scope of a
document whose text source is `html_transcription` or, when it lands,
`pdf_text`. Not OCR text: there is no validation set for it, and a misread
number can move text between clauses in a way the checks below do not always
catch. OCR is revisited once the version 3 cross-check results exist: a text
whose check found no item could qualify. Not source trees, not neutral or tail
scopes, not structure provisions.

## Grammar

Lines are the article's paragraphs after its heading paragraph. A paragraph
inside quoted text (the document-wide state of text extraction) never starts a
clause or a point.

- Clause start: the number token (digits, then at most one inserted letter as in
  `2a`) followed by `.` and then whitespace, the end of the paragraph, an
  uppercase letter or a footnote marker `[<digits>]`; or followed by `-` and
  whitespace (older documents write `1-` then a space). A paragraph starting
  `<digits>.` and another digit (`1.1.`) is running text of the current clause.
- Point start: one letter of the Vietnamese order
  `a b c d đ e g h i k l m n o p q r s t u v x y`, optionally followed by one or
  two digits for an inserted point (`a1`), then `)`, or `.` or `-` followed by
  whitespace.
- Failed candidates: a paragraph that starts with digits and `)`, or with digits
  and `.` followed by anything not listed above, or with one uppercase letter
  and `)`.

## Checks

An article is split only when every check passes; otherwise it stays one
provision, as today, and the reason is counted.

- Clause numbers run 1, 2, 3, ... with an inserted `n<letter>` allowed after `n`
  or after the previous letter, at least two clauses, no gap, repeat or step
  back. A lone `1.` is not split: it is more often a list item than a clause (17
  such parsed articles in the measured release).
- Point letters inside one clause, or directly inside the article when it has no
  clauses, run from `a` in the Vietnamese order with inserted `a1`, `a2` after
  their letter; at least two points when they sit directly in the article.
- One clause style (`.` or `-`) per article and one point style per parent.
- No failed candidate in the article, and no points before the first clause of
  an article that has clauses.

Reasons counted: `clause_sequence`, `clause_style`, `clause_candidate`,
`point_sequence`, `point_style`, `points_before_clauses`, `single_clause`.

## What is recorded

- A clause holds the paragraphs from its start through the paragraph before the
  next clause start or the end of the article; a point holds its paragraphs up
  to the next point start, the next clause start or the end of its clause. The
  article's lead (paragraphs before the first clause) stays in the article only.
  Text after the last point belongs to the last point, as in source trees.
- The article's `text` is unchanged and still holds the whole article. Each
  child's `text` is its paragraphs joined exactly as the article joins them, so
  the article contains each child once and in order, which is the containment
  shape [chunk](chunk.md#tree-rendering) maps onto the parent.
- Kinds `khoan` and `diem`; anchors `<article anchor>/khoan-<token>` and
  `.../diem-<token>` (`dieu-5/khoan-2a/diem-đ`, `dieu-3/diem-a` for points
  directly in an article, `kem-theo-1/dieu-4/khoan-2` in an instrument scope);
  labels `Khoản <token>` and `Điểm <token>`; empty heading; ordinals by
  position; locators from the paragraphs.
- The anchor segment is the kind, `-` and the lowercased token. One function,
  `anchor.Segment(kind, token)` in a new package `apps/corpus/internal/anchor`,
  composes it for source trees (today inline in `titleParts`,
  `apps/corpus/internal/stage/extract/labels.go`), for the text parser (today
  inline in `textparse/parse.go`) and for relations from text (today `/khoan-`
  and `/diem-` plus the lowercased word in `textrule/locators.go`), so one label
  gives one anchor everywhere. Source-tree anchors do not change.
- Node ids follow the existing `tx:` rule (scope index, anchor and text), so a
  clause whose text changes gets a new id.
- `text.json` counts per document: articles split, clauses, points, and articles
  left whole by reason. No gap: an article left whole is still a true citation.
- The text parser version becomes `textparse/7` and the extract recipe changes
  with it.

## Effects

- Extract, chunk and embed repeat for documents read from parsed text (1,070 in
  the measured release); source-tree documents keep their artifacts.
- Chunk: an article that fits stays one article-owned chunk. A larger article
  now splits at its clauses and points instead of at lines, and those chunks
  cite the clause.
- Relations from text ([relations text](relations-text.md)): an anchor such as
  `dieu-5/khoan-2` in a parsed document now resolves where it named a clause;
  build counts the change in `relation_clause_unresolved`.
- Validity: unchanged. Effect articles of parsed documents stay review evidence
  under [text extraction](text-extraction.md#validity-rules).
- Identity map and manual corrections: existing anchors keep their meaning; new
  child ids carry no entries.

## Source trees with heading-only articles

7,324 source-tree articles in 686 documents hold their heading line only, while
other articles of the same tree hold text; the body sits in the bundle's
transcription. Text extraction already switches a document to its transcription
when every article is heading-only. For a partial tree the candidate rule is:
when the transcription's article reading is accepted and its article numbers
equal the tree's, read the whole document from the transcription; otherwise keep
the tree and mark the document incomplete. It needs its own measurement (how
many of the 686 pass the number match, and whether the texts of articles that do
hold text agree) before it becomes a rule. It comes next after this note, ahead
of the heading grammar below: these documents lose text, not only structure.

## Older article headings

Of 159 documents with `structure_uncertain` in the measured release, 74 fail on
an article heading the grammar refuses. The most common shapes, by count of
paragraphs: `Điều 9` alone (204), a separator `.-`, `-` or `. -` before the
title (62), a footnote marker after the number or the period (`Điều 24.[36]`,
13), a colon (15), a title that opens with a quote mark (42) or with
`(được bãi bỏ)` (5), and no separator before a capital (66). An approximate
prototype (no instrument or structure-anchor rules) that accepts every shape
except the last makes about 42 of the 159 pass the article sequence; about 22
more then fail the sequence, mostly an attached regulation restarting at
`Điều 1` without an instrument title. The last shape stays a failed candidate,
because `Điều 6 Luật ...` wrapped from a reference has the same form. Inserted
article letters follow the Vietnamese order (`29d`, then `29đ`).

This is a separate change to the article grammar of text extraction, measured
with the real parser on the recorded fixtures and on these 159 documents before
it is accepted.

## Acceptance

The primary runs these; they are requirements, not results.

- Fixtures with expected provision lists for: clauses with points, inserted
  clauses and points (`2a`, `a1`, `đ`), old dash style, footnote markers after
  the clause number, `1.1.` running text, points directly in an article, a
  single `1.`, quoted amended clauses, and each rejection reason.
- The containment property holds for every split article of the recorded
  fixtures: each child text occurs once, in order, inside the article text.
- A comparison test over recorded source-tree bundles that also carry a
  transcription: clause starts read from the transcription equal the tree's
  clause anchors, with each difference listed.
- Source-tree and OCR documents produce byte-identical extract artifacts before
  and after the change; the compatibility test pins them.
- For every clause and point label form in the fixtures, the anchor that
  relations from text produce equals the anchor extract produces.
- The live run is gated by the change rule of
  [ADR 0010](../adr/0010-retrieval-evaluation-for-agents.md) (item 4), because
  chunks and vectors change for the parsed documents: every gate passes on the
  old product and development cohorts, the net count of matched expected
  citations within 20 hits rises on at least one cohort, and recall@20 and MRR@8
  fall on neither; otherwise the extract change is reverted. A clause-owned hit
  satisfies an article expectation through the scorer's ancestor credit
  (`agent-eval/3`, [evaluation](evaluation.md)). The run also reports the split
  counts and reasons and the change in `relation_clause_unresolved`.
