# Retrieval validity

How retrieval decides eligibility and the status it serves; part of the
[shared retrieval recipe](retrieval-recipe.md).

## Validity and evidence

Documents of class `relation_context` are not candidates in either arm unless a
validated number reference in the query equals the document's normalized number;
then they compete like any other document. They stay in the corpus as resolved
relation targets with metadata, text and validity. The recipe field is
`context_scope:"number-routed/1"`. Reason: on run `vn/20261110_4` they took 82
of 736 legacy hit slots at limit 8, matched none of the 96 expected citations,
and three out-of-scope negatives found a context document as their best hit,
which raised the abstention threshold from 0.4812 to 0.5595 and made 14 answered
positives abstain. One-hop context covers a topic by accident, so serving it as
primary evidence misleads on topics the scope never claimed.

Without explicit `as_of`, primary eligibility requires stored `is_current` and
the known date interval containing the caller's civil date in Vietnam. The
engine clock is injected; eval freezes that date. Missing start dates pass the
date test with a gap. Unknown status is not current. A document with
`validity_conflict` is not eligible in default mode and appears only in the
non-current group, marked unresolved; with an explicit `as_of` the dated
determination ignores the flag and the hit still carries it. Partial status is
current at document level only, with targeted repeal relations as warnings.

Set `Validity.Current` to that status/date eligibility result in this default
mode, including eligible missing-start cases with their gap. With an explicit
historical date, use the dated determination below instead.

### Served status

The stored `status_class` never changes at query time, but every response gives
the class at the reference date: today in Asia/Ho_Chi_Minh, or `as_of`. A stored
`in_force` or `partial` is served as `expired` from its `eff_to` (exclusive end)
and as `not_yet` before its `eff_from`; any other class is served as stored.
Search hits, `read`, document status, public summaries and taxonomy lists use
one rule, and a taxonomy list's `is_current` equals `Validity.Current` for that
date. Response caches either key on the Vietnam date (MCP search and taxonomy,
the public summary) or expire within minutes: a topic page can show the previous
day's class for up to about 20 minutes after midnight (Nitro 540 s plus a
browser max-age of 600 s). Eligibility, ranking and the validity policy version
do not change.

Copy the document's `status_basis` into `Validity.StatusBasis` in both modes,
including `derived_from_consolidated` when [build](normalization.md) derived its
status. `Basis` continues to distinguish default status/date filtering from
explicit historical date filtering; `StatusBasis` separately exposes the origin
of the status. Return derivation gaps with the hit. Derived status is not
independently adjudicated legal effect.

With `as_of`, primary eligibility uses document dates only, independent of the
latest source status. Missing start dates remain in primary results with a gap,
but `Validity.Current` is false when the date test is indeterminate. A known
start and no known end is an open interval, not a missing-end gap. Every such
response says historical filtering uses document dates, not reconstructed
provision history or past suspension status.

`in_force_only` defaults true. In that default mode the complement (expired and
not-yet documents) still runs the same arms as a separate group with its own
depths and shaping, and the response reserves `non_current_tail` slots (2 in
recipe 2, [budgets](budgets.md#retrieval)) for it: primary hits are shaped to
`limit` minus the effective tail, then up to that many complement hits follow in
`NonCurrent`, marked non-current with their status; when the complement has
fewer hits, primary keeps the freed slots. The effective tail is the smaller of
`non_current_tail` and half of `limit` rounded down, so primary always keeps at
least half of the slots (limit 1: no tail; limit 2 or 3: one; limit 4 and up:
two). In both modes a slot a group cannot fill from its shaped hits is filled
from that group's wider ranking first, then by the other group; the complement
never exceeds its reserved count in default mode. Abstention and confidence are
judged on the primary group alone, as before; complement hits never make a
response answer. Not-yet documents never enter the primary group, since
current-law precision counts every primary hit. With `in_force_only` false the
caller asked for non-current law, so the response reserves half of `limit`
(rounded down) for the complement: primary is shaped to the other half, the
complement fills its half in score order, and either group's unused slots return
to the other. Measured on run vn/20261111_4, the earlier rule (complement only
in leftover slots) changed nothing because primary always filled the limit.
Primary failures are not rescued by the complement: no primary hits means
abstain. Evidence size includes both groups. `as_of` never silently swaps
expired and primary law based on score. Measured on run vn/20261111_4 (cap 5
with tail 0, 2, 3, 4): old product recall@20 82, 85, 85, 86 and recall@8 77, 77,
75, 73; development recall@20 75, 83, 85, 85 and recall@8 73, 76, 72, 67;
against cap 5 alone, tail 2 raises recall@20 on both cohorts and recall@8 and
MRR@8 on development, and leaves old product recall@8 and MRR@8 unchanged;
against the cap-3 baseline every one of those numbers rises.

Build `Citation` from the group's actual ancestry: join nonempty article, clause
and point labels in ancestor order with a comma and a space; otherwise use its
own label. The number is a separate field. Never derive a citation from an
anchor suffix or infer absent legal levels.

Hit text is the substantive lead spans of the group's ancestors that the chunk
recipe carried (role `lead`, ancestor order), followed by the group's own
representation and identified descendants, using
[chunk rendering](chunk.md#tree-rendering), never by joining prefixes or
guessing text from lexical tokens. Return the full text when within the evidence
cap. Above it, keep the leads first, then the best matched member's body spans
up to the cap, with exact offsets and `Complete:false`, `NextPart:1` for a
document read. A cut or missing lead makes the hit incomplete, so a clause never
looks complete without the article condition that qualifies it. Mark any cut
span incomplete. Each fragment carries `Complete`, true only when it holds every
covered source interval of its identity; child credit requires a complete child
fragment, not a coincidentally repeated sentence or shared label. Fragments
reference slices of `Hit.Text`; they do not send duplicate text. Source/content
aliases follow the chunk span contract. For ambiguous parent containment, return
its own text and an ambiguity gap without child credit. Do not attach a second
full enclosing article to every hit.

Attach incident relations whose provision endpoint is null, the group itself, or
its descendant, and the warning rows ([search results](mcp-results.md#warnings))
whose exact target is an ancestor of the group, so a clause hit, and a
`relations` read of that clause, carries the evidence of an exact warning on its
article. Preserve source direction; sort by kind, source key, target key,
artifact hash and ordinal. Apply the relation count/text caps in budgets, mark
omissions with `relations_omitted` and a document-read pointer, and mark cut
evidence incomplete. `EvidenceSHA256` hashes the full stored UTF-8 evidence;
product relation credit requires complete returned evidence and a mapped edge.
Deduplicate edges by source key, artifact hash and ordinal per hit, not across
different hits; two documents can have byte-identical relation artifacts. The
response is bounded and its actual evidence size is scored.

Abstain on validation rejection, no eligible primary hits, or first-group best
cosine strictly below the stored threshold. Equality passes. Low-confidence
responses retain their hits and gaps, as evidence agents may inspect; metrics
still score those returned hits and the abstention decision separately.
Infrastructure failure returns an error, never a successful abstention.
Threshold provenance and provisional status follow [evaluation](calibration.md).
