# MRR@8 tolerance on corpus-only releases

Decision: [ADR 0010](../adr/0010-retrieval-evaluation-for-agents.md), item 4.
This note owns the one bounded exception to "MRR@8 does not fall against the
previous release". Everything else about evaluation is in
[evaluation](evaluation.md); the number is in [budgets](budgets.md); the
measured case that led to it is in ADR 0010's History.

## Why

A corpus-only release changes text, and ranks move with it. Holding a release
because one case left the first eight hits keeps a less complete corpus in
production. The accepted cost to agents: more coverage may cost a small, bounded
loss of early-ranked evidence, and that loss may not accumulate. The tolerance
is a policy allowance, not a noise estimate: it bounds the net fall of a record,
not the number of cases that moved.

## The records

The gated MRR@8 records are four: cohorts `old_product` and `development`, each
in modes `exact` and `ann`, hybrid arms, limit 8. The top-level records repeat
`old_product` and must equal it. The `development` records leave out the
[period cohort](evaluation-period-cases.md). Recall@20 is gated separately and
is never relaxed; abstention is not a gate.

## When the tolerance applies

A record is judged with the tolerance only when all of these hold between the
candidate report and the previous report; otherwise the strict rule applies:

1. The dense, chunk and lexical recipes are equal, and the retrieval recipes are
   equal with `abstention.calibration_sha256` left out: that field identifies
   the release's own calibration data, not a recipe choice. The abstention
   threshold and every other field must be equal. Calibration is recomputed from
   each release's data on every build.
2. The scorer versions are equal, or are a pair the evaluator and the validators
   list as compatible because product matching and the metric definitions are
   unchanged between them.
3. The legacy, old-product and development fixture hashes are equal, and the
   cohort has the same case ids, positive count and denominators, the same
   rejection set, and the same modes, arms and limits. Across a revised
   development fixture, a `development` record meets this condition over its
   comparable cases instead ([revisions](evaluation-fixtures.md#revisions)).
4. For every case of the record whose first-correct reciprocal rank at limit 8
   fell, every scoring alternative the previous report records for the case
   (`mapped_identities`, compared by the fields the matcher uses, not by
   provenance) is still a scoring alternative of the case in the candidate
   report. So a fall caused by a removed or changed expectation is never
   tolerated; the identity map may otherwise differ and gain entries. A previous
   report that does not record the alternatives of such a case gives the strict
   rule.

## Rule

- **`high_water`** is the baseline the candidate is compared with: the larger of
  the previous report's value and the previous report's `high_water`. A previous
  report of the earlier version has no such field and gives its value alone. A
  previous report under other recipes gives its value alone too: a watermark
  never crosses a recipe change. The candidate's own value is not part of it.
- **Pass** when the value is at least `high_water` minus the tolerance.
- The record states `previous`, `high_water`, `tolerance`, the value, its
  `status`, `within_tolerance: true` when the value is below `previous`, and
  `losses`: every case whose first-correct reciprocal rank at limit 8 fell, with
  the previous and the new rank.

Because the comparison is with the watermark, successive releases cannot each
give up the tolerance: the total fall from the best value under one recipe set
never exceeds it.

## What a release within tolerance owes

One backlog line or finding per case in `losses`, naming its cause, added in the
change that records the release.

## Versions and evaluation runs

The rule changes what `status: pass` means for MRR@8, so the report version
becomes 2 and the scorer version moves. A tolerated pass is never written in a
version 1 report.

The gating report of a release is its run's `eval` report
([selected report](release-evidence.md#selected-report)). Scoring an unchanged
build with a newer evaluation version through `corpus eval` is a diagnostic; to
gate a release on it takes a new run and `rel`. The report records the versions
it used, and a changed stage recipe needs a new run ([index](index.md)).

## Validators

The uploader and the signing validator ([corpus release](corpus-release.md))
both:

- accept only the report versions and scorer versions they list; version 1 is
  judged by the strict rule; an unknown version is refused, as today;
- obtain the previous report itself, `<cc>/<previous rel>/report.json` in the
  releases bucket, and require its SHA-256 to equal both
  `previous_diff.previous.report_sha256` and the `report.sha256` of the previous
  release's signed manifest; a missing or different file refuses the release
  rather than resetting the watermark;
- for every gated record, recompute the four conditions of
  [When the tolerance applies](#when-the-tolerance-applies) from the two
  reports, recompute `high_water`, the comparison and `losses` from their case
  records, with the tolerance as their own constant, and refuse a record whose
  stated fields or status differ, a non-finite value, or a version 2 record
  without its fields.

The previous signed manifest is verified under the release policy before its
`report.sha256` is used.

The host reads `quality_pass` as before.
