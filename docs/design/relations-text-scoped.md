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
scan, not parser output; one item can yield several rows. For 679 of the items
the scan found no item line in the article's text and took them from descendant
nodes instead (Article 1 of the act above holds only its heading; its four items
are clause nodes).

## One fail-closed reading rule

Version 2 reads an article only when its structure is unambiguous, and an
article it cannot read yields no text row at all. Three conditions make it
unambiguous; each failure is one counted gap for the article:

1. **Source order.** Either the article's text contains the normalized text of
   every descendant node, and the article's text alone is read; or the article
   has descendants, its text is only its label and lead, and it contains none of
   theirs, and the lead is read, then each direct `khoan` child's subtree by the
   same rule. Anything else (partial containment, own text besides the lead next
   to separate children) gives `relation_text_source_order`.
2. **Quoted wording.** A sentence ending in `như sau` (optionally `:`) after an
   item introduces new wording, which must be exactly one quotation opening
   right after it; the quotation's text never yields a row. An item's `như sau`
   followed by anything else gives `relation_text_unquoted_wording`. A
   scope-list lead's `như sau` introduces its item list instead. Quotation state
   carries across the article's read units; a quotation still open at the end of
   an item's unit, or of the article, gives `relation_text_unbalanced_quote`.
3. **Sentence ends.** A line break ends a sentence only before a line that
   starts with a list marker (`N.` or `x)`) or a quotation mark; every other
   line continues the sentence, joined by one space, so `Điều 5.` on its own
   line belongs to the locator before it. A sentence other than the article's
   first must end with `.`, `;`, `:` or the end of its unit; one that breaks
   before a marker line without them gives `relation_text_grammar`.

These rules apply to version 1 sentences as well, since version 2 replaces how
an article is read. A scan of run vn/20261112_2 with the scope-list leads below
found 494 such articles: 410 read their text alone, 75 read a heading-only lead
and its clause nodes, and 9 fail source order. Of 2,313 numbered items in them,
1,616 end in `như sau` followed by a quotation, 440 have no `như sau`, and 257
have unquoted new wording, which drops their articles.

## Scope lead

The article's first sentence, after the label `Điều N.`, is a scope lead when it
reads, in order:

1. One or more verbs of the table joined by `,` or `và`. Kinds may differ, since
   a lead emits no row.
2. A scope list: optional `một số` or `các`, then nouns from the closed list
   `điều`, `điều khoản`, `khoản`, `điểm`, `nội dung`, `quy định`, `cụm từ`,
   `từ`, `từ ngữ`, `mẫu`, `mẫu biểu`, `biểu mẫu`, `phụ lục`, joined by `,` or
   `và` (about 480 of the 563 leads).
3. `của` or `tại`, then version 1 items 5 and 6 unchanged: type, number, issuer
   table, tail, stated date.

The scope opens when the lead's target passes the version 1 identity rule; that
target is the scope document. A lead that fails identity keeps its version 1 gap
(`relation_text_unlinked_target` for a number without a diagram link), and each
item under it yields gap `relation_text_scope_rejected` and no row. A lead whose
object is a target group (`Sửa đổi Điều 8 của ...`) is a version 1 sentence and
opens no scope, because its `như sau` cannot tell an item list from the
article's new wording. A lead that starts with a verb but fits neither gets
`relation_text_grammar`.

## Items

An item is a top-level entry of the scope: in the article's text, a sentence
that starts a line with marker `N.`; in a direct `khoan` child, the child's
first sentence. It starts, after the marker, with a verb of the table or with a
phrase starter of the special forms, and names no document (no type word
followed by a number token). It reads version 1 items 1 to 4 (marker, verb
phrase of one kind, date range, target groups, each group closed by an article),
then optionally `như sau` and `:`, and nothing else. A candidate that names a
document is a version 1 sentence with its own target, never an item.

Every other sentence in a scope that starts with a verb outside a quotation (a
point line `x)`, a sentence in a `diem` node or deeper, the second sentence of
an item) is a nested instruction: gap `relation_text_nested_scope`, no row. The
scope document is never carried below the top level.

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
  in the article text, read alone; items in clause nodes after a heading-only
  article); a scope-list lead ending in `như sau:` followed by items; an item
  whose `như sau:` opens a quotation that spans its point nodes; each special
  form in the table; a locator continued on the next line (`1. Bãi bỏ khoản 2` /
  `Điều 5.`, anchor `dieu-5/khoan-2`); an item continued with its own document
  (`1. Bãi bỏ Điều 5` / `của Thông tư số 39/2016/TT-NHNN.`, a version 1 row on
  39/2016/TT-NHNN).
- Reject fixtures: items under a lead whose number has no diagram link
  (`relation_text_scope_rejected`); a lead whose tail names a second number;
  `Bãi bỏ cụm từ ... tại khoản 2 Điều 5` emitting `repeals`;
  `Bãi bỏ tên Điều 18`; an annex item; a new article; a clause item without its
  article; a target-group lead with clause items (no scope); a point instruction
  (`relation_text_nested_scope`); an item `Sửa đổi khoản 2 Điều 8 như sau:`
  followed by an unquoted `2. Bãi bỏ Điều 5.` (`relation_text_unquoted_wording`,
  no repeal); an article whose own text holds one item while another item is a
  separate clause node (`relation_text_source_order`); a quotation opened in one
  clause and closed in the next.
- On the next evaluated run the relate report counts rows per kind and gaps per
  name. The primary reads 50 item rows drawn at random against the source
  article; a wrong document or anchor blocks the recipe until a fixture covers
  it. ADR 0010 gates and the ADR 0013 compact size gate must still pass, since
  warnings add to hits.

## Not in version 2

Chapter or section headings as leads (the 11/2026/TT-NHNN Chapter VIII case);
leads naming one article with clause items; unquoted new wording; nested
instructions (counted as `relation_text_nested_scope`); amendment-history
parentheticals in a tail (`(đã được bổ sung theo ...)`, which hold another
document number and keep version 1's `relation_text_multi_target`; 87 rejected
sentences on run vn/20261112_2, among them dev-004's); annex anchors; and
targets without a diagram link. Version 3 decides the last from the
`relation_text_unlinked_target` count of leads, which version 2 makes
measurable.
