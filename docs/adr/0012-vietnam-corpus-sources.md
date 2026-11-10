# ADR 0012: Vietnam primary and secondary legal sources

Status: accepted source choice 2026-11-09; execution design pending review.

## Context

The [source research](../research/vn-sources-2026-11.md) and primary's measured
number lookups distinguish discovery omissions from source omissions. Four
missing golden documents are in VBPL; recent central AI documents need another
source. The owner selected this source policy on 2026-11-09.

First-party entry points are [VBPL](https://vbpl.vn/),
[Government documents](https://vanban.chinhphu.vn/) and
[the national law portal](https://phapluat.gov.vn/). Measurements are supplied
evidence; this design did not recheck the sites or assume a documented API.

## Decision

Keep VBPL as the only primary source. Widen discovery through explicit named
numbers, local scope selection over the full central listing and one-hop
relation context. Use `vanban.chinhphu.vn`, source key `chinhphu`, only for
central documents absent from VBPL, starting with AI. VBPL takes precedence only
when number, issuer and issue date establish the same identity. Equal numbers
with different verified issue dates are different documents; missing or
conflicting evidence otherwise retains ambiguity.
[Sources](../design/sources.md) owns the hand-over decision table.

Reuse banhmi data and source knowledge under
[ADR 0001](0001-reuse-banhmi-data-only.md), never its scraper code. Keep the
single workflow in [ADR 0008](0008-write-path-temporal-self-hosted.md).

## Consequences

Agents gain referenced laws and recent central metadata without duplicate
primary sources. One-hop expansion increases downloads and indexing volume;
source limits and measurements gate adoption. A second HTML adapter needs
recorded fixtures because no JSON API was found. Signed PDFs remain
metadata-only until separate extraction work; metadata presence is not usable
provision coverage. Source hand-over preserves old releases and records loss of
the secondary key rather than claiming identical provenance.

## Alternatives rejected

- VBPL with unchanged discovery: leaves known available golden documents out.
- VBPL alone after widening: still omits recent central AI documents.
- Recursive relation crawling: unbounded scope and cost beyond the owner choice.
- Equal primary status for Government documents: duplicate evidence and unclear
  precedence when VBPL catches up.
- [Công báo](https://congbao.chinhphu.vn/): measured search missed known
  documents; its needed missing document was available on Government documents.
- State Bank Region 1 and [State Bank](https://sbv.gov.vn/) portals: golden
  circulars are in VBPL; extra administrative decisions need a demonstrated case
  before a separate scope decision.
- [phapluat.gov.vn](https://phapluat.gov.vn/) as a separate source: measured ids
  identify the same VBPL database, so it adds another adapter, not coverage.

## History

- 2026-11-09: recorded the owner's source choice from the corpus scope brief;
  execution details remain a proposal for independent review and owner approval
  of losses, lists and limits.
- 2026-11-09: clarified precedence under the primary's review rulings: require
  matching identity evidence before replacement, because a number alone can
  collide. Execution now specifies shared contracts, durable pauses and a
  non-acceptable expansion-drift failure; reviewer confirmation remains pending.
- 2026-11-09: recorded the owner's full central listing and local selection
  choice, approved lists and carried-over migration loss approval; applied the
  primary's date-qualified identity and capped chinhphu completeness rulings.
- 2026-11-10: clarified the execution contracts and preserved the two-listing
  withdrawal exception. Kept drift fatal after incomplete expansion; selected
  [one-run removal accounting](../design/membership.md#removal-lifetime) so
  historical losses stay auditable without recurring acceptance. The changed
  lifetime requires independent review and new run recipes before adoption; the
  accepted source choice is unchanged.
- 2026-11-10: applied the owner's review rulings: acceptance ends removal carry;
  one recorded predecessor binds membership reconciliation. Clarified probe
  seeds, legacy support migration and fetch freshness. Reviewer confirmation
  remains pending.
