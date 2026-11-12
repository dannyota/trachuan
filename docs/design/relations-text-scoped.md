# Scoped amendment lists

Version 2 of [relations from text](relations-text.md). An amending act usually
names its target once, in the lead of an article, and lists the changed
provisions below it without the document:

```text
Điều 2. Thay thế, bãi bỏ một số cụm từ, điểm, khoản, Điều, Phụ lục của Thông tư số 19/2018/TT-NHNN ngày 28 tháng 8 năm 2018 ...
1. Bãi bỏ Điều 5.
2. Bãi bỏ điểm b khoản 1 Điều 6.
```

Version 1 requires every sentence to name its document, so it rejects both
lines. This note owns how version 2 reads such lists. Everything not stated here
is version 1: the verb table, locator groups, number and issuer rules, the
fail-closed rule, identity through the diagram link, artifact rows, build
resolution.

## Measured on run vn/20261112_2

Version 1 emitted 41 text rows (24 `amends_supplements`, 17 `repeals`) next to
19,143 diagram rows. Of its 5,283 `relation_text_grammar` gaps (817 documents),
2,836 are list items naming a provision without a document and 654 are leads
such as `một số điều của`. A scan of the extract artifacts found 563 article
leads in 287 documents with 3,295 list items: 1,730 name an article, 361 name
only a clause or point, 369 insert new provisions, 195 change a phrase, 425 name
a document of their own, 89 an annex or form, 62 a chapter or section, 36 a
title and 28 none of these. Of the items, 679 sit only in clause nodes, never in
the article's text (Article 1 of the same act holds its heading, its four items
are clause nodes), so version 1 never read them.

## Reading the article

Version 2 reads the article subtree: the article's text, then each descendant
`khoan` and `diem` node in preorder, skipping a node whose normalized text is
contained in the article's or an already read ancestor's normalized text. A line
break ends a sentence; the version 1 full-stop rule splits within a line, and a
list marker at the start of a line belongs to the sentence it starts. Quotation
tracking runs per node: text inside a quotation is never a candidate, a
quotation inside a candidate is one opaque token, and a node with unbalanced
quotation marks yields nothing (`relation_text_unbalanced_quote`). These rules
apply to version 1 sentences as well.

## Scope lead

The article's first sentence, after the label `Điều N.`, is a scope lead when it
reads, in order:

1. One or more verbs of the table joined by `,` or `và`. Kinds may differ,
   because a lead emits no row of its own unless it is a version 1 sentence.
2. An object, either a scope list (optional `một số` or `các`, then nouns from
   the closed list `điều`, `điều khoản`, `khoản`, `điểm`, `nội dung`,
   `quy định`, `cụm từ`, `từ`, `từ ngữ`, `mẫu`, `mẫu biểu`, `biểu mẫu`,
   `phụ lục`, joined by `,` or `và`; about 480 of the 563 leads) or one version
   1 target group closing in one article (`khoản 2 Điều 8`; about 20 leads).
3. `của` or `tại`, then version 1 items 5 and 6: type, number, issuer table,
   tail (which may end in `như sau` and `:`), stated date.

The lead's target must pass the version 1 identity rule; it is then the scope
document. A lead whose object is a target group is also a version 1 sentence and
emits its own row; its article is the scope article. A lead that fails identity
keeps its version 1 gap (`relation_text_unlinked_target` for a number without a
diagram link), and each item under it yields gap `relation_text_scope_rejected`
and no row. A lead that starts with a verb but fits neither object form gets
`relation_text_grammar` and opens no scope.

## Items

Inside a scope, every other candidate that starts, after an optional list
marker, with a verb of the table and names no document (no type word followed by
a number token) is an item. It reads version 1 items 1 to 4 (marker, verb phrase
of one kind, date range, target groups), then optionally `như sau` and `:`, and
nothing else. A target group may lack its closing article only when a scope
article exists, and then closes on it. A candidate that names a document is a
version 1 sentence with its own target, never an item.

