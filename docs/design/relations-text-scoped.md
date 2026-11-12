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
title and 28 none of these. These are heuristic candidate counts from a pattern
scan, not parser output; one item can yield several rows. In 679 of the items
the article's own text holds no item line and the items sit only in clause nodes
(Article 1 of the act above holds its heading, its four items are clause nodes);
version 1 reads only article text and never saw them.

## Reading the article

Version 2 reads an article as one stream: the article's text, then each
descendant `khoan` and `diem` node in preorder, skipping a node whose normalized
text is contained in the article's or an already read ancestor's normalized
text. Quotation state carries across the whole stream: text inside a quotation
is never a candidate, a quotation inside a candidate is one opaque token, and a
stream that ends inside a quotation yields nothing for the article
(`relation_text_unbalanced_quote`).

A line break ends a sentence only when the next line starts with a list marker
(`N.` or `x)`), an article label or a quotation mark; any other line continues
the sentence, joined by one space. Within a line the version 1 full-stop rule
splits.

After a sentence that ends with `như sau` (optionally `:`), every following
sentence up to the next sentence at that sentence's list level or higher is the
new wording it introduces, quoted or not, and never yields a row. These rules
apply to version 1 sentences as well.

## Scope lead

The article's first sentence, after the label `Điều N.`, is a scope lead when it
reads, in order:

1. One or more verbs of the table joined by `,` or `và`.
2. An object, either a scope list (optional `một số` or `các`, then nouns from
   the closed list `điều`, `điều khoản`, `khoản`, `điểm`, `nội dung`,
   `quy định`, `cụm từ`, `từ`, `từ ngữ`, `mẫu`, `mẫu biểu`, `biểu mẫu`,
   `phụ lục`, joined by `,` or `và`; about 480 of the 563 leads) or one version
   1 target group closing in one article (`khoản 2 Điều 8`; about 20 leads).
3. `của` or `tại`, then version 1 items 5 and 6 unchanged: type, number, issuer
   table, tail, stated date.

Opening a scope and emitting a row are separate. The scope opens when the lead's
target passes the version 1 identity rule; that target is the scope document,
and a target-group object makes its article the scope article. The lead emits a
row of its own only when its object is a target group and its verbs map to one
kind (version 1); a target group with mixed verbs opens the scope and gets
`relation_text_mixed_verbs` for itself. A lead that fails identity keeps its
version 1 gap (`relation_text_unlinked_target` for a number without a diagram
link), and each item under it yields gap `relation_text_scope_rejected` and no
row. A lead that starts with a verb but fits neither object form gets
`relation_text_grammar` and opens no scope.

## Items

An item is a top-level entry of the scope: in the article's text, a sentence
that starts a line with marker `N.`; in the tree, the first sentence of a direct
`khoan` child of the article. It starts, after the marker, with a verb of the
table or with a phrase starter of the special forms, and names no document (no
type word followed by a number token). It reads version 1 items 1 to 4 (marker,
verb phrase of one kind, date range, target groups), then optionally `như sau`
and `:`, and nothing else. A target group may lack its closing article only when
a scope article exists, and then closes on it. A candidate that names a document
is a version 1 sentence with its own target, never an item.

Every other sentence in a scope that starts with a verb outside a quotation (a
point line `x)`, a sentence in a `diem` node or deeper, the second sentence of
an item, unquoted new wording after `như sau`) is a nested instruction: gap
`relation_text_nested_scope`, no row. Version 2 never carries the scope's
article or document below the top level.

An item row is a version 1 row whose `target_vbpl_id`, `target_number` and
`stated_issue_date` come from the lead and whose `target_anchor` comes from the
item. `method` stays `text_rule/1`: it names the evidence contract (operative
text, requested path in `target_node_id`), not the grammar version, which is the
relate recipe's `text_rules:"vn-targets/2"`; build, warnings and readers need no
change. Its `evidence` is the lead sentence, a line break, then the item
sentence, so the row alone names both the document and the provision.

