# Data model

Tables, keys and rules. The proposed exact corpus columns, indexes and
constraints are in the [corpus schema](corpus-schema.md), implemented by the
root `internal/corpusdb` template. Workshop changes use goose migrations.
Changing a rule here needs an ADR History entry when an ADR owns it.

## Corpus database `corpus_<cc>_<rel>` (read only when serving)

| Table                 | Content                                                                                                                                                  |
| --------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `document`            | Durable key, metadata, raw and parsed dates, mapped status/current flag, content state and source hashes                                                 |
| `source_file`         | Attachment name, hash, size and known provenance; missing URL, role or time stays explicit                                                               |
| `provision`           | Source node, actual parent, native kind/label/heading, source text, public anchor and internal ltree ancestry                                            |
| `provision_validity`  | Provision validity backed by evidence; empty at launch                                                                                                   |
| `chunk`               | One owning provision, separate content/prefix, source spans, dense and sparse vectors, actual token count and input hash                                 |
| `lexical_df`          | Complete pre-hash token document frequencies; summary and hashes in release lexical statistics                                                           |
| `relation`            | Source/target keys and numbers, nullable resolved document/provision IDs, raw source codes, evidence and method                                          |
| `taxonomy_node`       | Public code, parent, level, name, description, origin, status and internal ancestry                                                                      |
| `taxonomy_assignment` | Provision, node, family, method, exclusion, confidence and taxonomy version                                                                              |
| `coverage_gap`        | Kind, optional document/taxonomy identity, absent document key, source stage/item, detail and acceptance                                                 |
| `release`             | Singleton: schema/taxonomy versions; dense, chunk, lexical and retrieval recipes; lexical statistics; source/build hashes; evaluation status; build time |

Rules:

- `source_file.source` is the document key's provider: `vbpl` or `chinhphu` for
  Vietnam. Preserve source-specific provenance; a hand-over never rewrites an
  old file's provider. Membership and hand-over rules live in
  [sources](sources.md).
- Results are provisions, never chunks. Citation and validity come from
  `document` and `provision`.
- Preserve every valid extracted provision even if a short/duplicate chunk is
  omitted or its vector is rejected. Missing document endpoints remain named
  relations; no fake document or vector row conceals a gap. Invalid ancestry
  remains in its source artifact with a gap, not a broken corpus tree.
- Corpus tables for taxonomy and provision validity are empty in the index
  baseline; `taxonomy_version` is empty. Absence of taxonomy is not a passed
  classification or subject-filter gate.
- A provision may carry several assignments from different families. A node
  family is the set of nodes under one level-2 ancestor. Within a family a
  provision holds assignments from one method only: a `manual` entry replaces
  every `phapdien_seed` and `llm` entry in that family, and a seed replaces
  `llm` entries there. A manual entry may be negative (`exclude`), which removes
  the family from the provision and blocks later automatic entries.
- The public identity of a node is its `code`; integer ids are internal. A
  retired node keeps its code; codes are never reused.
- Provision-level repeal shows as "a repeal relation targets this provision",
  not as a changed stamp, until `provision_validity` has evidence.
- `as_of` applies document-level dates only. Responses say so.
- Everything that changes over time belongs to the release: schema version,
  dense recipe (model, dimensions, pooling, prompt, quantization), lexical
  recipe (tokenizer version, normalization, BM25 parameters), lexical stats,
  chunk recipe, retrieval recipe, taxonomy version, source manifest, evaluation.
  Recipe 1 retrieval includes provisional threshold provenance. The immutable
  build dump starts with evaluation `not_run`; [eval](evaluation.md) produces a
  hash-linked sidecar. The host plan must bind that sidecar in its signed
  manifest. The server holds no corpus configuration except the release pointer.

### Official number normalization (proposed extension)

Keep the raw official number. The shared matching form uses NFC, trim,
uppercase, Cyrillic look-alike folding, `Đ` to `D`, then removes whitespace
around `/` and `-`. Fold only `А→A`, `В→B`, `С→C`, `Е→E`, `Н→H`, `К→K`, `М→M`,
`О→O`, `Р→P`, `Т→T`, `Х→X`; lowercase forms follow uppercasing. Thus `NĐ-CР` and
`NĐ-CP` compare equally. Retain leading zeroes, years and other characters;
never infer a missing year or use substring equality. Reason: known cache
confusables need repair without collapsing distinct numbers.

Use one versioned function for discovery comparisons, source precedence and
corpus `number_norm`; serving document-number lookup must use the same version.
Wire queries preserve official `Đ` spelling after confusable and whitespace
cleanup. Freeze the normalizer version in dependent recipes. Ambiguous equality
remains a candidate set, never proof that two documents are identical. Legacy
evaluation's historical matcher stays unchanged under ADR 0010.

### Named-entry eligibility

VBPL named entries carry a number and optional civil issue date (`YYYY-MM-DD`);
the qualifier is required without a four-digit year segment in the number. A
returned item is eligible only when normalized numbers match and its parsed
issue date equals the qualifier, if present. An unqualified entry must contain a
four-digit year segment and the item must have a parsed issue date. An
equal-number result with a missing or unparsable date is unresolved and makes
the entry ambiguous. A different verified date under a qualified entry is
ineligible, not unresolved. Preserve all result evidence. No issuer evidence is
required: the entry has no expected issuer and VBPL is primary. This rule
selects a VBPL document; it does not establish cross-source identity.

