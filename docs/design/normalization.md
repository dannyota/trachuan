# Build normalization

This note owns how the build normalizes dates, numbers and source status, and
the consolidated status derivation. [Build](index.md) owns the stage;
[validity corrections](validity-status.md) owns corrections applied after this
layer.

The single date normalizer accepts trimmed `YYYY-MM-DD`, `DD/MM/YYYY`,
offset-free `YYYY-MM-DDTHH:MM:SS` at any valid time of day, or RFC3339
timestamps with an explicit offset. Date-only and offset-free values retain
their civil date; timestamps with an offset convert to Asia/Ho_Chi_Minh before
taking the date. Reject invalid calendar dates, year zero, invalid times and
other formats. `normalizer_version` is `corpus-normalizer/3`; a date or number
normalization change bumps it in the build recipe. Empty becomes null; invalid
values become null plus `invalid_date` naming the field. Raw strings remain
stored. If the parsed end precedes the start, clear both effective dates,
preserve raw values and add `invalid_validity_interval`.

`eff_to` is the first excluded civil date, so eligibility is
`eff_from <= date AND (eff_to IS NULL OR date < eff_to)`. A same-day interval is
empty, not automatically extended. Missing `eff_from` stays eligible for the
date test with `missing_effective_date`. No issue-date fallback for legal
validity; issue dates serve prefixes and consolidation ordering only. Source
boundary semantics need the primary's official-source check before a release;
this convention is explicitly reported as source-date interpretation, not
independently adjudicated legal effect.

Evidence supplied by the primary: live run `vn/20261109`, database
`corpus_vn_20261109`, read 2026-11-10. Offset-free `effFrom` times included T17
(319), T07 (243) and T01 (31); `effTo` included T17 (178), and `issueDate` T01
(31). T07 effective dates equal issue dates in 242/243 documents. Of T17 starts,
12.5% fall on a month's first day and 1.6% on its last; T17 ends have 41
first-day and 4 last-day values. These distributions support retaining the date
part, not advancing T17 to the next day. T01 texts state the same civil date,
including 446-TC/LB, 01-01-1971. In 770/865 T00 replaces/repeals pairs, the old
end equals the new start, supporting the exclusive-end interpretation. These are
supplied observations, not an independent live-source verification.

Trim and uppercase status code; look up its `class` and `current` in the frozen
`sources/vn.yaml` map. Reject an inconsistent map at startup. Before
[validity corrections](validity-status.md), unmapped/empty codes become
`unknown,false` plus `unknown_status`, except for the consolidated derivation
below. Never infer status from a translated name or title. Preserve raw status
code and name. Status and dated eligibility remain separate; a contradiction
produces a warning.

After loading resolved `consolidates` relations, derive status only for a
consolidated document whose source supplies no status. Never replace a supplied
status, even if unrecognized. Apply the first matching branch in this release:

| Condition                                                                         | `status_class,is_current` | `status_basis`              | Derivation gap                   |
| --------------------------------------------------------------------------------- | ------------------------- | --------------------------- | -------------------------------- |
| Source status present                                                             | Source mapping            | `source_status`             | None                             |
| Unordered competition below                                                       | `unknown,false`           | `derived_from_consolidated` | `consolidated_status_unordered`  |
| No resolved target                                                                | `unknown,false`           | `derived_from_consolidated` | `consolidated_status_unresolved` |
| A current target and no newer consolidation of any shared target                  | `in_force,true`           | `derived_from_consolidated` | None                             |
| Resolved targets, but none current, or a newer consolidation of any shared target | `expired,false`           | `derived_from_consolidated` | None                             |

Ordering fails closed symmetrically. Any pair of consolidated documents sharing
a target is unordered if either lacks a normalized issue date or both dates are
equal. Every document participating in such a pair takes the unordered branch
when deriving status, even if it also has an ordered competitor. This check
precedes and overrides both current and expired derivation. With no competitor,
a missing issue date alone does not prevent derivation. A strictly later issue
date identifies a newer consolidation; one for any shared target makes the older
document non-current even if another target is current.

A target counts as current only through its own source-supplied `in_force` or
`partial` status. A status-less consolidated target never counts as current;
derivation never recurses, including through cycles. The table lists every
derivation gap; preserve other date, relation and provenance gaps and raw-source
evidence. Derivation does not establish provision equivalence or alter
historical date filtering.

Number normalization follows
[data](data.md#official-number-normalization-proposed-extension). Source URLs
are copied only when present and verified as HTTP(S); otherwise use null plus a
provenance gap. Do not make up a URL from an opaque source id.
