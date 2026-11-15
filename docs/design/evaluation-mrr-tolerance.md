# MRR@8 tolerance on corpus-only releases

Decision: [ADR 0010](../adr/0010-retrieval-evaluation-for-agents.md), item 4.
This note owns the one bounded exception to "MRR@8 does not fall against the
previous release". Everything else about evaluation is in
[evaluation](evaluation.md); the number is in [budgets](budgets.md).

## Why

A corpus-only release changes text, and ranks move with it. The measured case is
in ADR 0010's History: recall@20 rose, four cases were gained, one case fell out
of the first eight hits behind a document that had just become complete, and
MRR@8 fell by 0.002. Holding such a release keeps a less complete corpus in
production. The accepted cost to agents is stated plainly: more coverage may
cost a small, bounded loss of early-ranked evidence, and that loss may not
accumulate.

## Rule

It applies only when the dense, chunk, lexical and retrieval recipes of the
release equal those of the previous release. A recipe or model change keeps the
strict rule. Recall@20, the floors, current-law precision and the rejection
checks are unchanged in every case.

For each `non_regression` record of MRR@8 (each mode the report gates today):

- **High-water mark.** `high_water` is the highest MRR@8 of that record along
  the chain of releases with these recipes: the larger of the previous report's
  value and the previous report's `high_water`. A previous report without the
  field, or under other recipes, gives its value alone.
- **Pass** when the value is at least `high_water` minus the tolerance.
- The record states `previous`, `high_water`, `tolerance`, the value, and
  `within_tolerance: true` when the value is below `previous`.

Because the comparison is with the high-water mark, successive releases cannot
each give up the tolerance: the total fall from the best value under these
recipes never exceeds it. A value above the mark raises the mark.

## What a release within tolerance owes

The report already lists the lost cases in `previous_diff`. A release that
passes with `within_tolerance: true` adds, in the same change that records the
release, one backlog line or finding per lost case naming its cause, so the loss
is worked on and not forgotten.

## Report and validators

The tolerance changes what `status: pass` means for MRR@8, so the report version
moves and the evaluation recipe version with it. The uploader and the signing
validator ([corpus release](corpus-release.md)) accept the new version only by
recomputing the rule from the record (recipes equal, `high_water` from the
previous report they are given, the tolerance from the validator's own constant,
the value) and refuse a record whose stated status differs. A report of the
earlier version is judged by the strict rule, as before. The host reads
`quality_pass` as before.
