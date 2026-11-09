# Data model

Tables, keys and rules. The proposed exact corpus columns, indexes and
constraints are in [schema version 1](corpus-schema.md), implemented by the root
`internal/corpusdb` template. Workshop changes use goose migrations. Changing a
rule here needs an ADR History entry when an ADR owns it.

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

## Auth database `auth` (written when serving)

| Table                                                      | Content                                                                                                                             |
| ---------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------- |
| `account`                                                  | `id`, `google_sub` unique, current `email`, `blocked_at`, `created_at`, `last_login_at`, `delete_requested_at`, `auth_epoch`        |
| `web_session`                                              | `token_hash`, `account_id`, `created_at`, `expires_at`, `revoked_at`                                                                |
| `oauth_client`, `oauth_grant`, `oauth_code`, `oauth_token` | as defined by the copied `oauthsrv` store, pinned to one aboutme commit; refresh and access tokens stored hashed with `revoked_at`  |
| `personal_token`                                           | `token_hash`, `account_id`, `name`, `created_at`, `expires_at` (at most one year, chosen at creation), `last_used_at`, `revoked_at` |
| `corpus_release`                                           | `country`, `active_db`, `activated_at`, `activated_by`                                                                              |
| `retired_release`                                          | `country`, `db_name`, `retired_at`, `retain_until`, `dropped_at`                                                                    |
| `release_journal`                                          | `id`, `country`, `rel`, `step`, `state`, `at`, `message`; append only                                                               |

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
