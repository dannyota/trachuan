# Provision-level relations from article text

VBPL's relation diagram names documents, never provisions: every one of the
19,141 imported relations on run vn/20261111_4 is document-level, and the
diagram carries no suspension for a notice such as 10/2023/TT-NHNN Article 1
("Ngưng hiệu lực thi hành khoản 8, khoản 9 và khoản 10 Điều 8 của Thông tư số
39/2016/TT-NHNN"). The operative sentence of an amending, repealing or
suspending article names its targets with one grammar: clause and article
labels, then the target document number. This note owns the text rule that turns
such sentences into provision-level relations. [Sources](sources.md) owns the
diagram import; [validity corrections](validity-status.md) owns how relations
change validity; [retrieval](retrieval.md) shows provision-level repeal and
suspension relations as warnings on hits.

## Scope and yield

Measured on run vn/20261111_4: 43 provisions contain "ngưng hiệu lực", of which
2 are operative acts naming a target (03/2023/TT-NHNN Article 1 and
10/2023/TT-NHNN Article 1); 1,265 `amends_supplements` and 871 `repeals`
document-level relations exist whose source articles use the same grammar. The
rule is written for all three kinds; suspension alone would yield 2 rows.

## Rule

The relate stage reads every `dieu` provision of a document that the diagram
links to the target by `amends_supplements`, `repeals` or `suspends`, or whose
heading starts with a verb of those kinds. The verb list is closed: "sửa đổi",
"bổ sung", "bãi bỏ", "hủy bỏ", "thay thế", "ngưng hiệu lực", "tạm ngưng hiệu
lực", "tạm hoãn thi hành", with "thi hành" and "áp dụng" optional after the
suspension verbs. One sentence yields relations when it matches, in order: the
verb; an optional date range ("từ ngày ... đến hết ngày ..."); one or more
target groups, each `(điểm <x> )?(khoản <N> )?Điều <N>` joined by commas, "và"
or semicolons; then "của" or "tại"; then the document type word, optional "số",
and the document number. Anything else in the sentence fails the match. The
match is on the normalized sentence ([normalization](normalization.md)); no
other pattern language is used, and the grammar is small enough to implement as
a hand-written scanner.

Fail closed, per target group:

- The number must resolve to exactly one document of the run by normalized
  number; otherwise the sentence yields nothing and the relate artifact gets a
  gap `relation_target_unresolved` with the number.
- The article anchor `dieu-<N>` must exist in the target document; otherwise the
  group is skipped with gap `relation_target_article_missing`.
- A clause or point target is written as `dieu-N/khoan-M` or
  `dieu-N/khoan-M/diem-x` only when that provision exists in the target; when
  the target article has no clause nodes (29,900 of 57,176 articles on run
  vn/20261111_4, many with clauses written inline) the relation lands on the
  article with gap `relation_target_clause_unresolved`, so the warning still
  reaches the article's hits.
- A document whose own text is not complete (`text_complete` false) yields no
  text relations.

Each relation records `method:"text_rule/1"`, `source_provision_id` (the
article), `target_provision_id` (article or clause), the matched sentence as
`evidence`, and the kind from the verb. It never replaces a diagram relation;
both coexist, and the diagram keeps document-level credit. The relate recipe
gains `text_rules:"vn-targets/1"`, so the first run after this change reruns
relate for every document.

## What this does not do

It does not read consolidated texts (their trees are flat on VBPL, 06/VBHN-NHNN
has 178 untyped nodes), does not infer targets from titles, and does not decide
validity: a suspension relation becomes a warning on the target's hits and an
input for [validity corrections](validity-status.md), never a status change by
itself.

## Evaluation

The relate stage reports counts per kind and per gap. The development case
dev-004 (10/2023/TT-NHNN Article 1) stays an honest retrieval miss until the
target clauses exist in a structured text; the relation makes the suspension
visible on the hits of 39/2016/TT-NHNN Article 8.
