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
`old_product` and must equal it. Recall@20 is gated separately and is never
relaxed; abstention is not a gate.

## When the tolerance applies

A record is judged with the tolerance only when all of these hold between the
candidate report and the previous report; otherwise the strict rule applies:

1. The dense, chunk, lexical and retrieval recipes are equal.
2. The scorer versions are equal.
3. The cohort has the same case ids, positive count and denominators, the same
   rejection set, and the same modes, arms and limits.
4. For every case of the record whose first-correct reciprocal rank at limit 8
   fell, the identity that was first correct in the previous report (the hit at
   that rank in its case record) is still an accepted identity of the case under
   the candidate's identity map. So a fall caused by a changed expectation is
   never tolerated; the identity map may otherwise differ.

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

The gating report of a release comes from
`corpus eval <cc> <rel> --previous <report>`, which scores an existing, verified
build with the binary's own evaluation code. Re-evaluating an unchanged build
with a newer evaluation version this way is allowed and needs no new run; the
report records the versions it used. The frozen recipes of pipeline stages are
not affected: a changed stage recipe still needs a new run ([index](index.md)).

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
- for every gated record, recompute the conditions of
  [When the tolerance applies](#when-the-tolerance-applies) that the two reports
  can show (1 to 3), recompute `high_water`, the comparison and `losses` from
  the two reports' case records, with the tolerance as their own constant, and
  refuse a record whose stated fields or status differ, a non-finite value, or a
  version 2 record without its fields. Condition 4 is asserted by the evaluator
  from the identity map and recorded per case as `expectation_kept: true`; a
  record with a loss lacking it is judged strictly.

The host reads `quality_pass` as before.
