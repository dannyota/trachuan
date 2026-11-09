# Vietnam corpus scope

Proposed execution contract for the owner's 2026-11-09 source choice in
[ADR 0012](../adr/0012-vietnam-corpus-sources.md). VBPL remains primary;
`chinhphu` fills central-document omissions, starting with AI. This note owns
selection, identity and loss rules; [pipeline](pipeline.md) owns execution,
[data](data.md) owns stored fields and [budgets](budgets.md#pipeline) owns
limits. Shared interfaces and close transactions live in
[source execution](source-execution.md).

## Evidence and boundaries

Use the primary's measurements and the
[source research](../research/vn-sources-2026-11.md). They supersede the missing
document list in the earlier
[ingest report](../research/vbpl-ingest-2026-11.md). No network research was
authorized for this design. Endpoint behavior below is measured evidence, not a
published API guarantee; fixtures must pin it.

First-party entry points supplied by that evidence:

- [VBPL listing](https://vbpl-bientap-gateway.moj.gov.vn/api/qtdc/public/doc/all)
  accepts `optionDoc: title|docNum`, `matchMode: all_words|exact` and
  `agencyLevel: TRUNG_UONG|DIA_PHUONG`.
- [Government documents](https://vanban.chinhphu.vn/) use an ASP.NET postback
  search and `/?pageid=27160&docid=<n>` detail pages. No JSON API was found.
- [Signed PDF example](https://datafiles.chinhphu.vn/cpp/files/vbpq/2026/4/142-2026-ndcp.signed.pdf)
  establishes an attachment host, not a filename template or verified signature.

## Direct discovery

Retain the configured State Bank sweeps and central keyword agencies. A direct
VBPL hit is class `primary`, whether found by sweep, title, named number or a
secondary candidate's VBPL number probe. Reason: discovery route must not change
a document's authority or searchable content.

Add `vbpl.named_numbers` to `sources/vn.yaml`. Initially include `71/2025/QH15`,
`368/2025/NĐ-CP`, `58/2021/NĐ-CP`, `18/2018/TT-NHNN`, `134/2025/QH15`,
`53/2026/TT-NHNN` and `142/2026/NĐ-CP`. Reason: explicit coverage must survive
titles that do not contain a subject keyword.

Discover each number with `optionDoc: docNum`, `matchMode: exact`,
`agencyLevel: TRUNG_UONG`, no agency filter, and all result pages. Compare
normalized returned numbers locally; an API hit alone does not establish
equality. Deduplicate by VBPL id. One equal result joins the primary seeds; zero
produces `named_document_missing`; several produce `named_document_ambiguous`
with all candidate ids. Never pick the newest result. Already independently
discovered candidates stay expected. Reason: official numbers are not unique,
and a named lookup must expose uncertainty.

Every search or lookup returns the [evidence-bearing result](source-execution.md#lookup-and-search-results).
Apply zero/one/many rules only to complete results. A partial result may add
observed ids, but cannot prove absence, uniqueness or withdrawal.

A zero VBPL match can be covered by one eligible secondary candidate with the
same number: keep the zero-match evidence and record secondary coverage instead
of `named_document_missing`. Ambiguity is never converted to absence. Correct or
qualify an ambiguous entry in a later reviewed source list, never inside a
frozen run. The initial contract has no fuzzy title or inferred-year fallback.

### Keyword list and loss gate

Use `optionDoc: title`, `matchMode: exact` for keyword queries. Keep these
reviewed proposal terms, adding the digital technology industry term because the
known golden law is missed by current discovery:

| Area                    | Terms                                                                                              |
| ----------------------- | -------------------------------------------------------------------------------------------------- |
| Security                | an toàn thông tin; an ninh mạng; dữ liệu cá nhân; bảo vệ dữ liệu cá nhân                           |
| Electronic transactions | giao dịch điện tử; chữ ký số; chữ ký điện tử; định danh điện tử; thương mại điện tử                |
| AI and technology       | trí tuệ nhân tạo; công nghiệp công nghệ số; cơ chế thử nghiệm có kiểm soát; công nghệ tài chính    |
| Data and identity       | dữ liệu quan trọng; dữ liệu cốt lõi; luật căn cước; luật dữ liệu; luật viễn thông                  |
| Banking operations      | hệ thống thông tin ngân hàng; thuê ngoài; kinh doanh liên tục                                      |
| Payments and assets     | thanh toán không dùng tiền mặt; ví điện tử; ngân hàng số; tiền điện tử; tài sản số; tài sản mã hóa |

`exact` semantics are undocumented. Before freezing the first migration run,
record complete stable `all_words` and `exact` listings for every proposed term
with identical agency filters in the same comparison session. Also compare the
previous source list's full seed set with the candidate full seed set, including
sweeps and named lookups. Save query definitions, response hashes, ids, numbers,
titles, totals and both set differences in an immutable comparison artifact. An
incomplete listing cannot pass this gate. Reason: fewer hits alone do not prove
that the removed documents are irrelevant.

The primary accounts for every old-only id: retained by another query, restored
with a named number or revised term, or excluded with an owner-approved scope
reason. No unexplained loss and no loss of the known VBPL golden documents is
allowed. Include AI-related documents beyond the golden set in this review.
Freeze one immutable migration comparison hash and approval note with the source
snapshot. Later list changes need a loss review only for changed queries that
can remove coverage, such as removals or narrower filters. Pure additions need
no repeated migration comparison. Bind each loss review to the changed query
definitions and old-only ids; carry prior reviews forward unchanged. Never edit
a started run. Reason: review actual losses without repeating unrelated work.

Query identity hashes source, query type, option, mode, level, agencies and
keyword or number, with canonical ordering. Changing mode creates a new query;
old-only documents become `scope_removed`, not evidence of source withdrawal.
The comparison supplies their loss reasons. Normal two-listing withdrawal rules
in [pipeline](pipeline.md) apply to unchanged query identities.

## One-hop relation context

Add `expand` between `discover` and `fetch`, with one item per active primary
VBPL seed. Freeze that seed list before planning expand; never plan from its
output. Each item reads detail and diagram by VBPL id and records both payloads
and hashes. Like discover, expand observes the source afresh each run and learns
its content hash after download; a seed id is not a cache-validity signal. Fetch
still downloads a fresh full bundle for every expected id. Reason: a separate
bounded stage fits the existing batch/checkpoint model and avoids a mutable work
queue inside fetch or recursive relate execution.

Qualify explicit outward targets in the seed's `references.targetDocument` and
`documentNamesByType`, using the existing relation-code table: `legal_basis`,
`implements`, `guided_by`, `amends_supplements`, `replaces`, `repeals`,
`suspends`, `consolidates`, `cites`, `references`. Direction means the seed's
source record points to that target; preserve the kind's meaning, including
`guided_by`. Do not reverse edges or search for inbound documents. Reason: even
citations supply useful legal context, but inferred inbound edges would widen
scope without source evidence.

Only a valid explicit VBPL id adds a target; keep number-only references as
`unknown_target` for later named-list review. Unknown codes remain gaps and add
no targets. Exclude `vbpqta_` renditions and self-links. Central or local
targets may qualify by id without opening local title sweeps. Reason: the
source's specific link supplies scope, while guessing a number match or
jurisdiction would hide identity errors.

Union and deduplicate targets as class `relation_context`; a direct seed always
wins class `primary`. Retain every seed, raw code and evidence hash that caused
membership. Context never seeds expansion, even when a previous run called it
primary. Fetch by id and use the normal extract, relate, chunk and embed stages.
Valid provisions are searchable without a special ranking penalty; missing trees
stay metadata-only. Reason: agents need the cited law's text, and a
metadata-only class would preserve the coverage failure.

Expand freezes active membership, excluding retained `withdrawn` and
`scope_removed` keys. A failed observation retains known targets of that active
seed with a gap; a new failed seed has unknown coverage. Compare each primary
seed's qualified fresh fetch targets against this frozen active membership.
An absent target, including a retained removal key, causes non-retryable
`scope_changed` failure with seed, target, reason and both evidence hashes.
Persist it as a run failure, never an acceptable gap. Start, gap acceptance and
release cannot clear it; a new run is required. Check during run-specific
fetch finalization, never inside cached relation artifacts. Context documents'
second-hop edges remain `unknown_target` gaps without expansion. Reason: the
fetch inventory must not conceal an unfetched target or allow accepted drift.

For context-only membership, keep the target while any active seed points to it.
If every supporting seed leaves scope, retain a `scope_removed` gap item without
fetching. Otherwise consider only currently active supporting seeds: when all
have complete observations and none retains the link, keep a `withdrawn` gap
item without fetching. A failed or incomplete observation retains its known
targets for fetch. A departed seed cannot prevent removal; a current direct hit
or qualifying link always retains the document. No two-observation history is
required. Reason: protect against failed observations without tracking absent
seeds or delaying a fully evidenced removal.

## Secondary discovery and hand-over

Add `chinhphu.keywords` and `chinhphu.named_numbers` to the source list, frozen
like VBPL inputs. Start keywords with `trí tuệ nhân tạo`; start named numbers
with `142/2026/NĐ-CP`, `33/2026/QĐ-TTg`, `05/2026/TT-BKHCN`, `367/QĐ-TTg`,
`804/QĐ-TTg`, `1671/QĐ-TTg`, `1528/QĐ-TTg`, `127/QĐ-TTg` and `134/2025/QH15`.
Reason: the measured AI sample bounds the first adapter while named entries
cover relevant titles that omit the keyword.

Discover candidates from recorded, paginated search responses and detail pages;
deduplicate by docid. Require explicit central issuer evidence and an intact
official number. Provincial documents are out of scope. Unknown jurisdiction or
number produces `invalid_source_data`, never inferred inclusion. Apply the same
zero/multiple named-result rules as VBPL. Reason: this source is a central gap
filler, not a second unrestricted corpus.

For every candidate, discover performs a fresh exact VBPL number lookup with the
direct-discovery rules. A unique match becomes a primary seed, even if no
keyword found it. An incomplete lookup records a source gap and defers new
admission; it cannot remove an existing secondary member. A complete zero-match
result admits the candidate unless its own identity evidence conflicts with an
active member. Compare positive matches with the table below, including multiple
VBPL matches. Fetch plans the candidate superset and finalizes precedence before
extract through [source execution](source-execution.md#fetch-close-finalization).
Post-fetch deferral applies only to that candidate's conflicting identity
evidence. Missing metadata on an unrelated context id never blocks admission.
Reason: narrow discovery is not proof of absence, and unrelated failures must
not veto the secondary source.

Key secondary documents as `vn:chinhphu:<docid>`, class `secondary`. Compare
normalized number, issuer and issue date using [data](data.md#identity-comparison).
For a candidate-specific comparison with an active VBPL member:

| Evidence | Membership decision |
| --- | --- |
| Number, issuer and issue date all match; one VBPL identity | VBPL wins; mark secondary `source_replaced`. |
| Any comparison field conflicts | Keep both with `source_identity_ambiguous`; never replace. |
| Any comparison field is missing or unverifiable | Keep both with `source_identity_ambiguous`; never replace. |
| Several VBPL identities remain possible | Keep all evidenced members with `source_identity_ambiguous`; never choose one. |

The ambiguity rows are an explicit exception to single-source precedence;
deferral means deferring that candidate's precedence decision, not dropping its
metadata. Unrelated unequal numbers do not establish a candidate pair. Never
rewrite keys. A failed VBPL fetch does not reverse a hand-over already proved by
complete identity evidence; retain its fetch gap. Reason: number equality alone
cannot safely replace a yearless-number document.

Only the table's proven-match row removes the secondary key from active
membership. Fetch finalization records `source_replaced` on its fetch item,
naming old key, VBPL key and comparison evidence. The owner must accept it before
release; it does not mean legal repeal. Apply this exception to removal items; build
carries the gap and old absent key, not a secondary metadata row. Keep old
release databases and artifacts intact. Re-adjudicate evaluation identity maps.
Reason: provenance stays truthful and the new release avoids duplicate evidence.

For other secondary disappearance or source-list removal, use the same stable
listing and retained-gap rules as VBPL, with source-specific query identities.
An unavailable lookup never triggers hand-over or proves withdrawal.

### Fetch and safety

Store token-redacted detail HTML, parsed metadata, canonical URL, attachment
inventory and each linked signed PDF as hashed files. Preserve original names,
URLs, actual fetch times, sizes and hashes; do not guess missing roles or
status. Source labels claiming a signed PDF do not prove cryptographic
validation. No PDF parsing, OCR, tree fabrication or inferred relations in this
work: extract emits `no_tree`, relate emits an empty evidenced relation set, and
build keeps metadata with no provisions/chunks. Reason: text extraction has its
own plan and acceptance criteria.

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

## Dependencies and acceptance evidence

Release order: record fixtures and the keyword loss comparison; approve scope
and limits; implement the shared [prerequisites](source-execution.md) before
discovery, expansion or source adapters; integrate build inventory and schema;
run a new ingest and re-adjudicate coverage before indexing. The primary owns
the plan and independent review. Text
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

Owner approval is still needed for deliberate old-only exclusions, the final
keyword/named lists and budget ceilings after measurement. New sources or a
broader State Bank administrative scope need a separate owner decision. No
decision on those is implied by this proposal.
