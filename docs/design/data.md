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
  The retrieval recipe includes provisional threshold provenance. The immutable
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

| Table                 | Content                                                                                                                                                                                                                                                                                                                                                      |
| --------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `account`             | `id`, `google_sub` unique, current `email`, `blocked_at`, `created_at`, `last_login_at`, `delete_requested_at`, `auth_epoch`                                                                                                                                                                                                                                 |
| `web_session`         | `id`, `token_hash`, `account_id`, `csrf_secret`, `auth_epoch`, `created_at`, `last_seen_at`, `expires_at`, `revoked_at` ([web API](web-api.md#sessions))                                                                                                                                                                                                     |
| `login_transaction`   | `handle_hash`, `state`, `nonce`, `pkce_verifier`, `next_path`, `redirect_uri`, `created_at`, `expires_at`, `claimed_at` ([web API](web-api.md#google-login))                                                                                                                                                                                                 |
| `oauth_client`        | `id`, `client_id` unique (CIMD URL, pre-registered or generated ID), `kind` (`cimd`, `preregistered`, `dcr`), `name`, `redirect_uris`, `grant_types`, `metadata_sha256`, `created_at`, `last_used_at` ([OAuth](oauth.md))                                                                                                                                    |
| `oauth_request`       | `handle_hash`, `browser_hash`, `client_id`, `redirect_uri`, `state`, `code_challenge`, `resource`, `scope`, `created_at`, `expires_at`, `claimed_at`, `account_id` (set on claim)                                                                                                                                                                            |
| `oauth_grant`         | `id`, `account_id`, `client_id`, `scope`, `resource`, `created_at`, `last_used_at`, `revoked_at`; at most one live grant per account and client                                                                                                                                                                                                              |
| `oauth_code`          | `code_hash`, `grant_id`, `client_id`, `redirect_uri`, `resource`, `code_challenge`, `scope`, `auth_epoch`, `created_at`, `expires_at`, `consumed_at`                                                                                                                                                                                                         |
| `oauth_token`         | `token_hash`, `kind` (`access`, `refresh`), `grant_id`, `family_id`, `rotated_from`, `resource`, `scope`, `auth_epoch`, `created_at`, `expires_at`, `family_expires_at`, `revoked_at`, `superseded_at`, `last_used_at`                                                                                                                                       |
| `personal_token`      | `id`, `token_hash`, `account_id`, `name`, `created_at`, `expires_at` (at most one year, chosen at creation), `last_used_at`, `revoked_at`, `auth_epoch`                                                                                                                                                                                                      |
| `issue_report`        | `id`, `account_id`, `release`, `document_key`, `anchor`, `document_number`, `kind`, `target`, `suggestion` (typed JSON), `state` (`open`, `accepted`, `rejected`, `withdrawn`), `created_at`, `changed_at`, `decided_by`, `export_batch`; one open report per account and group key (document_key, anchor, kind, target) ([error reports](error-reports.md)) |
| `issue_report_export` | `batch_id`, `report_id`, `created_at`, `document_key`, `anchor`, `document_number`, `release`, `kind`, `target`, `suggestion`; the fixed membership of an export batch, without account data                                                                                                                                                                 |
| `corpus_release`      | `country`, `active_db`, `activated_at`, `activated_by`                                                                                                                                                                                                                                                                                                       |
| `retired_release`     | `country`, `db_name`, `retired_at`, `retain_until`, `dropped_at`                                                                                                                                                                                                                                                                                             |
| `release_journal`     | `id`, `country`, `rel`, `step`, `state`, `at`, `message`; append only                                                                                                                                                                                                                                                                                        |

Rules:

- No table stores question text. Logs record `account_id`, error code and
  latency, never the question or returned text; kept 30 days.
- Every credential is stored as a hash. `auth_epoch` increments on block, delete
  request and auth restore; tokens issued before the epoch are invalid.
- Account deletion revokes all sessions, tokens and grants in one transaction
  and removes the row after 7 days.

## Local workshop database (MacBook)

Tables and contracts: [workshop data](workshop-data.md).
