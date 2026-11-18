# Losses from corrected validity

Decision: [ADR 0010](../adr/0010-retrieval-evaluation-for-agents.md), item 4.
This note owns the one exception to "recall@20 and MRR@8 do not fall against the
previous release" for a loss that better validity data causes. The MRR@8
tolerance is in [MRR@8 tolerance](evaluation-mrr-tolerance.md); everything else
about evaluation is in [evaluation](evaluation.md).

## Why

Run `vn/20261118_1` carried 213 reviewed whole-repeal corrections. One of them
ends 32/2012/TT-NHNN on 2013-03-26, when 08/2013/TT-NHNN Điều 4 khoản 1 replaced
it. In `vn/20261117_7` that circular was wrongly stored as current, and
development case dev-033 (tagged `historical`, scored with current-law
eligibility like every case) credited it at rank 2. With the correction it
correctly leaves current-law results, dev-033 abstains, and development
recall@20 fell from 0.7611 to 0.7522 and MRR@8 from 0.6673 to 0.6595. The gate
held the release, and the correction was dropped to ship the other 212
(`vn/20261118_2`). Current-law precision is a hard gate; a non-regression rule
that rewards a wrongly current document works against it.

## Excused losses

A case's loss in a gated record (recall@20, or MRR@8 at limit 8) is excused when
all of these hold:

1. Every expected citation the case lost at the record's limit was credited in
   the previous report only by hits on documents that are not eligible in the
   candidate release under current-law eligibility at the candidate's evaluation
   date: stored status not current, `eff_to` at or before the date, `eff_from`
   after it, or `validity_conflict`.
2. Each such document's eligibility differs between the two releases. The
   candidate report records, for every excused citation, the document key and
   the candidate's `status_class`, `is_current`, `eff_from`, `eff_to`,
   `validity_conflict` and `status_basis`; the previous report's case record
   supplies the hit that was credited.
3. The case lost nothing else: every other citation it matched in the previous
   report at the limit is still matched, at the same or a better rank.

An excused case leaves the comparison on both sides: the record compares the
candidate with the previous value and `high_water` each lowered by the credit
the excused citations gave in the previous report, recomputed from its case
records. That credit was false in every report that counted it, since the
document was wrongly current there too, so lowering the watermark by it cannot
let a real loss accumulate. Every other loss keeps the rules it has today.

The record lists excused cases in `excused`, separate from `losses`, with the
citation, the document and the fields of item 2. A release with an excused case
owes a backlog line when the case can no longer be matched under current-law
scoring, so its expectation is reviewed.

## What stays

Current-law precision and the rejection checks stay hard gates. Recall@20 is not
relaxed for any other cause. A loss on a document that is still eligible, a loss
beside a gain of an ineligible document, and a loss from a changed expectation
are judged as today.

## Validators

The uploader and the signing validator recompute `excused` from the two reports:
the previous report's case records give the credited hits, and the candidate
report's validity fields give eligibility at its evaluation date. They refuse a
record whose `excused`, adjusted values or status differ from their own result.
Trust boundary: they check that each excused document is ineligible by the
fields the candidate report states, not that those fields equal the release
database; the candidate report and the database come from the same signed build
artifact.

## Versions

The rule changes what `status: pass` means for recall@20 and MRR@8, so the
report version becomes 3 and the scorer version moves. A version 2 previous
report has every case record the rule needs; the candidate must be version 3.

## Open: historical cases

13 of 65 development cases are tagged `historical`. Scored with current-law
eligibility, their expected documents are out of force by construction, so they
can be credited only through a wrongly current document, and each correct
validity fix on them shows up as a loss. This note excuses those losses but does
not decide how such cases should be scored: at a recorded `as_of` inside their
period, outside the gated records, or rewritten as current-law questions. Each
choice changes the development baseline.
