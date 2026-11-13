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
  a finding about the whole document goes into `coverage.note`, not into an
  item. The rest of the version 3 text is unchanged.
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
    `placeholder_item`, counted on its own.
  - `coverage.note` is stored with the result as reviewer evidence and never
    changes a verdict; an empty note is allowed.
- Verdicts, the readings rules, the quote check and the retry rule are those of
  the cross-check note.

## Before a pass

A sample run decides whether version 5 replaces version 3 for later passes: 20
documents with a complete version 3 check, chosen deterministically, half of
them from the 27 that failed with a placeholder first. Version 5 is accepted
when no `placeholder_item` or page-0 failure occurs, `coverage_incomplete` stays
below 2 of 20, and the verdicts agree with version 3 on at least 18 of 20 (each
disagreement read by hand). The sample costs about 25 USD at the version 3 rate
and needs a budget from the owner.

## Checks

The primary runs these; they are requirements, not results.

- Prompt and schema fixtures: the instruction states the rendered page count;
  the schema has `coverage` with its three fields and no `pages`.
- Validator fixtures: `pages_read` unequal to `pdf_pages`; `unreadable_pages`
  nonempty and out of range; an item on page 0 and above `pdf_pages`; an
  all-empty item (`placeholder_item`); an empty `items` list with full coverage
  (`clean`); a note with findings (stored, verdict unchanged).
- The failure diagnostic records `coverage` facts (counts and lengths only) for
  schema failures, as it does for items.
- Version 3 results and their checker id are untouched; the extract recipe
  changes only when a later pass under version 5 completes the store.
