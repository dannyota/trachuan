# Validity status corrections

This note owns two build-time corrections to document validity and their
evidence. [Build normalization](index.md#normalization) owns everything else
about status; [retrieval](retrieval-recipe.md#validity-and-evidence) owns
eligibility at query time. Both corrections ship with the next corpus schema
version, together with scope adoption.

## Problem

Run `vn/20261110_2` returned replaced law as current law in four evaluation
cases, measured on the corpus database:

- `52/2024/NĐ-CP` (issued 2024-05-15): VBPL metadata says status `CCHL` (not yet
  in force) with `effFrom` 2027-07-01. Its own Điều 37 khoản 1 says "có hiệu lực
  thi hành từ ngày 01 tháng 7 năm 2024". The document is never a candidate, and
  the decree it replaces, `101/2012/NĐ-CP`, ranks first as in force.
- `134/2025/QH15`: the source status code is empty, so the class is `unknown`
  and the law is excluded, although `effFrom` 2026-03-01 has passed and Điều 34
  states the same date.
- 6 non-consolidated documents have an empty status code; 9 have `CCHL`.

Reason for acting: a wrong "not current" hides the governing text and promotes
the text it replaced. That is worse for an agent than a flagged uncertainty.

## Rule 1: empty source status

Applies to a document that is not consolidated and whose source status code is
empty. Consolidated documents keep the
[consolidated derivation](index.md#normalization).

Derive `in_force,true` with `status_basis = derived_from_dates` when all hold:

1. `eff_from` parsed to a civil date;
2. `eff_to` is absent;
3. no resolved relation of kind `replaces` or `repeals` targets the document
   from a source whose own source status is `in_force` or `partial`.

Record the acceptable gap `status_derived_from_dates` on the document. Otherwise
keep `unknown,false` and `unknown_status`. Query-time date eligibility is
unchanged, so a future `eff_from` stays not current until that date. A source
status that is present but unmapped is never derived.

Reason: the source asserts a start, no end and no replacement; treating that as
unknown removes current law. The trade-off is that an old text the source never
closed can appear as current; the basis and the gap tell the agent the status is
derived, and condition 3 removes the cases the corpus itself can disprove.

## Rule 2: effective date stated by the text

Applies to a document with a provision tree whose source class is `not_yet`, or
whose class comes from Rule 1.

Read the stated date only from articles (`dieu`) whose heading, NFC and
lowercase, equals `hiệu lực thi hành` or `điều khoản thi hành`, plus their
clauses. Accept exactly these forms, after whitespace collapse:

- `có hiệu lực thi hành từ ngày D tháng M năm YYYY`
- `có hiệu lực thi hành kể từ ngày D tháng M năm YYYY`
- the same two without `thi hành`
- `D/M/YYYY` in place of the spelled date

The subject before `có hiệu lực` must be the document itself:
`<loại văn bản> này` (Luật này, Nghị định này, Thông tư này, Quyết định này,
Nghị quyết này, Pháp lệnh này). A sentence about a clause, article or another
document is not a match. A trailing `trừ ...` exception does not prevent the
match.

Outcomes:

| Text evidence                                  | Result                                                                                                                          |
| ---------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------- |
| One distinct stated date, equal to `eff_from`  | No change.                                                                                                                      |
| One distinct stated date, different            | `eff_from` = stated date; class `in_force,true`; `status_basis = derived_from_text`; gap `effective_date_conflict` (acceptable) |
| Several distinct stated dates for the document | No change; gap `effective_date_conflict_unresolved`                                                                             |
| No match, no tree, or the date fails to parse  | No change.                                                                                                                      |

A stated date earlier than the issue date is not accepted and records
`effective_date_conflict_unresolved`. `raw_eff_from`, `status_code` and
`status_name` keep the source values. The gap detail holds the source date, the
stated date, the provision anchor and the artifact hash of the extract bundle
read. Rule 2 never changes `eff_to` and never touches another document:
`101/2012/NĐ-CP` keeps its source status, and its `replaces` relation from
`52/2024/NĐ-CP` is returned as the existing warning. Reason: the enacting clause
is the legal source of the date and the portal metadata is a transcription; a
single unambiguous statement outranks it, anything less does not. Fail closed on
every other shape.

Rule 2 runs after Rule 1 and before the consolidated derivation, so a corrected
document can count as a current target.

## Status code map

`TNHL` ("Ngưng hiệu lực") maps to `suspended,false`, not `not_yet`. The change
is in `sources/vn.yaml` and takes effect in a new run, because the map is part
of the frozen source list.

## Contracts changed

- [Corpus schema](corpus-schema.md): `status_basis` accepts `source_status`,
  `derived_from_consolidated`, `derived_from_dates`, `derived_from_text`. This
  is part of the next schema version; readers reject older versions as today.
- Build recipe: `normalizer_version` changes, so the build recipe hash changes
  and a frozen run is never rebuilt under the new rules.
- Retrieval: no eligibility change. `Validity.StatusBasis` carries the new
  values and the hit returns the derivation gap, as for consolidated derivation.
- Evaluation: report counts of documents by `status_basis`, and list every
  `effective_date_conflict` with both dates, so the owner can spot-check them
  before a release.

## Bounds and checks

Both rules read only the run's verified artifacts and resolved relations; no
network. Rule 2 scans at most the matching articles of each candidate document.
Tests cover each table row, a clause-level sentence that must not match, a `trừ`
exception, a spelled and a numeric date, a stated date before the issue date, a
replaced document under Rule 1, and that a source `expired` status is never
overridden. Expected effect on `vn/20261110_2`: `52/2024/NĐ-CP` and
`134/2025/QH15` become eligible; measure the count of derived documents and
review every conflict before accepting the new baseline.
