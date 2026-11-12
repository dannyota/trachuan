# Provision-level relations from article text

VBPL's relation diagram names documents, never provisions: all 19,141 relations
imported on run vn/20261111_4 are document-level. The operative article of an
amending, repealing, replacing or suspending act names its targets with one
grammar: point, clause and article labels, then the target document. This note
owns the text rule that turns such sentences into provision-level relations.
[Sources](sources.md#one-hop-relation-context) owns the diagram import and code
table; [validity status](validity-status.md) owns how relations change validity;
[retrieval](retrieval.md) shows provision-level repeal and suspension relations
as warnings on hits, never as a status change.

## Candidate measurements

On run vn/20261111_4: 43 provisions contain "ngưng hiệu lực", of which 2 are
operative sentences naming targets (03/2023/TT-NHNN Article 1 and
10/2023/TT-NHNN Article 1, the second naming three clauses of one article);
1,265 `amends_supplements` and 871 `repeals` document-level relations exist
whose operative articles use the same grammar. These are candidate counts;
emitted rows are known only after the parser and the deduplication rule below
run, and the relate stage reports them per kind and per gap.

## One fail-closed rule

A text relation exists only when every part of the sentence is accounted for by
the grammar, the target document is a diagram target of the same source
document, the target article is unambiguous and the requested provision exists.
Every other outcome is a counted gap, never a relation and never a guess.

## Inputs and identity

- The relate item for a document reads two artifacts: the fetch artifact
  (diagram and references, as today) and the extract artifact (the document's
  own provisions). Its input hash becomes
  `sha256("fetch:<fetch sha256>\nextract:<extract sha256>")`; `Plan` selects
  documents whose fetch and extract items are done, and build checks the same
  pair. The relate recipe gains `text_rules:"vn-targets/1"`. Recipes freeze when
  a run first includes the stage ([pipeline](pipeline.md)), so the change
  applies to new runs only; an existing run keeps its frozen relate recipe and
  input identity.
- The artifact stays run independent: it records what the sentence says and the
  diagram target's VBPL id, never a resolved provision of another document.
  Resolution against the run (membership, `unknown_target`) stays in `Resolve`;
  mapping anchors to provision rows stays in build.

## Grammar

Text comes from the source document's extract artifact. For each `dieu`
provision whose text is not empty, the parser normalizes the text (NFC,
whitespace collapsed to one space, ASCII digits, letters compared
case-insensitively) and splits it into sentences at a full stop followed by a
space or the end; a full stop inside a number token (`39/2016/TT-NHNN.` only at
the end) does not split. Quotation marks (`"`, `“ ”`, `« »`, `'`) are tracked
across the provision: text inside a quotation is never a candidate, and a
provision with unbalanced quotation marks yields nothing with gap
`relation_text_unbalanced_quote`. The parser is a hand-written scanner over
tokens; no pattern language.

A candidate sentence has, in order:

1. An optional list marker (`N.` or `x)`).
2. A verb phrase: one or more verbs from the table below joined by `,` or `và`.
   All verbs must map to one kind; a mixed phrase (`sửa đổi, bãi bỏ`) rejects
   the sentence with gap `relation_text_mixed_verbs`.
3. An optional date range: `từ ngày <date>` or `kể từ ngày <date>`, optionally
   `đến hết ngày <date>` or `đến ngày <date>`, dates under the shared date
   normalizer ([index](index.md#normalization)).
4. One or more target groups. A group is a list of locators joined by `,`, `và`
   or `;` and closed by an `Điều N` locator:
   `(điểm <x>)* (khoản <N>)* Điều <N>`. A trailing `Điều N` scopes every clause
   before it, and a clause scopes every point before it;
   `điểm a, điểm b khoản 2 Điều 5` yields `dieu-5/khoan-2/diem-a` and
   `dieu-5/khoan-2/diem-b`; `khoản 8, khoản 9 và khoản 10 Điều 8` yields three
   clause anchors; `điểm a Điều 5` yields `dieu-5/diem-a`; `Điều 3 và Điều 4`
   yields two article anchors. A group without a closing article, or a point
   after a clause-less article, rejects the sentence with gap
   `relation_text_grammar`.
5. `của` or `tại`, then the document type (`Luật`, `Pháp lệnh`, `Nghị quyết`,
   `Nghị định`, `Quyết định`, `Thông tư`, `Thông tư liên tịch`), optional `số`,
   then exactly one number token under
   [official number normalization](data.md#official-number-normalization-proposed-extension).
6. An optional tail (the usual `ngày ... của <issuer> <title>`), which must
   contain no further locator keyword (`điều`, `khoản`, `điểm`) and no further
   number token; otherwise the sentence names several target documents or
   provisions beyond the grammar and is rejected with gap
   `relation_text_multi_target`.

Anything that does not fit this order rejects the sentence silently when no verb
from the table starts it, and with gap `relation_text_grammar` when a verb
starts it. `Điều 8 Thông tư 39/2016/TT-NHNN` without `của` or `tại` is rejected;
a list spanning two documents is rejected.

| Verb phrase                                                                                                      | Kind                 |
| ---------------------------------------------------------------------------------------------------------------- | -------------------- |
| `sửa đổi`, `bổ sung`, `sửa đổi, bổ sung`                                                                         | `amends_supplements` |
| `bãi bỏ`, `hủy bỏ`                                                                                               | `repeals`            |
| `thay thế`                                                                                                       | `replaces`           |
| `ngưng hiệu lực`, `tạm ngưng hiệu lực`, `tạm hoãn thi hành`, each optionally followed by `thi hành` or `áp dụng` | `suspends`           |

The table is closed; `corrects` (VBPL code 6) is not a kind in the corpus schema
and stays out of this design.

## Target identity

The number token must normalize equal to the `target_number` of exactly one
diagram or references row of the same relate item that carries a
`target_vbpl_id`. That id is the target's identity; the rule never searches the
run by number. Zero matches give gap `relation_text_unlinked_target` with the
number (this counts how often the diagram misses an operative relation, the
10/2023/TT-NHNN case); two or more matches give gap
`relation_text_ambiguous_target`. The target may differ from the diagram row's
kind: a diagram `amends_supplements` row may anchor a text `suspends` relation.

## Artifact rows

Each emitted row has `method:"text_rule/1"`, `raw_code:"text"`, `raw_label` set
to the normalized verb phrase, `kind` from the table, `target_vbpl_id` and
`target_number` copied from the matched diagram row, `source_node_id` of the
article, `target_anchor` as the requested path (`dieu-8/khoan-9`), and
`evidence` set to the matched sentence. Rows with equal (`source_node_id`,
`target_vbpl_id`, `target_anchor`, `kind`) collapse to one, keeping the first
sentence. Diagram rows are unchanged; both coexist and the diagram keeps
document-level credit.

## Resolution in build

Build maps `target_anchor` against the target document's loaded tree by path.
Extraction gives a repeated article label the path `dieu-5~2` and keeps `dieu-5`
for the first; an article is ambiguous when any `dieu-N~k` sibling exists.

| Case                                         | `target_provision_id` | Gap                                |
| -------------------------------------------- | --------------------- | ---------------------------------- |
| Anchor exists                                | that provision        | none                               |
| Clause or point missing, article exists once | the article           | `relation_clause_unresolved`       |
| Article missing                              | null                  | `relation_article_unresolved`      |
| Article ambiguous (`dieu-N~k` exists)        | null                  | `relation_article_ambiguous`       |
| Target document not in the build             | null                  | existing `unknown_target` handling |

The row is inserted in every case because the target document identity is
verified; the requested anchor stays in `target_node_id` so the reader can show
what the act named even when the clause node is missing (29,900 of 57,176
articles on run vn/20261111_4 have no clause nodes; the text-structure work in
Plan 7 adds them and later builds resolve more anchors without a relate rerun).

## What this does not do

It does not read consolidated texts (flat trees of `other` nodes on VBPL), does
not infer targets from titles or from documents the diagram does not link, and
does not decide validity: a suspension relation becomes a warning on the
target's hits and an input for [validity status](validity-status.md). It does
not add relation kinds; code 6 (`corrects`) and code 13 stay
`unknown_relation_code` gaps until a schema change adds a kind.

## Evaluation

The relate stage report counts emitted rows per kind and gaps per name.
Acceptance fixtures in the parser's test data cover: the three-clause
suspension, a point list under one clause, two articles, `sửa đổi, bổ sung` with
a title tail, a date range; rejection fixtures cover: a quoted replacement
article, a missing `của`, two document numbers, mixed verbs, an unlinked number.
The development case dev-004 stays an honest retrieval miss until
39/2016/TT-NHNN Article 8 has clause nodes; the relation makes the suspension
visible on that article's hits.
