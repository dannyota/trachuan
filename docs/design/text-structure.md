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

A prototype of the grammar and checks below ran over release `vn/20261111_4`
(57,176 articles); method, tables and the cases read by hand are in
[clause and point parsing](../research/vn-clause-parsing-2026-11.md). The
prototype reads each article's stored text and starts the quoted-text state at
the article; the parser keeps that state across the document.

- Parsed text (`html_transcription`): 14,711 articles, all without children.
  10,676 split, adding 40,651 clauses and 26,608 points before the two checks
  below that remove points or whole articles (308 parents, 6 articles); 3,778
  hold no clause or point paragraph; 257 are rejected.
- Source trees: 15,112 articles without children. 7,324 (in 686 documents) hold
  a single line under 200 characters, by a heuristic that does not compare it
  with the heading, so these are heading-only candidates; 7,648 hold unnumbered
  text, 46 would split. Source trees are left alone: 46 articles do not justify
  changing source-tree artifacts, which stay byte-identical, so their chunk and
  embed work is reused.
- Validation on 21,223 source-tree articles whose clause children each occur in
  the article text in order: 20,553 accepted, of which 20,436 have the tree's
  clause starts; 78,505 of 78,556 clause spans and 54,498 of 54,500 point spans
  equal the tree's text. Of 28 differing articles read by hand, 23 are tree
  errors (missing clauses or points, continuation paragraphs or points recorded
  as clauses, article text that stops early); the other five changed the grammar
  to what this note states (old `1-` style, uppercase `C)`, sub-numbered `2.1.`)
  or confirm it (quoted amendment points, `c1)` as siblings).

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
clause or a point and is never a failed candidate.

- Clause start: one to three digits, at most one inserted letter (`2a`), then
  `.` followed by whitespace, the end of the paragraph, an uppercase letter or a
  footnote marker `[<digits>]`; or `-` followed by whitespace (older documents).
- Point start: one letter of the Vietnamese order
  `a b c d đ e g h i k l m n o p q r s t u v x y`, at most two digits for an
  inserted point (`a1`), then `)`, or `.` or `-` followed by whitespace.
- Sub-item: a paragraph starting with `-`, `+`, `•` or `–`. It belongs to the
  clause or point before it.
- Failed candidate, outside quoted text: any other paragraph that starts with
  digits (and an optional letter) followed, after optional spaces, by `.`, `)`,
  `-` or `:`, including sub-numbering (`2.1.`) and `2a)`; any other paragraph
  that starts with a lowercase letter (and optional digits) followed by `)`,
  `.`, `-` or `:` (`c.Nội dung`); one uppercase letter (and optional digits)
  followed by `)`. A heading the grammar does not read can therefore never hide
  inside the clause or point before it.

## Checks

An article is split only when every check passes; otherwise it stays one
provision, as today, and the reason is counted.

- Clause numbers run 1, 2, 3, ... with an inserted `n<letter>` allowed after `n`
  or after the previous letter in the Vietnamese order, at least two clauses, no
  gap, repeat or step back. A lone `1.` is not split: it is more often a list
  item than a clause.
- Point letters inside one clause, or directly inside the article when it has no
  clauses, run from `a` in the Vietnamese order with inserted `a1`, `a2` after
  their letter; at least two points when they sit directly in the article.
- One clause style (`.` or `-`) per article and one point style per parent.
- No failed candidate in the article, and no points before the first clause of
  an article that has clauses.
- Unique containment: each clause text occurs exactly once in the article text
  and each point text exactly once in its parent's text. Otherwise the article
  stays whole, because chunk would read the tree as ambiguous.

One check removes points without failing the article: when the last point of a
parent is followed by a paragraph that is not a sub-item, that paragraph may
speak for the whole clause, so the parent keeps its text and gets no point
children (308 parents in the measured release). Source trees put such a
paragraph in the last point (974 of 974 cases), but a citation of that point
would then claim prose that may belong to the clause.

Reasons counted: `clause_sequence`, `clause_style`, `clause_candidate`,
`point_sequence`, `point_style`, `point_candidate`, `points_before_clauses`,
`single_clause`, `containment`; and `point_tail` for parents left without
points.

## What is recorded

- A clause holds the paragraphs from its start through the paragraph before the
  next clause start or the end of the article; a point holds its paragraphs up
  to the next point start or the end of its parent. The article's lead
  (paragraphs before the first clause) stays in the article only. Text after the
  last clause belongs to the last clause, as in source trees (the clause spans
  above).
