# Vietnam corpus scope

Proposed execution contract for the owner's 2026-11-09 source choice in
[ADR 0012](../adr/0012-vietnam-corpus-sources.md). VBPL remains primary;
`chinhphu` fills central-document omissions, starting with AI. This note owns
selection and identity; [membership](membership.md) owns retention and loss,
[pipeline](pipeline.md) owns execution, [data](data.md) owns stored fields and
[budgets](budgets.md#pipeline) owns limits. Shared interfaces and close
transactions live in [source execution](source-execution.md).

## Evidence and boundaries

Use the primary's
[scope measurements](../research/vn-scope-comparison-2026-11.md) and
[source research](../research/vn-sources-2026-11.md). They supersede the missing
document list in the earlier
[ingest report](../research/vbpl-ingest-2026-11.md). Endpoint behavior below is
measured evidence, not a published API guarantee; fixtures must pin it.

First-party entry points supplied by that evidence:

- [VBPL listing](https://vbpl-bientap-gateway.moj.gov.vn/api/qtdc/public/doc/all)
  accepts `optionDoc: title|docNum`, `matchMode: all_words|exact` and
  `agencyLevel: TRUNG_UONG|DIA_PHUONG`.
- [Government documents](https://vanban.chinhphu.vn/) use an ASP.NET postback
  search and `/?pageid=27160&docid=<n>` detail pages. No JSON API was found.
- [Signed PDF example](https://datafiles.chinhphu.vn/cpp/files/vbpq/2026/4/142-2026-ndcp.signed.pdf)
  establishes an attachment host, not a filename template or verified signature.

## Direct discovery

List the full central catalog once per run through `POST /doc/all`, with
`agencyLevel: TRUNG_UONG`, no agency filter and no keyword. Use the page size in
[budgets](budgets.md#pipeline) and the stable-listing rule in
[pipeline](pipeline.md#source-ingest-rules). Replace server-side title searches
and the separate State Bank sweep with local selection over this listing.

Select a listing item when `agencyIds` intersects `{62, 908}` or its title
matches an approved term below. Normalize title and term with NFC, lowercase and
whitespace collapse, then require a contiguous phrase. Exclude `vbpqta_`
renditions. Record every matching rule using the
[typed support tuple](data.md#scope-workshop-contracts). A direct hit stays
class `primary`, including a qualified named lookup or secondary probe. Freeze
agencies, terms and normalization version in the source list and discover
recipe; changes require a new run.

Reason: local selection retains prior hits and adds relevant central documents,
with some noise. Listing items lack field classification; details often leave it
unset, so it cannot define scope. Diacritic folding and phrase gaps added mostly
noise; exclude both. Named numbers cover title typos.

Add `vbpl.named_numbers` to `sources/vn.yaml`. Initially include `71/2025/QH15`,
`368/2025/NĐ-CP`, `58/2021/NĐ-CP`, `18/2018/TT-NHNN`, `134/2025/QH15`,
`53/2026/TT-NHNN`, `142/2026/NĐ-CP`, `88/2019/NĐ-CP`, `143/2021/NĐ-CP`,
`340/2025/NĐ-CP`, `52/2024/NĐ-CP` and `304/2025/NĐ-CP`. The owner approved these
on 2026-11-09 to preserve coverage when titles omit a subject term.

Discover each number with `optionDoc: docNum`, `matchMode: exact`,
`agencyLevel: TRUNG_UONG`, no agency filter, and all result pages. Compare
normalized returned numbers locally; an API hit alone does not establish
equality. Deduplicate by VBPL id and apply only
[VBPL named-entry eligibility](data.md#named-entry-eligibility) before counting
matches. One eligible match with no unresolved candidate joins the primary
seeds; zero produces `named_document_missing`; several or unresolved candidates
produce `named_document_ambiguous`. Never pick the newest result. Independently
discovered candidates stay expected.

Every listing or lookup returns the
[evidence-bearing result](source-execution.md#lookup-and-search-results). Apply
zero/one/many rules only to complete results. A partial result may add observed
ids, but cannot prove absence, uniqueness or withdrawal.

A zero eligible VBPL match can be covered by one eligible secondary candidate
matching the named entry, including its date qualifier: keep the lookup evidence
and record secondary coverage instead of `named_document_missing`. Never convert
ambiguity to absence. Correct or qualify an ambiguous entry in a later reviewed
source list, never inside a frozen run. The initial contract has no fuzzy title
or inferred-year fallback.

### Local terms and loss gate

Use these owner-approved terms for local title selection:

| Area                    | Terms                                                                                                                   |
| ----------------------- | ----------------------------------------------------------------------------------------------------------------------- |
| Security                | an toàn thông tin; an ninh mạng; dữ liệu cá nhân; bảo vệ dữ liệu cá nhân; an toàn hệ thống thông tin; an ninh thông tin |
| Electronic transactions | giao dịch điện tử; chữ ký số; chữ ký điện tử; định danh điện tử; thương mại điện tử; định danh và xác thực điện tử      |
| AI and technology       | trí tuệ nhân tạo; công nghiệp công nghệ số; cơ chế thử nghiệm có kiểm soát; công nghệ tài chính                         |
| Data and identity       | dữ liệu quan trọng; dữ liệu cốt lõi; luật căn cước; luật dữ liệu; luật viễn thông; thông tin cá nhân                    |
| Banking operations      | hệ thống thông tin ngân hàng; thuê ngoài; kinh doanh liên tục; ngân hàng                                                |
| Payments and assets     | thanh toán không dùng tiền mặt; ví điện tử; ngân hàng số; tiền điện tử; tài sản số; tài sản mã hóa                      |

The owner approved the keyword-exact candidate set's migration loss accounting
on 2026-11-09. Local selection is a measured superset, so that approval carries
over. Freeze the immutable comparison hash recorded by the primary in the scope
research and its approval note with the source snapshot. Retain definitions,
response hashes, ids, numbers, titles, totals and both set differences. A
partial listing cannot pass a loss gate.

The primary accounts for every old-only id: retained by another rule, restored
with a named number or revised term, or excluded with an owner-approved scope
reason. No unexplained loss and no loss of the known VBPL golden documents is
allowed. Include AI-related documents beyond the golden set in this review.
Review later changes only when rules or queries can remove coverage. Compute
local differences offline from a recorded complete stable listing, without new
network comparisons. Bind reviews to changed definitions and old-only ids;
retain prior reviews. Pure additions need no loss review. Never edit a started
run. Reason: review actual losses without repeating unrelated work.

The central listing has one query identity: source, listing type, level, absent
agency and keyword filters, and page size. Other query identities hash their
source-specific definitions, including option, mode and number or keyword, with
canonical ordering. Local selection rules are frozen separately from the listing
identity. Reconcile all independent supports under [membership](membership.md);
selection loss is not catalog absence.

## One-hop relation context

Add `expand` between `discover` and `fetch`, with one item per active primary
VBPL seed. Freeze that seed list before planning expand; never plan from its
output. Each item reads detail and diagram by VBPL id and records both payloads
and hashes. Like discover, expand observes the source afresh each run and learns
its content hash after download; a seed id is not a cache-validity signal. Fetch
requests fresh source responses for every active member. An attachment may come
from the verified attachment cache under the
[pipeline rule](pipeline.md#source-ingest-rules). Removal items (`withdrawn`,
`scope_removed`) are never fetched. Reason: a separate bounded stage fits the
existing batch/checkpoint model and avoids a mutable work queue inside fetch or
recursive relate execution.

Qualify explicit outward targets in the seed's `references.targetDocument` and
`documentNamesByType`, using the existing relation-code table: `legal_basis`,
`implements`, `guided_by`, `amends_supplements`, `replaces`, `repeals`,
`suspends`, `consolidates`, `cites`, `references`. Direction means the seed's
source record points to that target; preserve the kind's meaning, including
`guided_by`. Do not reverse edges or search for inbound documents. Reason: even
citations supply useful legal context, but inferred inbound edges would widen
scope without source evidence.

Only an explicit VBPL id that passes the
[observation contract](data.md#expand-observation-artifact) adds a target.
Number-only records are audit input for later named-list review; they do not add
targets. Unknown codes remain gaps and add no targets. Central or local targets
may qualify by id without opening local title sweeps. Reason: the source's
specific link supplies scope, while guessing a number match or jurisdiction
would hide identity errors.

Union and deduplicate targets as class `relation_context`; a direct seed always
wins class `primary`. Retain every seed, raw code and evidence hash that caused
membership. Context never seeds expansion, even when a previous run called it
primary. Fetch by id and use the normal extract, relate, chunk and embed stages.
Valid provisions are searchable without a special ranking penalty; missing trees
stay metadata-only. Reason: agents need the cited law's text, and a
metadata-only class would preserve the coverage failure.

Retention and removal follow [membership](membership.md#relation-loss).

Fetch checks the frozen inventory under the
[drift rule](membership.md#fetch-drift), including incomplete expansion.

## Secondary discovery and hand-over

Add `chinhphu.keywords` and `chinhphu.named_numbers` to the source list, frozen
like VBPL inputs. Start keywords with `trí tuệ nhân tạo`; start named numbers
with `142/2026/NĐ-CP`, `33/2026/QĐ-TTg`, `05/2026/TT-BKHCN` and `134/2025/QH15`.
Drop the initial yearless named entries because the keyword search already
covers them. Reason: the measured AI sample bounds the first adapter; retained
named entries preserve explicit coverage if titles change.

Use the [secondary discovery contracts](secondary-discovery.md) for central
issuer evidence, query items, exact named filtering, the candidate ceiling and
support attribution. Reason: this source is a central gap filler; explicit
classification and complete query evidence bound admission and later loss.

For every candidate, discover performs a fresh exact VBPL number lookup using
the wire and completeness rules above, then applies
[cross-source identity comparison](data.md#identity-comparison) before seeding.
Seed through this probe only with one proven identity and no unresolved
candidate; unresolved matches retain `source_identity_ambiguous`. Different-date
matches remain evidence and neither seed nor block admission. An incomplete
lookup records a source gap and defers new admission; it cannot remove an
existing secondary member. A complete lookup with no eligible or unresolved
match admits the candidate unless its own identity evidence conflicts with an
active member. Apply the table to positive matches. Fetch plans the candidate
superset and finalizes precedence before extract through
[source execution](source-execution.md#fetch-close-finalization). Post-fetch
deferral applies only to that candidate's conflicting identity evidence. Missing
metadata on an unrelated context id never blocks admission. Reason: narrow
discovery is not proof of absence, and unrelated failures must not veto the
secondary source.

Key secondary documents as `vn:chinhphu:<docid>`, class `secondary`. Compare
normalized number, issuer and issue date using
[data](data.md#identity-comparison). For a candidate-specific comparison with an
active VBPL member:

| Evidence                                                                                      | Membership decision                                                                                                                            |
| --------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------- |
| Number, issuer and issue date all match; one VBPL identity                                    | VBPL wins; mark secondary `source_replaced`.                                                                                                   |
| Equal number, different verified issue dates                                                  | Different documents; the probe neither seeds the VBPL result nor blocks secondary admission. Preserve evidence and any independent membership. |
| Equal number and date, conflicting issuer evidence                                            | Keep both with `source_identity_ambiguous`; never replace.                                                                                     |
| Equal number, missing or unverifiable date or issuer evidence, with no proven date difference | Keep both with `source_identity_ambiguous`; never replace.                                                                                     |
| Several VBPL identities remain possible                                                       | Keep all evidenced members with `source_identity_ambiguous`; never choose one.                                                                 |

The ambiguity rows are an explicit exception to single-source precedence;
deferral means deferring that candidate's precedence decision, not dropping its
metadata. Unrelated unequal numbers do not establish a candidate pair. Never
rewrite keys. A failed VBPL fetch does not reverse a hand-over already proved by
complete identity evidence; retain its fetch gap. Reason: number equality alone
cannot safely replace a yearless-number document.

Only the table's proven-match row removes the secondary key from active
membership. Fetch finalization records `source_replaced` on its fetch item,
naming old key, VBPL key and comparison evidence. The owner must accept it
before release; it does not mean legal repeal. Apply this exception to removal
items; build carries the gap and old absent key, not a secondary metadata row.
Keep old release databases and artifacts intact. Re-adjudicate evaluation
identity maps. Reason: provenance stays truthful and the new release avoids
duplicate evidence.

For other secondary disappearance or source-list removal, use the same stable
listing and retained-gap rules as VBPL, with source-specific query identities.
An unavailable lookup never triggers hand-over or proves withdrawal.

### Fetch and safety

The [secondary bundle](fetch-bundles.md#secondary-contents) retains redacted
detail HTML, parsed metadata, canonical URL, attachment inventory and signed PDF
bytes. Actual times live in workshop observations outside the bundle; preserve
names, URLs, sizes and hashes. Do not guess missing roles or status. Source
labels claiming a signed PDF do not prove cryptographic validation. No PDF
parsing, OCR, tree fabrication or inferred relations in this work: extract emits
`no_tree`, relate emits an empty evidenced relation set, and build keeps
metadata with no provisions/chunks. Reason: text extraction has its own plan and
acceptance criteria.

Use HTTPS with exact host allowlists: pages on `vanban.chinhphu.vn`, files on
`datafiles.chinhphu.vn`. Reject credentials, nonstandard ports and nonpublic
addresses; revalidate every redirect and stop at the budget. Never execute page
scripts or PDF content. Validate media type and PDF signature bytes, stream with
size limits, and use content hashes rather than source paths as local filenames.
Reason: public source content is untrusted input to the worker.

Keep ASP.NET cookies and hidden postback tokens only in memory. Artifacts and
fixtures retain placeholders, never cookie or token values. Use a named user
agent, source-wide throttling, bounded retries and `Retry-After` handling from
[budgets](budgets.md#pipeline). A challenge or access denial pauses the source
through the durable [source pause](source-execution.md#source-pause) contract;
do not bypass it. Reason: unattended work must remain polite across concurrent
batches and worker restarts.

A client enforces rate, burst and in-flight limits for every request it makes,
including attempts, retries and redirects across page and file hosts. The worker
shares one client instance per source across all batches and stages. One worker
process serves a run, so instance limits are source-wide limits. Stages that
call a narrow source also run at the per-stage in-flight limit. The client
instance counts the download budget for its own lifetime: a worker restart
resets it and runs sharing a worker share it. Durable per-run accounting is
deferred work. Limits remain in [budgets](budgets.md#pipeline).

## Dependencies and acceptance evidence

Release order: record fixtures and freeze the approved comparison; approve
remaining limits; implement [shared prerequisites](source-execution.md) before
source stages; integrate build inventory and schema; ingest and re-adjudicate
coverage before indexing. The primary owns the plan and independent review. Text
extraction follows separately; metadata coverage is not search recall.

The plan must include these spikes before adapter contracts are implemented:

- Record redacted ASP.NET search, pagination, empty/multiple results, detail and
  attachment fixtures; establish field selectors, central issuer evidence,
  complete-versus-truncated detection, token rotation, redirects and challenge
  recognition. Record safe PDF signature bytes, not only attachment headers.
  Provide offline seams for clocks, transport, DNS and reduced limits.
- Reconcile the claimed eleven AI documents and nine omissions with the supplied
  enumeration, which includes the already-present law and does not identify
  every claimed omission. Do not invent ids, years or missing document names.
- Record VBPL exact-number collisions, phrase-match losses, relation direction
  examples and malformed/non-JSON gateway responses; confirm the five known
  golden omissions against the corrected source research.
- Measure the full proposed seed set, including newly admitted seeds: unique
  targets, requests, bytes, time and final document count against budgets.
  Report source failures and second-hop gaps separately from successfully
  recovered first-hop targets.

The initial lists and migration losses are approved. Owner approval remains
required for later deliberate old-only exclusions and unapproved budget ceilings
after measurement. New sources or broader State Bank administrative scope need a
separate owner decision; this proposal implies none.
