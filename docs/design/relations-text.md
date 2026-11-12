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

A text relation row exists only when every part of the sentence is accounted for
by the grammar and the target document is a diagram or references target of the
same source document whose stated type, year and date agree with it. Its
provision endpoint is set only when the requested anchor resolves unambiguously
in the target's tree; otherwise the endpoint is null or the deepest resolved
ancestor on the requested path, with a named gap. Every other outcome is a
counted gap, never a row and never a guess.

## Inputs and identity

- The relate item for a document reads two artifacts: the fetch artifact
  (diagram and references, as today) and the extract artifact (the document's
  own provisions). Its input hash becomes
  `sha256("fetch:<fetch sha256>\nextract:<extract sha256 or ->")`. `Plan`
  selects documents whose fetch item is done and whose extract item is done or
  ended as a gap without an artifact (`tree_too_large`, a source gap); the
  second case writes `-` and the item imports the diagram only, so diagram
  import never depends on extraction. Relate processing, the build relation
  reader and the build identity-inventory reader validate the same pair with one
  shared function ([pipeline](pipeline.md#items-artifacts-and-gaps)). The relate
  recipe gains `text_rules:"vn-targets/1"` (`vn-targets/2` with
  [scoped amendment lists](relations-text-scoped.md)). Recipes freeze when a run
  first includes the stage, so the change applies to new runs only; an existing
  run keeps its frozen relate recipe and input identity.
- The artifact stays run independent: it records what the sentence says and the
  diagram target's VBPL id, never a resolved provision of another document.
  Resolution against the run (membership, `unknown_target`) stays in `Resolve`;
  mapping anchors to provision rows stays in build.

## Grammar

Text comes from the source document's extract artifact. A source document whose
extract evidence makes `text_complete` false
([text extraction](text-extraction.md#what-is-recorded): `tree_headings_only`,
`structure_uncertain`, `table_text`, `image_content`, `ocr_flagged`,
`ocr_rejected`, `ocr_label_flagged` or `needs_ocr`) yields no text relations and
gets gap `relation_text_source_incomplete`; an incorrectly read sentence must
not create a relation. For each `dieu` provision whose text is not empty, the
parser normalizes the text (NFC, whitespace collapsed to one space, ASCII
digits, letters compared case-insensitively) and splits it into sentences at a
full stop followed by a space or the end; a full stop inside a number token
(`39/2016/TT-NHNN.` only at the end) does not split. Quotation marks (`"`,
`“ ”`, `« »`, `'`) are tracked across the provision: text inside a quotation is
never a candidate, and a provision with unbalanced quotation marks yields
nothing with gap `relation_text_unbalanced_quote`. The parser is a hand-written
scanner over tokens; no pattern language.

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
   `(điểm <x>)* (khoản <N>)* Điều <N>`. Scoping is left to right: a run of
   points is consumed by the first clause (or, with no clause, the closing
   article) that follows it; a clause with no unconsumed points before it is a
   whole-clause target; every clause is scoped by the closing article.
   `điểm a, điểm b khoản 2 Điều 5` yields `dieu-5/khoan-2/diem-a` and
   `dieu-5/khoan-2/diem-b`; `điểm a khoản 1 và khoản 2 Điều 5` yields
   `dieu-5/khoan-1/diem-a` and `dieu-5/khoan-2`;
   `khoản 8, khoản 9 và khoản 10 Điều 8` yields three clause anchors;
   `điểm a Điều 5` yields `dieu-5/diem-a`; `Điều 3 và Điều 4` yields two article
   anchors. A group without a closing article rejects the sentence with gap
   `relation_text_grammar`.
5. `của` or `tại`, then the document type (`Luật`, `Pháp lệnh`, `Nghị quyết`,
   `Nghị định`, `Quyết định`, `Thông tư`, `Thông tư liên tịch`), optional `số`,
   then exactly one number token under
   [official number normalization](data.md#official-number-normalization-proposed-extension).
   The token must contain a four-digit year segment (a yearless number such as
   `01/QĐ-UBND` rejects the sentence with gap `relation_text_yearless_number`),
   and its type suffix must agree with the type word (`TT` Thông tư, `TTLT`
   Thông tư liên tịch, `NĐ` Nghị định, `QĐ` Quyết định, `NQ` Nghị quyết, `QH`
   Luật, `UBTVQH` Pháp lệnh); disagreement rejects with gap
   `relation_text_type_mismatch`. The issuer segment (the part after the type,
   `NHNN` in `TT-NHNN`, `CP` in `NĐ-CP`) must be a key of the closed table
   `central_issuers` in `sources/vn.yaml`, which maps each code to the issuer's
   normalized name (`CP`, `TTg`, `QH`, `UBTVQH`, `NHNN`, `BTC`, `BTP`, `BCA`,
   `BQP`, `BKHĐT`, `BCT`, `BGTVT`, `BXD`, `BTNMT`, `BTTTT`, `BLĐTBXH`, `BYT`,
   `BGDĐT`, `BKHCN`, `BNV`, `BNG`, `BNNPTNT`, `BVHTTDL`, `UBDT`, `TTCP`, `VPCP`,
   `KTNN`); the table's hash is part of the relate recipe. For `QH` and `UBTVQH`
   the segment carries the legislature number (`QH15`, `UBTVQH14`): the trailing
   digits are dropped before the table lookup and kept in the recorded number.
   Any other segment, including `UBND` and `HĐND`, rejects the sentence with gap
   `relation_text_local_number`. Build then verifies the target: the target
   document's issuer from its own frozen fetch metadata, normalized under
   [identity comparison](data.md#identity-comparison), must equal the table's
   name for the code; a different, missing or unparsable issuer drops the row
   with gap `relation_text_identity_conflict`. Provincial documents therefore
   get no text relations in version 1.
6. An optional tail (the usual `ngày ... của <issuer> <title>`), which must
   contain no further locator keyword (`điều`, `khoản`, `điểm`) and no further
   number token; otherwise the sentence names several target documents or
   provisions beyond the grammar and is rejected with gap
   `relation_text_multi_target`. A date in the tail (`ngày D tháng M năm Y` or
   `ngày D/M/Y`) is normalized and recorded on the row as `stated_issue_date`;
   build compares it with the target document's issue date and drops the row
   with gap `relation_text_identity_conflict` on a mismatch. The issuer phrase
   is not compared: tails name officeholders
   (`của Thống đốc Ngân hàng Nhà nước`), not the issuer record, and identity
   rests on the diagram link, the central-body restriction, the year, the type
   and the date.

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
article, `target_anchor` as the requested path (`dieu-8/khoan-9`),
`stated_issue_date` (null when the tail names none), and `evidence` set to the
matched sentence. Rows with equal (`source_node_id`, `target_vbpl_id`,
`target_anchor`, `kind`) collapse to one, keeping the first sentence. Diagram
rows are unchanged; both coexist and the diagram keeps document-level credit.

Run gap rows for text gaps carry a bounded detail,
`<index in the report>:<source node id>:<first 200 characters of the sentence>`,
because the gap table indexes detail; the full sentence stays in the artifact
report's `gap_details`.

## Resolution in build

Build maps `target_anchor` against the target document's loaded tree by path,
one component at a time. Extraction gives a repeated label at any level the path
suffix `~k` (`dieu-5~2`, `dieu-5/khoan-1~2`) and keeps the unsuffixed path for
the first; a component is ambiguous when any `~k` sibling of it exists. Build
walks the requested path from the article down and stops at the first component
that is missing or ambiguous.

| Case                                                     | `target_provision_id`     | Gap                                |
| -------------------------------------------------------- | ------------------------- | ---------------------------------- |
| Every component exists once                              | that provision            | none                               |
| A clause or point component is missing, all above unique | the deepest resolved node | `relation_clause_unresolved`       |
| Any component ambiguous (`~k` sibling)                   | null                      | `relation_anchor_ambiguous`        |
| Article missing                                          | null                      | `relation_article_unresolved`      |
| Stated issue date differs from the target's              | row dropped               | `relation_text_identity_conflict`  |
| Target document not in the build                         | null                      | existing `unknown_target` handling |

Except for the identity conflict, the row is inserted because the target
document identity is verified; the requested anchor stays in `target_node_id` so
the reader can show what the act named even when the clause node is missing
(29,900 of 57,176 articles on run vn/20261111_4 have no clause nodes; the
text-structure work in Plan 7 adds them and later builds resolve more anchors
without a relate rerun).

## What this does not do

It does not read consolidated texts (flat trees of `other` nodes on VBPL), does
not infer targets from titles or from documents the diagram does not link, and
does not decide validity: a suspension relation becomes a warning on the
target's hits and an input for [validity status](validity-status.md). It does
not add relation kinds; code 6 (`corrects`) and code 13 stay
`unknown_relation_code` gaps until a schema change adds a kind.

Known gap, measured 2026-11-12: 11/2026/TT-NHNN amends 47/2025/TT-NHNN (its
Chapter VIII heading reads "SỬA ĐỔI, BỔ SUNG MỘT SỐ ĐIỀU CỦA THÔNG TƯ SỐ
47/2025/TT-NHNN" and changes Article 6 clause 2 from 28 days to 14 working
days), yet neither VBPL diagram links the two documents; only the consolidated
text 48/VBHN-NHNN names the amendment. Version 1 therefore emits nothing for it,
and 47/2025/TT-NHNN serves its old wording as current. Run vn/20261112_2 records
no `relation_text_unlinked_target` gap at all: version 1 never reads a chapter
heading, and it rejects article leads such as `một số điều của` as grammar
before the target check. [Scoped amendment lists](relations-text-scoped.md)
(version 2) read article leads, so their unlinked count measures how often the
diagram misses an amended document; version 3 decides from it whether a
heading-named target with a verified number, type, year and issuer may stand
without a diagram link.

## Evaluation

The relate stage report counts emitted rows per kind and gaps per name.
Acceptance fixtures in the parser's test data cover: the three-clause
suspension, a point list under one clause, two articles, `sửa đổi, bổ sung` with
a title tail, a date range; rejection fixtures cover: a quoted replacement
article, a missing `của`, two document numbers, mixed verbs, an unlinked number.
The development case dev-004 stays an honest retrieval miss: on run
vn/20261111_4 the diagram does not link 10/2023/TT-NHNN to 39/2016/TT-NHNN, and
39/2016/TT-NHNN Article 8 has no clause nodes. Version 1 rejects its sentence
earlier, as `relation_text_multi_target` (run vn/20261112_2), because the tail
holds a parenthetical naming 06/2023/TT-NHNN; version 2 reads that parenthetical
([tails](relations-text-scoped.md#tails)), so the sentence reaches the target
check and becomes a `relation_text_unlinked_target` gap. The suspension becomes
visible only when VBPL adds the link, or when a future manual relation contract
(not defined by [validity status](validity-status.md), which covers validity
corrections only) supplies the verified pair; it resolves to the clause only
when the clause nodes exist.