An item row is a version 1 row whose `target_vbpl_id`, `target_number` and
`stated_issue_date` come from the lead and whose `target_anchor` comes from the
item. `method` stays `text_rule/1`: it names the evidence contract (operative
text, requested path in `target_node_id`), not the grammar version, which is the
relate recipe's `text_rules:"vn-targets/2"`; build, warnings and readers need no
change. Its `evidence` is the lead sentence, a line break, then the item
sentence, so the row alone names both the document and the provision.

## Special forms

These apply to every candidate, item or version 1 sentence:

| Form                                                                                                           | Example                                                                          | Row                                                                                                                                       |
| -------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------- |
| `bổ sung <new locators> vào sau` or `vào trước <group>`                                                        | `Bổ sung khoản 4 và khoản 5 vào sau khoản 3 Điều 113`                            | `amends_supplements` at the parent of the group's last component (`dieu-113`); a new article gives `relation_text_document_level`, no row |
| `bổ sung <new locators> vào` or `vào cuối <group>`                                                             | `Bổ sung điểm đ vào khoản 2 Điều 5`                                              | `amends_supplements` at the group (`dieu-5/khoan-2`)                                                                                      |
| `thay thế [cụm] từ <quote> bằng [cụm] từ <quote> tại <groups>`; `bỏ` or `bãi bỏ [cụm] từ <quote> tại <groups>` | `Thay thế cụm từ "..." bằng cụm từ "..." tại tên Điều 18 và tại khoản 2 Điều 18` | `amends_supplements` at each group, never `replaces` or `repeals`: a phrase changes wording, not the provision's force                    |
| `tên` or `tên của` before a group                                                                              | `tên Điều 18`                                                                    | the group (`dieu-18`)                                                                                                                     |
| First locator `phụ lục`, `mẫu`, `biểu mẫu`, `chương`, `mục`, `phần`                                            | `Bãi bỏ Phụ lục số 01`                                                           | no row, gap `relation_text_unsupported_locator`                                                                                           |

New locators before `vào` are not anchors and need no closing article.

## Tails

Two tail forms join version 1 item 6, for every candidate:

- A parenthetical that opens with `(đã được` and closes in the same sentence is
  the target's amendment history and is one opaque token, whatever it names
  (`(đã được bổ sung theo khoản 2 Điều 1 Thông tư số 06/2023/TT-NHNN ...)` in
  dev-004's sentence). It held 87 rejected sentences on run vn/20261112_2.
- `stated_issue_date` is only the date directly after the number token
  (`ngày D tháng M năm Y` or `ngày D/M/Y`). A date range of item 3 may instead
  close the sentence (`từ ngày 01 tháng 9 năm 2023 cho đến ngày ...`); text
  after its first date holds no locator keyword and no number token.

## Checks

- Accept fixtures: both articles of 19/2018/TT-NHNN's amending act above (items
  in the article text and in clause nodes); a lead with a target group and an
  item `Bãi bỏ khoản 3.` (rows `dieu-8` and `dieu-8/khoan-3`); each special form
  in the table; a linked target with an `(đã được ...)` parenthetical and a
  closing date range.
- Reject fixtures: items under a lead whose number has no diagram link
  (`relation_text_scope_rejected`); a lead whose tail names a second number;
  `Bãi bỏ cụm từ ... tại khoản 2 Điều 5` emitting `repeals`; an annex item; a
  new article; a clause item without a scope article; an item inside a
  quotation; dev-004's sentence (`relation_text_unlinked_target`, not
  `relation_text_multi_target`).
- On the next evaluated run the relate report counts rows per kind and gaps per
  name (at most 3,295 item rows can exist, fewer after identity). The primary
  reads 50 item rows drawn at random against the source article; a wrong
  document or anchor blocks the recipe until a fixture covers it. ADR 0010 gates
  and the ADR 0013 compact size gate must still pass, since warnings add to
  hits.

## Not in version 2

Chapter or section headings as leads (the 11/2026/TT-NHNN Chapter VIII case),
nested scopes (an item naming one article whose own points omit it), annex
anchors, and targets without a diagram link. Version 3 decides the last from the
`relation_text_unlinked_target` count of leads, which version 2 makes
measurable.