- The article's `text` is unchanged and still holds the whole article; each
  child's `text` is its paragraphs joined exactly as the article joins them.
  Every paragraph still sits in exactly one article or structure provision;
  clauses and points are descendants whose text is contained in it.
- Parent links: each parsed provision records its parent's anchor, and extract
  sets `parent_node_id` of a clause to its article and of a point to its clause,
  or to the article for a point directly in it. Chunk follows these links, not
  the anchor's slashes; structure ancestors of the article are unchanged.
- Kinds `khoan` and `diem`; anchors `<article anchor>/khoan-<token>` and
  `.../diem-<token>` (`dieu-5/khoan-2a/diem-đ`, `dieu-3/diem-a` for points
  directly in an article, `kem-theo-1/dieu-4/khoan-2` in an instrument scope);
  labels `Khoản <token>` and `Điểm <token>`; empty heading; ordinals by
  position; locators from the paragraphs. The checks reject repeats, so `~k`
  suffixes never arise here.
- The anchor segment is the kind, `-` and the lowercased token. One function,
  `anchor.Segment(kind, token)` in a new package `apps/corpus/internal/anchor`,
  composes it for source trees (today inline in `titleParts`,
  `apps/corpus/internal/stage/extract/labels.go`), for the text parser (today
  inline in `textparse/parse.go`) and for relations from text (today `/khoan-`
  and `/diem-` plus the lowercased word in `textrule/locators.go`), so one label
  gives one anchor everywhere. Source-tree anchors do not change.
- Node ids follow the existing `tx:` rule (scope index, anchor and text), so a
  clause whose text changes gets a new id. Article ids do not change.
- `text.json` counts per document: articles split, clauses, points, and articles
  or parents left whole by reason. No gap: an article left whole is still a true
  citation.
- The text parser version becomes `textparse/7` and the extract recipe changes
  with it.

## Effects

- Extract, relate, chunk and embed repeat for documents read from parsed text
  (1,070 in the measured release): relate binds the extract manifest hash, so it
  replans even where its relation rows stay equal. Source-tree documents keep
  every artifact.
- Chunk: an article that fits stays one article-owned chunk. A larger article
  splits at its clauses and points instead of at lines, and each chunk cites the
  clause or point that owns it.
- Relations from text ([relations text](relations-text.md)): an anchor such as
  `dieu-5/khoan-2` in a parsed document now resolves where it named a clause;
  build counts the change in `relation_clause_unresolved`.
- Validity: unchanged. Effect articles of parsed documents stay review evidence
  under [text extraction](text-extraction.md#validity-rules). Document status is
  unchanged for clause-owned hits. Relation warnings keep the rule of
  [results](mcp-results.md): a hit owned by a clause carries a warning whose
  target is that clause, an ancestor of it or, for an exact warning, a provision
  inside it; a warning on clause 1 does not reach a hit owned by clause 2, and
  an inexact warning on the article reaches no clause.
- Identity map and manual corrections: existing anchors and article ids keep
  their meaning; new child ids carry no entries.

## Source trees with heading-only articles

7,324 source-tree articles in 686 documents hold a single short line (a
heuristic count of heading-only candidates, not yet compared with each heading),
while other articles of the same tree hold text; the body sits in the bundle's
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

- Fixtures with expected provision lists, parents included, for: clauses with
  points, inserted clauses and points (`2a`, `a1`, `đ`), old dash style,
  footnote markers after the clause number, sub-items, points directly in an
  article, an instrument scope, a lone `1.`, quoted amended clauses, a last
  point followed by clause prose (`point_tail`), a clause that quotes an earlier
  clause verbatim (`containment`), and each rejection reason, including a failed
  candidate after the last clause (`2a)`) and after the last point
  (`c.Nội dung`).
- An integration fixture through extract, chunk and build: clause and point
  provisions have the stated parents and paths; a large article chunks at its
  clauses; a relation text naming `khoản 2 Điều 5` resolves to the clause, and
  one naming a missing point resolves to the deepest node with
  `relation_clause_unresolved`; a clause-owned hit keeps its document's status,
  carries an exact warning on its article or on itself, and carries no warning
  that targets a sibling clause or an inexact warning on the article.
- A comparison test over recorded source-tree bundles that also carry a
  transcription: clause starts read from the transcription equal the tree's
  clause anchors, with each difference listed.
- Source-tree and OCR documents produce byte-identical extract artifacts before
  and after the change; for parsed documents, article node ids, validity
  outcomes, manual-correction evidence and identity-map validation are
  unchanged.
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