## Special forms

These apply to every candidate, item or version 1 sentence, and set the kind
whatever the verb:

| Form                                                                                                           | Example                                                                          | Row                                                                                                                                       |
| -------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------- |
| `bổ sung <new locators> vào sau` or `vào trước <group>`                                                        | `Bổ sung khoản 4 và khoản 5 vào sau khoản 3 Điều 113`                            | `amends_supplements` at the parent of the group's last component (`dieu-113`); a new article gives `relation_text_document_level`, no row |
| `bổ sung <new locators> vào` or `vào cuối <group>`                                                             | `Bổ sung điểm đ vào khoản 2 Điều 5`                                              | `amends_supplements` at the group (`dieu-5/khoan-2`)                                                                                      |
| `thay thế [cụm] từ <quote> bằng [cụm] từ <quote> tại <groups>`; `bỏ` or `bãi bỏ [cụm] từ <quote> tại <groups>` | `Thay thế cụm từ "..." bằng cụm từ "..." tại tên Điều 18 và tại khoản 2 Điều 18` | `amends_supplements` at each group: a phrase changes wording, not the provision's force                                                   |
| `sửa đổi`, `bổ sung` or `thay thế` with `tên` or `tên của` before a group                                      | `Sửa đổi tên Điều 18 như sau:`                                                   | `amends_supplements` at the group (`dieu-18`); `bãi bỏ tên` gets `relation_text_grammar`                                                  |
| First locator `phụ lục`, `mẫu`, `biểu mẫu`, `chương`, `mục`, `phần`                                            | `Bãi bỏ Phụ lục số 01`                                                           | no row, gap `relation_text_unsupported_locator`                                                                                           |

New locators before `vào` are not anchors and need no closing article. `bỏ`
starts a candidate only when `cụm từ` or `từ` follows it, and admits only the
phrase form; it is not a verb of the table.

## Checks

- Accept fixtures: both articles of 19/2018/TT-NHNN's amending act above (items
  in the article text and in clause nodes); a lead with a target group and an
  item `Bãi bỏ khoản 3.` (rows `dieu-8` and `dieu-8/khoan-3`); a mixed-verb
  target-group lead whose items still emit; each special form in the table; an
  item continued on the next line with its own document (`1. Bãi bỏ Điều 5` /
  `của Thông tư số 39/2016/TT-NHNN.`, a version 1 row on 39/2016/TT-NHNN).
- Reject fixtures: items under a lead whose number has no diagram link
  (`relation_text_scope_rejected`); a lead whose tail names a second number;
  `Bãi bỏ cụm từ ... tại khoản 2 Điều 5` emitting `repeals`;
  `Bãi bỏ tên Điều 18`; an annex item; a new article; a clause item without a
  scope article; a point instruction under `Sửa đổi khoản 2 như sau:`
  (`relation_text_nested_scope`, not `dieu-8/diem-b`); a quotation opened in a
  clause node and closed in a later point node, with an unquoted directive
  between them; new wording after `như sau` without quotation marks.
- On the next evaluated run the relate report counts rows per kind and gaps per
  name. The primary reads 50 item rows drawn at random against the source
  article; a wrong document or anchor blocks the recipe until a fixture covers
  it. ADR 0010 gates and the ADR 0013 compact size gate must still pass, since
  warnings add to hits.

## Not in version 2

Chapter or section headings as leads (the 11/2026/TT-NHNN Chapter VIII case);
nested instructions (counted as `relation_text_nested_scope`); amendment-history
parentheticals in a tail (`(đã được bổ sung theo ...)`, which hold another
document number and keep version 1's `relation_text_multi_target`; 87 rejected
sentences on run vn/20261112_2, among them dev-004's); annex anchors; and
targets without a diagram link. Version 3 decides the last from the
`relation_text_unlinked_target` count of leads, which version 2 makes
measurable.
