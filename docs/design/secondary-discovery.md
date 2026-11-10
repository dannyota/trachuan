# Secondary discovery contracts

Proposed refinement of
[source scope](sources.md#secondary-discovery-and-hand-over). This note owns
chinhphu issuer classification and discover work. It does not change
cross-source precedence, membership loss rules or budget values.

## Central issuer evidence

Classify only the detail table's `Cơ quan ban hành` value. Preserve its raw
text, normalized text and redacted detail response reference. Apply the issuer
normalization in [data](data.md#identity-comparison), without abbreviation
expansion, accent folding, punctuation changes or inferred aliases. The host,
title, number suffix, signer and search issuer filter cannot prove jurisdiction.

Proposed source-list fields in `sources/vn.yaml`:

```yaml
chinhphu:
  issuer_policy_version: 1
  central_issuer_forms:
    - Chính phủ
    - Thủ tướng Chính phủ
    - Quốc hội
    - Bộ Khoa học và Công nghệ
    - Ngân hàng Nhà nước Việt Nam
```

These are whole-field forms, not substrings or canonical issuer ids. Normalize
each configured form with the same function. Reject empty forms and duplicate
normalized forms. Require these fields when secondary discovery is configured;
an absent list or unsupported policy version fails planning. Freeze and hash the
list with the source snapshot. Bind the classifier and normalization versions in
the discover recipe. A changed form requires a new run.

Client prerequisite: the chinhphu detail parser must expose every occurrence of
each field, in source order, losslessly. The current parser keeps the last
value; the secondary discovery implementation must change that client contract.
Issuer classification and official-number extraction consume the full occurrence
lists under the repeated-field rule below, using each field's normalization in
[data](data.md). Preserve structural ambiguity instead of selecting one value.

Apply these rules in order:

1. Missing, empty or structurally ambiguous issuer fields are `unknown`.
   Repeated fields with identical normalized values count as one; different
   values are ambiguous, never last-value-wins.
2. Exact equality with a configured whole-field form is `central`.
3. Otherwise, a normalized field beginning with an optional `CHỦ TỊCH` and a
   space, followed by `UBND`, `ỦY BAN NHÂN DÂN`, `HĐND` or `HỘI ĐỒNG NHÂN DÂN`,
   then `TỈNH` or `THÀNH PHỐ` and a nonempty locality is `provincial`.
4. Every other value is `unknown`.

Exclude provincial results with their classification evidence. Unknown results
produce `invalid_source_data` and cannot gain new membership. Do not infer that
an unrecognized issuer is central because no provincial pattern matched. These
patterns only establish exclusions; they are not a complete jurisdiction list.

Several spellings of one body require separately reviewed whole-field entries.
For example, `Ngân hàng Nhà nước` is unknown under the initial list. A combined
issuer value is also unknown unless its entire displayed form is explicitly
listed after verifying every issuer is central. Never split on commas, hyphens,
slashes or `và`: those characters can belong to one body's name. Separate
conflicting fields remain unknown even if each value appears in the allowlist.
Classification does not alias issuers for cross-source identity comparison.

Reason: a small source-list allowlist is auditable and travels with each run;
code-only lists hide scope changes in adapter releases. Prefix-based central
recognition would admit unreviewed bodies and mixed issuers. The trade-off is
missed central forms until reviewed, surfaced as gaps rather than silent loss.

The primary approved the initial forms on 2026-11-10 under the owner's
delegation of technical decisions; the owner can amend the list. They cover the
primary's
[recorded AI sample](../research/vn-scope-comparison-2026-11.md#ai-documents),
not every central body. First-party detail pages show
[`Chính phủ`](https://vanban.chinhphu.vn/?pageid=27160&docid=218029) and
[`Chủ tịch UBND tỉnh Phú Thọ`](https://vanban.chinhphu.vn/?pageid=27160&docid=215941);
both were rechecked on 2026-11-10 and have recorded detail fixtures. The
recorded `home.html` issuer options include alternate and combined forms; their
presence in a filter is not admission evidence. Search behavior remains
[measured evidence](../research/vn-scope-comparison-2026-11.md#vanbanchinhphuvn),
not a published API guarantee.

## Discover items

Plan one compound item per distinct frozen keyword or named entry:

| Input                    | Item key                     | Wire search                             |
| ------------------------ | ---------------------------- | --------------------------------------- |
| `chinhphu.keywords`      | `chinhphu:keyword:<keyword>` | Keyword text                            |
| `chinhphu.named_numbers` | `chinhphu:named:<entry>`     | Number only, without its date qualifier |

Keys use the exact configured strings; deduplicate identical entries and sort
keys bytewise. Named entries use `number` or `number@YYYY-MM-DD`, with the
number/date requirements in [data](data.md#named-entry-eligibility). A wire
query identity hashes the full wire definition, including source, keyword, no
issuer filter and the search cap. Keep the named qualifier and local eligibility
rules in the frozen item definition, not in the wire query. Two named entries
can share a wire query identity while retaining different item keys and
outcomes.

Each item owns search passes, candidate details, candidate-specific VBPL probes
and their gaps. Its immutable output contains only that item's own observations,
never predecessor support. There are no dynamically added discover items or
network calls at stage close. Detail and probe observations may repeat across
items; membership deduplicates docids and preserves every independent query's
evidence. Do not share an earlier run's detail or lookup as fresh evidence.

For each item:

1. Search with all pages and no issuer filter. Apply
   [result completeness](source-execution.md#lookup-and-search-results) and the
   [stable-pass rule](pipeline.md#source-ingest-rules) to the raw returned id
   sets before filtering. Keep every pass; unstable results use their union and
   retain `listing_unstable`. Local filters never repair a capped search.
2. For keywords, every observed docid is a candidate; do not apply the VBPL
   local-title matcher. For named entries, discard only listing rows with a
   known unequal normalized number. Keep missing or uncertain numbers for detail
   resolution. Use the last stable pass, or the union when unstable, and
   deduplicate candidate docids within that selected set.
3. Read fresh detail for each candidate. Classify its issuer above and require
   an intact official number under
   [data](data.md#official-number-normalization-proposed-extension). Missing or
   contradictory number evidence is `invalid_source_data`, not an inferred
   match. Missing detail or failed reads retain their source gaps.
4. A named candidate must have exact normalized-number equality in detail and
   satisfy the linked number/date eligibility rules. A valid unequal number or
   different verified qualified date is ineligible. Missing or unparsable dates,
   unknown issuers and unresolved detail reads prevent uniqueness. Provincial
   candidates are ineligible. Keep rejected and unresolved evidence.
5. For every remaining central candidate, make a fresh exact VBPL lookup using
   its detail number. Read all pages and apply the same stable-pass rule, then
   [identity comparison](data.md#identity-comparison) and the existing
   [hand-over table](sources.md#secondary-discovery-and-hand-over). Equal
   numbers on two docids do not merge their identities or justify skipping a
   candidate-specific comparison. Partial VBPL results cannot prove absence.

Thus a named search for `367/QĐ-TTg` never admits `1367/QĐ-TTg`. Search hits are
only candidates; detail and local equality determine eligibility. A complete
stable named search admits through that entry only with one eligible docid and
no unresolved candidate. Zero gives `named_document_missing`; several or any
unresolved candidate gives `named_document_ambiguous`. Incomplete searches keep
their source gap and cannot establish zero or one. An independent keyword or
other named support can still admit an observed candidate. A qualifying keyword
candidate from a partial search can be added, but the search gap remains.

## Candidate ceiling and close

Count the run-wide union of docids that reach step 3, including subsequently
provincial, invalid, missing and VBPL-replaced candidates. A docid found by
several queries or passes counts once. Known unequal named-number substring hits
remain search evidence and do not consume candidate slots. The ceiling's value
lives in [budgets](budgets.md#pipeline), before VBPL exclusion.

An item whose own candidate union exceeds the ceiling stops before detail work
and completes its immutable observation output with the candidate ids and
overflow evidence, so discovery close can record the durable failure. At
discover close, union every item's recorded candidate ids and check the run-wide
ceiling before publishing any seeds snapshot. Overflow returns a durable run
failure with code `secondary_candidate_ceiling`, a reason containing the
observed count and ceiling, and evidence references to each item's candidate
record. Preserve search evidence; do not select the first candidates or advance
to expand or fetch. Like `scope_changed`, this failure is non-acceptable: start,
gap acceptance and release refuse the run. Only a new run with a reviewed source
list or limit clears it; never raise the ceiling or narrow a frozen query.

This bounds admitted inventory, not aggregate detail requests: compound query
items can repeat work before close detects their combined overflow. Request,
byte and item-deadline limits still apply. Reason: fixed query items fit the
existing planner and retry model without a dynamic candidate queue or new shared
reservation protocol. Measure repeated work before release; never extend an item
deadline automatically to finish its candidate loop.

## Support and retention evidence

Every admitted secondary candidate and every proven VBPL seed from its probe
records `secondary_probe` under the
[typed tuple contract](data.md#scope-workshop-contracts). Set `rule_value` to
`<chinhphu item key>|<docid>`, using the candidate's decimal chinhphu docid, for
example `chinhphu:named:367/QĐ-TTg@2026-02-28|218029`. Use this value for both
the secondary key and a VBPL seed proven by that probe. Produce the tuple at
`discover` / the compound item key. Each configured query supplies independent
support: supersession applies per query and docid, so fresh support from one
query never supersedes another query's tuple. Retention applies per tuple under
[membership](membership.md), without lineage metadata.

The tuple evidence's `query_identity` is the hash of the frozen item definition:
source, item key, kind, keyword or number, date qualifier and cap. It identifies
configured support, so changing a date qualifier removes the old entry's support
even when the wire search is unchanged. The wire query identity stays inside the
item output for search completeness and absence comparisons.

The evidence reference points to a candidate record in that item's hashed
output. It binds the following, with artifact-relative paths and response
hashes:

- Search item definition, wire query identity, passes, pages containing the
  docid, completeness and stable outcome, including named-entry filtering
  outcomes.
- Detail URL and response, raw identity fields, issuer classification, matching
  form and policy version, and source-list hash.
- The candidate-specific VBPL query, all pass/page references, completeness,
  stable outcome and each identity comparison, including complete zero results.

Absent probe evidence is never a complete zero. Candidate records keep explicit
states for not attempted, incomplete, complete and ambiguous comparisons.
Conflicting candidate details across items stay unresolved with
`invalid_source_data`; do not choose the latest response or union conflicting
fields into a verified identity.

[Membership](membership.md) owns supersession and removal. For secondary keys,
use its other-query rule against item definition identities in the evidence; for
VBPL seeds, use its probe-lookup rule. A configured supporting query whose
candidate detail, issuer classification or VBPL probe is unresolved is
incomplete for that candidate, even if search pagination completed. A removed
query has no current observation. A complete stable search that omits the docid
remains a complete absence observation; it needs no detail or probe for that id.

## Adoption and acceptance

Independently review these contracts before the secondary discover
implementation. Add source-list validation and bump the discover recipe in a new
run. No frozen snapshot, earlier artifact or released membership is rewritten.
Deliberate exclusions still need the existing
[loss approval](sources.md#local-terms-and-loss-gate). ADR 0012's source choice
and precedence remain unchanged.

The primary must check alternate and combined issuers, repeated conflicting
fields, provincial exclusion, unknown-issuer gaps, substring rejection,
qualified dates, capped/unstable searches, overlapping queries, repeated docids,
ceiling overflow across items, incomplete probes and support supersession with
one incomplete query. Use recorded, token-redacted fixtures and reduced limits;
these are required checks, not reported results.

Keep [source safety](sources.md#fetch-and-safety) and source-pause checks for
every compound item's source dependencies. No new service or paid component is
needed. Duplicate detail/probe requests and false-negative issuer forms are the
main costs of this bounded first version; broader forms or a shared candidate
queue need a later reviewed change.
