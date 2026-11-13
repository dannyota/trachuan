# Workshop database

The local workshop database on the MacBook: run state, artifacts, gaps and the
scope contracts the source stages share. [Data model](data.md) owns the corpus
and auth databases.

| Table              | Content                                                                                                                                                                                                                                                                                                                          |
| ------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `run`              | `id`, `country`, `rel`, `generation`, `state`, `next_stage`, `until_stage`, Temporal workflow id, frozen snapshot, `message`, timestamps; lifecycle in [pipeline](pipeline.md#run-lifecycle)                                                                                                                                     |
| `run_item`         | `run_id`, `stage`, `item_key`, `input_hash`, `recipe_hash`, `state` (pending, done, gap, failed), `batch`, `generation`, `attempt`, `artifact_id`, `message`                                                                                                                                                                     |
| `submission`       | `id` (32 hex, prefix of every `custom_id`), `run_id`, `stage`, `provider`, ordered item list and its hash, `provider_job_id`, `state` (preparing, submitted, uncertain, abandoned, completed, cancelling, cancelled, failed), `created_at`, `submitted_at`, `deadline_at`, cost; mirrored in the intent log outside the workshop |
| `artifact`         | `stage`, `item_key`, `input_hash`, `recipe_hash`, `path`, `sha256`, `written_at`                                                                                                                                                                                                                                                 |
| `gap`              | `run_id`, `stage`, `item_key`, `kind`, `detail`, `accepted_at`, `note` (the owner's note when accepted)                                                                                                                                                                                                                          |
| `listing`          | `run_id`, source query, `item_key`, payload                                                                                                                                                                                                                                                                                      |
| `document`         | fetched document metadata, keyed by the artifact that produced it                                                                                                                                                                                                                                                                |
| `fetched_file`     | fetched attachment metadata, keyed by the artifact that produced it                                                                                                                                                                                                                                                              |
| `provision`        | extracted structure and verbatim text, keyed by the artifact that produced it                                                                                                                                                                                                                                                    |
| `relation`         | source relations and evidence, keyed by the artifact that produced it                                                                                                                                                                                                                                                            |
| `chunk`            | Artifact ordinal, owning extracted provision, prefix, content, spans and rough size; exact projection in [chunk](chunk.md#workshop-projection-and-checks)                                                                                                                                                                        |
| Later stage tables | Assignments when taxonomy lands; vectors stay only in verified embedding artifacts, never a workshop vector table                                                                                                                                                                                                                |

Publication is blocked while any expected `run_item` is `pending` or `failed` or
any `gap` row is unaccepted.

## Scope workshop contracts

These migrations are required before source-stage work; they are not optional
per-adapter additions. Interface and transaction rules live in
[source execution](source-execution.md).

- `run_document`, keyed by `(run_id,document_key)`: `source`, `source_id`,
  `class` in (`primary`,`relation_context`,`secondary`), `disposition` in
  (`active`,`withdrawn`,`scope_removed`,`source_replaced`), nullable raw and
  normalized number, and nullable `replacement_key`. Require replacement key
  exactly for `source_replaced`. Key must equal country, source and source id.
- `run_membership_evidence`: run, document key, producing stage/item, support
  state (`current`, `retained`, `removed`) and typed support tuple
  `(rule_kind, rule_value, evidence_reference)`. Kinds and values: `agency` with
  agency id; `term` with term; `named` with the named entry; `pinned` with the
  pinned key ([pinned documents](pinned-documents.md)); `secondary_probe` with
  `<chinhphu item key>|<docid>` under the
  [secondary support contract](secondary-discovery.md#support-and-retention-evidence);
  `relation` under the [relation evidence contract](#relation-support-evidence).
  The evidence reference carries listing or lookup query identity and page when
  applicable, response SHA-256, artifact hash and relative evidence path;
  deduplicate by these fields. [Membership](membership.md) owns supersession,
  retention and removal lifetime.
- `run_seed_observation`: run, seed key, producing expand item, completeness,
  nullable artifact hash and observed targets. Incomplete observations have no
  artifact hash. Membership can be rebuilt from these records under the
  [relation loss rules](membership.md#relation-loss).
- `run_scope`: run, seed/expanded/final snapshot JSON and their hashes, nullable
  until that phase closes, plus nullable fetch-close input hash. Snapshots
  contain byte-sorted membership and complete typed support tuples in every
  phase (`seeds`, `expanded`, `final`), serialized with the
  [inventory](index.md#build-stage) convention. The inventory commits to them
  through the final snapshot hash
  ([scope adoption](scope-adoption.md#inventory)), so they change
  `identity_inventory_hash` even when document keys do not. Freeze each phase's
  stored JSON and hash on close. Expanded active keys exclude retained removals;
  finalization may exclude candidates but never add unplanned keys. Retry
  invalidation clears only the affected phase references.
- Add three nullable workshop `run_scope` columns: `predecessor_run_id`,
  `predecessor_phase` (`final` or `legacy`) and `predecessor_snapshot_hash`.
  Require all three together or all null before selection or when none is
  chosen. The hash binds the selected final snapshot or the legacy reader's
  canonical membership projection using the same snapshot serialization.
  Selection lives in [membership](membership.md#membership-predecessor);
  freezing and verification live in
  [source execution](source-execution.md#shared-membership-and-artifacts).

Active and retained-removal members form the expected set; replacement records
only audit exclusion. Removal members follow the
[removal lifetime](membership.md#removal-lifetime). Fetch work can be a larger
candidate superset. The class is run membership, not legal document type or
ranking. Build copies the final class to the corpus column defined in
[schema](corpus-schema.md).

The workshop `document` projection carries `key`, `country`, `source`,
`source_id`, `number`, `title`, `doc_type`, `issuer`, `raw_issue_date`,
`raw_eff_from`, `raw_eff_to`, `status_code`, `status_name`, `is_consolidated`,
`has_content`, nullable `source_url`, and `identity_evidence` references.
Identity fields, dates and status retain raw values. The existing artifact id
remains the row owner. Source id is opaque text; a VBPL-specific id is never
required for a secondary row.

`fetched_file` carries `source`, `name`, `sha256`, `size`, artifact-relative
`path`, nullable `url`, `role` and nullable `fetched_at`. Role uses the corpus
vocabulary with explicit `unknown`. Source must agree with the owning document.
The row's time is the actual download time known at its first artifact-row
commit, or null. Reuse never updates that value; adoption time is not fetch
time. [Fetch bundles](fetch-bundles.md) owns the wire formats, observation
lifetime and recovery rules. Bundle files omit fetch times.

Add `run_fetch_observation`, keyed by
`(run_id,item_key,generation,attempt,resource_key)`: artifact SHA-256, source,
artifact-relative path, nullable URL, content SHA-256, size, acquisition
(`network`,`cache`,`adopted`,`pinned`), nullable `fetched_at`, and nullable
`origin_ref` to a retained observation, verified cache record or, for `pinned`,
the producing run's observation ([pinned documents](pinned-documents.md)).
Resource keys are compact JSON tuples `[endpoint,name,url]`, with null name for
a response and null URL when unknown. Paths identify only retained source
payloads or attachments under the
[observation scope](fetch-bundles.md#observation-commit-and-recovery), never
generated envelopes; distinct resources may share a content path. Times are UTC
completion times; cached/adopted bytes inherit only a verifiable origin time.
Unknown origins stay null. Rows are immutable audit evidence outside the
artifact; retry invalidation clears their current item association, not their
history. Only observations matching the committed item's artifact and attempt
are current.

Extend shared `Gap` and stored `gap` with nullable `source`, `origin`
(`gateway`,`source_data`,`transport`,`unknown`), `cause`
(`http_5xx`,`http_429`,`non_json_200`,`timeout`,`connection_error`,
`invalid_payload`,`unknown`), endpoint name, HTTP status and redacted evidence
reference. Propagate them through retry errors, outputs, resolution and commit.
For legacy `invalid_source_data`, automatic retry requires `origin=gateway` and
`cause=non_json_200|http_5xx`; unknown old rows are not reclassified by text.
The independent `source_unavailable` kind remains retry-eligible. Reason:
restart decisions must distinguish gateway failures from invalid legal data.

`run_failure`, keyed by run and failure identity, stores non-acceptable code,
producing stage/item, reason and evidence references. Codes are `scope_changed`,
[`secondary_candidate_ceiling`](secondary-discovery.md#candidate-ceiling-and-close),
and `pin_invalid` and `pin_conflict` ([pinned documents](pinned-documents.md)).
All are durable: start, gap acceptance and release refuse the run; only a new
run under the failure's recovery rule clears it. This record is separate from
`gap`.

`run_source`, keyed by run/source, stores `state` (`active`,`source_paused`),
pause reason, redacted evidence reference and paused/cleared timestamps. Append
`run_source_event` entries for pause and explicit clear, including the owner's
clear note. Source gates and worker startup read this shared state; retry reset
never deletes it. Retain only operational identifiers, no personal data or
response bodies in failure/state rows.

The run snapshot binds the immutable migration comparison hash and approval
note, plus later query-specific loss-review references. Scope projections,
bundle formats and snapshot fields are versioned; an unknown version fails
explicitly rather than defaulting class, source or completeness.

## Relation support evidence

A `relation` tuple's `rule_value` is `<seed document key>|<raw code>`. Its
evidence reference contains the detail or diagram response SHA-256, expand
artifact hash and `detail.json` or `diagram.json` path. Retention and removal
preserve that original evidence, not the later observation's hash.

The producing stage/item is `expand` and the seed document key for current,
retained and link-loss removed tuples. A tuple removed because its seed left the
active seeds belongs to `discover` / `vbpl:central`. This attribution does not
change its relation kind or logical production stage for
[retry invalidation](source-retry.md). Discovery's frozen-evidence check covers
the [direct seed kinds](membership.md#direct-discovery), excluding these
discovery-attributed relation removals.

## Expand observation artifact

`observation.json` version 1 contains `seed_key`, `targets`, `number_only` and
`unknown_codes`. Each target has `target_key`, `raw_code`, `kind`, `method`
(`vbpl_references` or `vbpl_diagram`), `response` (`detail.json` or
`diagram.json`) and `response_sha256`. The artifact retains both response files.
Completeness and the artifact hash belong to `run_seed_observation`, not this
JSON envelope.

An explicit target id matches `^[0-9A-Za-z][0-9A-Za-z_-]{0,63}$`, does not start
with `vbpqta_` (case-insensitive), and differs from the seed id. Only qualified
relation kinds in [sources](sources.md#one-hop-relation-context) add targets.
Sort targets by target key, raw code and method; deduplicate that triple.

Number-only records carry raw code and number or diagram name, including diagram
entries without an id. They are audit only and never expand membership. Sort and
deduplicate by raw code and number. Sort and deduplicate unknown codes; they add
no targets. Never resolve a number by guessing an id.