### Identity comparison

Only cross-source comparisons, for secondary probes and hand-over, require
issuer comparison: use the number form above; issuer NFC, trimmed and
whitespace-collapsed with uppercase; and a fully parsed civil issue date in
`YYYY-MM-DD`. Preserve raw values and evidence references. No fuzzy issuer
aliases or inferred dates. Missing, unparsable or contradictory fields are
unverifiable. Equal normalized numbers identify the same document only with
equal issue dates and matching issuer evidence. Different verified issue dates
establish different documents, even for equal numbers; preserve that evidence
without seeding or blocking secondary admission. Missing dates on either side,
or missing/conflicting issuer evidence without a proven date difference, retain
ambiguity. [Sources](sources.md) owns the hand-over table. A fetch failure does
not invalidate identity evidence already present in a complete listing, but
cannot supply missing fields.

## Auth database `auth` (written when serving)

| Table               | Content                                                                                                                                                                                                                |
| ------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `account`           | `id`, `google_sub` unique, current `email`, `blocked_at`, `created_at`, `last_login_at`, `delete_requested_at`, `auth_epoch`                                                                                           |
| `web_session`       | `id`, `token_hash`, `account_id`, `csrf_secret`, `auth_epoch`, `created_at`, `last_seen_at`, `expires_at`, `revoked_at` ([web API](web-api.md#sessions))                                                               |
| `login_transaction` | `handle_hash`, `state`, `nonce`, `pkce_verifier`, `next_path`, `redirect_uri`, `created_at`, `expires_at`, `claimed_at` ([web API](web-api.md#google-login))                                                           |
| `oauth_client`      | `id`, `client_id` unique (CIMD URL, pre-registered or generated ID), `kind` (`cimd`, `preregistered`, `dcr`), `name`, `redirect_uris`, `metadata_sha256`, `created_at`, `last_used_at` ([OAuth](oauth.md))             |
| `oauth_request`     | `handle_hash`, `client_id`, `redirect_uri`, `state`, `code_challenge`, `resource`, `scope`, `created_at`, `expires_at`, `claimed_at`                                                                                   |
| `oauth_grant`       | `id`, `account_id`, `client_id`, `scope`, `resource`, `created_at`, `last_used_at`, `revoked_at`; at most one live grant per account and client                                                                        |
| `oauth_code`        | `code_hash`, `grant_id`, `client_id`, `redirect_uri`, `resource`, `code_challenge`, `scope`, `auth_epoch`, `created_at`, `expires_at`, `consumed_at`                                                                   |
| `oauth_token`       | `token_hash`, `kind` (`access`, `refresh`), `grant_id`, `family_id`, `rotated_from`, `resource`, `scope`, `auth_epoch`, `created_at`, `expires_at`, `family_expires_at`, `revoked_at`, `superseded_at`, `last_used_at` |
| `personal_token`    | `id`, `token_hash`, `account_id`, `name`, `created_at`, `expires_at` (at most one year, chosen at creation), `last_used_at`, `revoked_at`, `auth_epoch`                                                                |
| `corpus_release`    | `country`, `active_db`, `activated_at`, `activated_by`                                                                                                                                                                 |
| `retired_release`   | `country`, `db_name`, `retired_at`, `retain_until`, `dropped_at`                                                                                                                                                       |
| `release_journal`   | `id`, `country`, `rel`, `step`, `state`, `at`, `message`; append only                                                                                                                                                  |

Rules:

- No table stores question text. Logs record `account_id`, error code and
  latency, never the question or returned text; kept 30 days.
- Every credential is stored as a hash. `auth_epoch` increments on block, delete
  request and auth restore; tokens issued before the epoch are invalid.
- Account deletion revokes all sessions, tokens and grants in one transaction
  and removes the row after 7 days.

## Local workshop database (MacBook)

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

### Scope workshop contracts

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
  agency id; `term` with term; `named` with the named entry; `secondary_probe`
  with `<chinhphu item key>|<docid>` under the
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
(`network`,`cache`,`adopted`), nullable `fetched_at`, and nullable `origin_ref`
to a retained observation or verified cache record. Resource keys are compact
JSON tuples `[endpoint,name,url]`, with null name for a response and null URL
when unknown. Paths identify only retained source payloads or attachments under
the [observation scope](fetch-bundles.md#observation-commit-and-recovery), never
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
producing stage/item, reason and evidence references. Codes are `scope_changed`
and
[`secondary_candidate_ceiling`](secondary-discovery.md#candidate-ceiling-and-close).
Both are durable: start, gap acceptance and release refuse the run; only a new
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

### Relation support evidence

A `relation` tuple's `rule_value` is `<seed document key>|<raw code>`. Its
evidence reference contains the detail or diagram response SHA-256, expand
artifact hash and `detail.json` or `diagram.json` path. Retention and removal
preserve that original evidence, not the later observation's hash.

The producing stage/item is `expand` and the seed document key for current,
retained and link-loss removed tuples. A tuple removed because its seed left the
active seeds belongs to `discover` / `vbpl:central`. This attribution does not
change its relation kind or logical production stage for
[retry invalidation](source-execution.md#retry-source-gaps). Discovery's
frozen-evidence check covers the
[direct seed kinds](membership.md#direct-discovery), excluding these
discovery-attributed relation removals.

### Expand observation artifact

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
