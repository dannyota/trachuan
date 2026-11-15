# Accepted rank losses

Decision: [ADR 0010](../adr/0010-retrieval-evaluation-for-agents.md), item 4.
This note owns the one way a release may ship when MRR@8 falls against the
previous release. Everything else about evaluation is in
[evaluation](evaluation.md).

## Why

A corpus-only release changes text: a document becomes complete, a new document
arrives. Ranks move with it. On `vn/20261115_2` recall@20 rose from 0.9247 to
0.9570 and four cases were gained, yet one case fell from rank 4 to 9 behind
paragraphs of a document that had just become complete, and MRR@8 fell by 0.002.
The strict rule would hold every release that adds text. A research override is
not a rule. So a fall caused by a named corpus change may be accepted, case by
case, as a reviewed decision.

## When it applies

Only when the dense, chunk, lexical and retrieval recipes of the release equal
those of the previous release. A recipe or model change keeps the strict rule of
ADR 0010. Recall@20, the floors, current-law precision and the rejection checks
are never adjusted.

## The decision file

`sources/<cc>-rank-losses.yaml`, in Git. Its SHA-256 enters the evaluation input
hash, so a changed file means a new evaluation. An entry:

| Field          | Meaning                                                                                  |
| -------------- | ---------------------------------------------------------------------------------------- |
| `id`           | unique in the file                                                                       |
| `case`         | case id and cohort                                                                       |
| `previous`     | the previous release's `rel` and the SHA-256 of its report                               |
| `losses`       | per scoring mode and limit: the previous rank and the new rank of the expected citation  |
| `outranked_by` | the documents now ranked above it that were not before: key and current `extract_sha256` |
| `cause`        | `text_changed` (the document's text or text source changed) or `document_added`          |
| `reason`       | at most 200 characters, no question text                                                 |
| `follow_up`    | the backlog line or finding that tracks the root cause                                   |
| `reviewed`     | date and reviewer; a person, not a model                                                 |

## How evaluation applies it

For each entry, in the evaluation that produces the report:

1. `previous` equals the report given as `--previous`.
2. For every mode and limit listed, the case's previous and new ranks in
   `previous_diff` equal the entry's.
3. Every document in `outranked_by` is ranked above the expected citation in
   this result, is not above it in the previous report's case record, and has
   the stated `extract_sha256` in this release.

An entry that fails any check is not applied and is listed as `unmatched`; the
strict comparison then stands for that case. For an applied entry, the previous
release's reciprocal rank of that case and mode is replaced by the new one
before the aggregate comparison, so the accepted case contributes no fall and
nothing else changes.

The `non_regression` record of MRR@8 states the unadjusted and the adjusted
values, the applied entries and the file's SHA-256. Its status is `pass` only
when the adjusted value does not fall.

## Limits

- At most two applied entries per cohort in one release.
- The unadjusted MRR@8 may not fall by more than 0.01 on any cohort and mode.
- No entry may cover a case whose expected citation is no longer returned within
  20 hits; that is a recall loss and stays strict.

Past either limit the release is held and the cause is fixed.

## Release

The signing validator ([corpus release](corpus-release.md)) refuses a report
with applied entries unless the decision file at the workflow's commit has the
SHA-256 the report names, and unless every limit above holds when it recomputes
them from the report. The host reads `quality_pass` as before.
