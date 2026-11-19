# Period cases

Decision: [ADR 0010](../adr/0010-retrieval-evaluation-for-agents.md), items 1
and 4. This note owns how development cases about a past period are scored. How
the gate compares a revised development fixture is in
[evaluation fixtures](evaluation-fixtures.md#revisions); everything else about
evaluation is in [evaluation](evaluation.md).

## Why

Run `vn/20261118_1` carried 213 reviewed whole-repeal corrections. One of them
ends 32/2012/TT-NHNN on 2013-03-26, when 08/2013/TT-NHNN Điều 4 khoản 1 replaced
it. In `vn/20261117_7` that circular was wrongly stored as current, and
development case dev-033, which asks for the deposit rate caps of 2011 to 2014,
credited it at rank 2. With the correction its first correct hit moved to rank 7
in the non-current tail, and the case was flagged `low_confidence` with its hits
kept. Development recall@20 fell from 86 to 85 of 113 and MRR@8 from 0.6673 to
0.6595: dev-033 from rank 2 to 7 (0.00714) and dev-089 from rank 5 to 6
(0.00067). The gate held the release, and the correction was dropped to ship the
other 212 as `vn/20261118_2`.

Every development case was scored with current-law eligibility. A case about a
past period expects documents that were in force only in the past, so it can be
credited only by a document wrongly stored as current or from the non-current
tail, and each correct validity fix on its documents shows up as a loss. Gating
such cases rewards wrong validity, while current-law precision is a hard gate.

## Tags

- `historical`: the question asks about a past period, or expects documents that
  were in force only in the past: dev-018, dev-022, dev-033, dev-039, dev-052,
  dev-070 and dev-088.
- `transition`: the question asks when an instrument takes effect and what it
  replaced or what replaced it, and expects documents in force today: dev-023,
  dev-055, dev-076 and dev-093. These four carried `historical` before.
- dev-032 and dev-073 lose `historical`. dev-032 names no period and its
  documents are in force. dev-073's documents are stored current; whether the
  Covid-19 measures have ended is a validity question for corrections, not a
  tag.

On `vn/20261118_2` (exact and ann, hybrid) the seven period cases match 6 of 21
citations at 20, with MRR@8 0.153; the other 43 positive cases match 80 of 92,
with 0.750.

## The period cohort

Cases tagged `historical` leave the gated `development` records and form their
own cohort, `development_period`, with its own case records.

- **Request:** the agent's period request, `in_force_only:false` with `as_of`
  unset, the request the research override `historical_policy: agent` sends
  today. One `as_of` date cannot cover a period: dev-033 spans six successive
  circulars. On `vn/20261118_2` the seven cases match 13 of 21 at 20 under this
  request, with MRR@8 0.206; a non-current tail of 2 adds nothing. ADR 0010's
  History of 2026-11-12 rejected this request as a recipe change under the
  earlier leftover-slot rule; here it only scores this cohort.
- **Records:** recall@20, recall@8 and MRR@8, in modes `exact` and `ann`, hybrid
  arms, with a per-case diff against the previous report's period cohort.
- **Status:** `reported`. No record of this cohort is a gate, a floor or a
  target. Current-law precision does not apply, because the request admits
  non-current documents by design. Abstention is reported, not gated.
- **Calibration:** unchanged. It still fits every development case, period cases
  included, under the default request, so the abstention threshold does not move
  with the split.
- **Membership:** a case moves into `development_period` only with an ADR 0010
  History line naming it and the reason. It leaves the gate for good.
- **Later gating:** after two releases with a period cohort, ADR 0010's History
  records whether and how it is gated, from how validity fixes moved it.

The `development` cohort keeps every other rule: current-law precision and the
rejection checks are hard gates, recall@20 does not fall, and MRR@8 follows the
[MRR@8 tolerance](evaluation-mrr-tolerance.md). The research override
`historical_policy` has no case left to act on in the gated records and is
removed.

## No excused losses

A loss in a gated record is never excused by the reason a document left
current-law results. When a reviewed correction ends a document that a
`development` case expects, the case's expectation is wrong for current law: the
change that adds the correction also revises the case in the fixture, under
[revisions](evaluation-fixtures.md#revisions). The revision is reviewed in Git
like the correction, and the release lists the case as revised.

## Adoption

The release that adopts this note is a development fixture revision: six cases
change tags only, and the seven period cases change cohort. ADR 0010's History
of 2026-11-19 names them. Compared under the revision rule, `vn/20261118_1`
would have passed. On the 43 positive gated cases recall@20 stays at 80 of 92.
MRR@8 falls from 0.750997 to 0.750221, with dev-089 the only fall. In exact mode
`high_water` is 0.754330, carrying the 0.003333 of tolerance already used, and
the value stays above 0.749330.

## Versions

The report version becomes 3: it adds the period cohort and the revision fields.
The scorer version moves to `agent-eval/6`, listed as comparable with
`agent-eval/5` because product matching and the metric definitions are
unchanged; only the cases in the `development` records change, which the
revision rule handles. `corpus start` accepts a previous report of version 2 or
3, and the validators list version 3.
