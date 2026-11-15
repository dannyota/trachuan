# Validity review queue, release run 20261114_5

Observations, not project authority. The rules are in
[validity status](../design/validity-status.md).

## Queue by kind

The build's `validity-review.json` for `vn/20261114_5` holds 1,550 entries and
394 partial documents.

| Kind                  | Entries |
| --------------------- | ------: |
| `unresolved_article`  |     699 |
| `parsed_text_article` |     384 |
| `unknown`             |     196 |
| `repeals_current`     |     194 |
| `partial_target`      |      39 |
| `replaces_unverified` |      22 |
| `rule_b_skipped`      |       9 |
| `date_conflict`       |       4 |
| `correction_external` |       2 |
| `rule_c_conflict`     |       1 |

878 entries carry a parser proposal.

## Small kinds, reviewed 2026-11-15

The 77 entries of the six small kinds were drafted against the release's
provision text by Codex `gpt-6-astra` and every draft was read by the primary.

| Kind                  | Correction | No change | Already corrected |
| --------------------- | ---------: | --------: | ----------------: |
| `partial_target`      |          9 |        30 |                 0 |
| `replaces_unverified` |          1 |        20 |                 1 |
| `rule_b_skipped`      |          0 |         9 |                 0 |
| `date_conflict`       |          4 |         0 |                 0 |
| `correction_external` |          0 |         0 |                 2 |
| `rule_c_conflict`     |          1 |         0 |                 0 |

The nine `rule_b_skipped` entries name targets that are not in the corpus. The
four date conflicts are source date errors that the documents' own commencement
articles settle. One further correction came from reading the evidence: Law
38/2019/QH14 takes effect on 2020-07-01 (Điều 151 khoản 1), not the source date
2019-07-01. Five corrections set only `status_class` because the acting
provision names no date.

The four bulk kinds (1,473 entries) are reviewed in a later batch under the
model-adjudicated rule in validity status.

## Partial documents without relations

On `vn/20261114_5` all 394 documents with source status `HHL1P` ("Hết hiệu lực
một phần") have no `provision_validity` row, as the table is empty by design.
274 have no incoming `repeals` relation, and 80 have no incoming `repeals`,
`replaces`, `amends_supplements` or `suspends` relation at all. Both counts feed
the bulk review batch.

## MRR@8 losses on `vn/20261115_2`

Against `vn/20261114_5`, recall@20 rose from 0.925 to 0.957 and old-product
MRR@8 fell from 0.7247 to 0.7226 (exact) and 0.7241 to 0.7226 (ANN). Four
old-product cases lost their first correct rank. Three of the four causes are
validity doing its job, which is a better picture than a text regression:

| Case                               | Rank   | Cause                                                                                                                                                                                         |
| ---------------------------------- | ------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `ai-law-high-risk-134-2025`        | 1 to 2 | 53/2026/TT-NHNN entered into force (`not_yet` to `in_force`); its Điều 4 sets banking-specific high-risk thresholds and ranks first. The question is about the Law, so the case is unchanged. |
| `ai-law-scope-134-2025`            | 1 to 2 | The same circular's Điều 1 (its own scope) ranks first; the question names the Law, so the case is unchanged.                                                                                 |
| `chu-ky-so-chuyen-dung-130-2018`   | 2 to 7 | 130/2018/NĐ-CP is expired by a reviewed correction (ended by 23/2025/NĐ-CP), so it is demoted; 23/2025/NĐ-CP Điều 14, its successor with the same title, is added as an alternate.            |
| `new-interbank-high-value-08-2024` | 4 to 9 | 111/2026/VBHN-TT-NHNN switched to its transcription and its indicator-table rows lead the lexical arm; a text effect, tracked in the backlog.                                                 |

Development case `dev-027` also moved, rank 3 to 5, without a gate.
