# OCR cross-check version 5

Proposed. Version 5 of the [OCR cross-check](ocr-check.md) changes the
instruction, the answer schema and the validator of
[version 3](ocr-check-v3.md); rendering, request mode and budget rules stay as
version 3 has them. It applies to later passes only: the version 3 pass of
2026-11-12 finishes under version 3, and its results stay as they are.

## Evidence

The version 3 pass (2026-11-12, 285 documents complete by its stop after
batch 9) failed 45 billed answers with `schema:pages` (34) or
`schema:items[0].page` (11), 50.52 USD, about one answer in seven. The failure
diagnostic recorded for the last 16 of them shows one shape:

| Fact                                                  | Count      |
| ----------------------------------------------------- | ---------- |
| Answers holding exactly one item, on page 0           | 16 of 16   |
| of those, `ocr`, `pdf` and `note` all empty           | 15         |
| of those, `pages` 0 as well                           | 12         |
| Item kinds: article_label, extra_text, date, doc. no. | 9, 3, 2, 2 |

The kinds vary with no relation to the document, so the item is a placeholder,
not a finding. It is not an empty result either: of the 27 documents that failed
this way and later completed on an automatic retry, 26 came back `flagged`
(median 3 items) and one `rejected`; none was `clean`. Accepting the placeholder
as a clean check would hide real findings, so version 3 is right to reject it;
version 5 aims to stop the model from writing it.

Structured outputs enforce types and required fields but no numeric bounds
(`minimum` is not enforced), so the schema alone cannot forbid page 0.

## Changes from version 3

| Setting     | Version 3                                  | Version 5                                                   |
| ----------- | ------------------------------------------ | ----------------------------------------------------------- |
| Checker id  | `claude-opus-5-5/high/ocr-check-3`         | `claude-opus-5-5/high/ocr-check-5`                          |
| Instruction | items name their 1-based page              | also: the page count, an empty `items` list, no placeholder |
| Schema      | `same_document`, `pages`, `items`          | `same_document`, `coverage`, `items`                        |
| Validator   | `pages` < 1 fails; item page bound `pages` | item page bound `pdf_pages`; a placeholder item is named    |

- Instruction. The pipeline states the page count it rendered ("There are N
  pages, labelled Page 1 to Page N") and adds: report `items` as an empty list
  when the OCR text has no disagreement; never write an item without a reading;
  a disagreement about the whole document is `same_document` false or items,
  never only a note; `coverage.note` is for what limited the reading (a page too
  faint, a page out of order). The rest of the version 3 text is unchanged.
- Schema. `pages` becomes `coverage`, an object with `pages_read` (integer),
  `unreadable_pages` (array of integers) and `note` (string). It gives the model
  a place for what is not a page finding.
- Validation, in order:
  - `coverage.pages_read` must equal `pdf_pages`, else `page_count_mismatch`.
  - A nonempty `unreadable_pages` (each between 1 and `pdf_pages`) makes the
    attempt fail as `coverage_incomplete`: an unread page is an unfinished
    check, never a clean one.
  - Each item's page must be between 1 and `pdf_pages`, else
    `schema:items[i].page`.
  - An item whose `ocr`, `pdf` and `note` are all empty fails the attempt as
    `placeholder_item`, counted on its own. - A nonempty `coverage.note` makes
    the verdict at least `flagged`: the pipeline cannot tell commentary from a
    disagreement written in the wrong place, so a note always reaches review and
    a check with one is never `clean`. An empty note is allowed and changes
    nothing.
- Stored format. The stored result keeps the version 3 fields: `pages` holds
  `coverage.pages_read`, `pdf_pages`, `items`, `verdict`, and adds
  `coverage_note` (string, may be empty). `ocrstore` decoding accepts
  `coverage_note` only for checker version 5 and later and requires it there;
  results of versions 2 and 3 decode unchanged. `unreadable_pages` is never
  stored: a check with unread pages is not complete.
- Review file. `ocr-review.json` carries `coverage_note` on the document's entry
  when it is nonempty, beside the items.
- Verdicts, the readings rules, the quote check and the retry rule are those of
  the cross-check note.

## Before a pass

A sample run decides whether version 5 replaces version 3 for later passes: 20
documents with a complete version 3 check, chosen deterministically, half of
them from the 27 that failed with a placeholder first. Each document gets one
attempt and, after a failure that is not a placeholder, one retry; a
`placeholder_item` or page-0 failure anywhere fails the sample. Version 5 is
accepted when, in addition, `coverage_incomplete` occurs in fewer than 2 of 20
documents and every difference from version 3 has been read and approved by
hand: a changed verdict, and any item version 3 had and version 5 lacks. An
unapproved loss of an `article_label` or `document_number` item, or an
unapproved move to a cleaner verdict, fails the sample, because it would let OCR
text reach the index that version 3 kept out. The sample costs about 25 USD at
the version 3 rate and needs a budget from the owner.

## Checks

The primary runs these; they are requirements, not results.

- Prompt and schema fixtures: the instruction states the rendered page count;
  the answer schema has `coverage` with its three fields and no `pages`.
- Validator fixtures: `pages_read` unequal to `pdf_pages`; `unreadable_pages`
  nonempty and out of range; an item on page 0 and above `pdf_pages`; an
  all-empty item (`placeholder_item`); an empty `items` list with full coverage
  (`clean`); empty items with a nonempty note (`flagged`, never `clean`).
- Store fixtures: a version 5 result round-trips with `coverage_note`; version 2
  and 3 results decode unchanged; `coverage_note` on a version 3 result is
  rejected; extract reads results of every version; the review file shows a
  nonempty note.
- The failure diagnostic records `coverage` facts (counts and lengths only) for
  schema failures, as it does for items.
- Version 3 results and their checker id are untouched; the extract recipe
  changes only when a later pass under version 5 completes the store.
