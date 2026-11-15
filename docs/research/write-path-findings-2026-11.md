# Write-path detection and reviewed inputs, 2026-11-15

Survey of main on 2026-11-15 for
[ADR 0017](../adr/0017-findings-and-reviewed-knowledge.md): what the corpus
pipeline detects and what reviewed inputs it applies today.

## Detection today

- Gaps: every stage writes rows to the workshop `gap` table
  (`run_id, stage, item_key, kind, detail, accepted_at, note`); build copies
  them to `coverage_gap` in the release. Kinds are plain strings in each stage;
  there is no registry and no check constraint. Every gap must be accepted
  before release, and acceptance is by kind for one run
  (`apps/corpus/internal/run/gaps.go`).
- `partial_status_unexplained`: a `partial` document with no acting relation.
  Any resolved document-level relation, the source diagram's included, counts as
  an explanation (`apps/corpus/internal/stage/build/partial_review.go`), so a
  missing provision-level repeal does not fire it.
- `validity-review.json`: 16 entry kinds and the `partial_documents` list,
  written by build. Counts go to `build.json`. No code reads the entries.
- OCR cross-check: verdicts and items become `ocr_rejected`,
  `ocr_label_flagged`, `ocr_flagged`; `ocr-review.json` is written to the
  working directory, not kept as an artifact.
- Relation gaps from relate and build (`relation_text_*`,
  `relation_*_unresolved`); text gaps that set `text_complete` false; membership
  gaps, of which only unaccepted removals carry to later runs.
- Evaluation: floors, rejections, `mapping_incomplete`; `previous_diff` compares
  per-case results and gates recall@20 and MRR@8 only.
- Nothing compares gap counts, review-queue counts or `documents_by_*` counts
  with the previous release.

## Reviewed inputs today

| Input                                   | Granularity                   | Evidence                                    | When the source changes                     |
| --------------------------------------- | ----------------------------- | ------------------------------------------- | ------------------------------------------- |
| `sources/vn-corrections.yaml` (39)      | document                      | anchor and verbatim quote, or URL and quote | missing quote fails the build               |
| `sources/vn-ocr-waivers.yaml` (1)       | document                      | each flagged item with page and reading     | hash mismatch voids it (`ocr_waiver_stale`) |
| identity map (209)                      | document, provision, relation | key, anchor, quoted span                    | `mapping_incomplete`                        |
| pins, named numbers (`sources/vn.yaml`) | document                      | verified artifacts                          | run failure or gap                          |
| taxonomy overrides (empty)              | provision                     | reason                                      | gap                                         |
| gap acceptance                          | kind, per run                 | owner's note                                | not carried over                            |
| issue reports                           | document or provision         | typed suggestion                            | exported to files nothing reads             |

Three quote rules differ: corrections match raw, the identity map after NFC and
white-space collapse, OCR quotes after white-space collapse.

## Not expressible today

- A reviewed provision-level effect: `provision_validity` exists and is never
  written; a correction sets document fields only.
- A reviewed manual relation ("not defined", relations-text.md).
- "Reviewed, no change" on a queue entry; the build emits it again.
- Acceptance of one gap rather than a kind.

## The four defects against this map

1. Repealed article served as current: the repealing circular was excluded from
   text relations by an incomplete-text gap; the `partial` check was satisfied
   by the diagram's document-level relation. Counts on `vn/20261114_5`: 394
   `partial` documents, 274 with no incoming repeal relation, 80 with no acting
   relation of any kind.
2. False OCR label flag: the flagged items were annex form codes; 196 documents
   carry the same flag kind.
3. Nine documents with leading white space in `number` and `title`: only
   `doc_type` and `issuer` are normalized.
4. A scheduled backup skipped without a failure (host, not corpus): the same
   pattern, a skip recorded as success.
