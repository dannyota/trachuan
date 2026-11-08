# Data model

Tables, keys and rules. Exact DDL belongs to migrations and the corpus schema
template. Changing a rule here needs an ADR History entry when an ADR owns it.

## Corpus database `corpus_<cc>_<rel>` (read only when serving)

| Table                 | Content                                                                                                         |
| --------------------- | --------------------------------------------------------------------------------------------------------------- |
| `document`            | `id`, durable `key` (`vn:vbpl:<ItemID>`), number, title, type, issuer, issue date, `eff_from`, `eff_to`, source validity status, `is_consolidated` |
| `source_file`         | `document_id`, `source` (vbpl, congbao, sbv), `url`, `hash`, `role` (main, appendix, scan), `fetched_at`      |
| `provision`           | `id`, `document_id`, `parent_id`, `kind` (text, per-country list), `label` native, `ordinal`, `path` ltree, `text` verbatim, `anchor` |
| `provision_validity`  | provision-level validity when evidence exists; empty at launch                                                  |
| `chunk`               | `id`, `provision_id`, `text` with context prefix, `embedding` (dimensions from the release recipe, HNSW cosine), `sparse` (BM25 `sparsevec`), `token_count` |
| `relation`            | `source_document_id`, `target_document_id`, `kind`, optional `source_provision_id`, `target_provision_id`, `evidence` verbatim, `method` |
| `taxonomy_node`       | `id`, `country`, `parent_id`, `level`, public `code`, `name`, `description`, `origin` (phapdien, custom), `status` (active, retired), `path` ltree |
| `taxonomy_assignment` | `provision_id`, `node_id`, `method` (manual, phapdien_seed, llm), `confidence`, `taxonomy_version`               |
| `coverage_gap`        | `kind`, `document_id` or `node_id`, message                                                                     |
| `release`             | one row: `rel`, `schema_version`, `taxonomy_version`, `dense_recipe`, `lexical_recipe`, `source_manifest_hash`, evaluation summary, `built_at` |

Rules:

- Results are provisions, never chunks. Citation and validity come from
  `document` and `provision`.
- A provision may carry several assignments. Precedence when methods disagree
  for the same node family: `manual` over `phapdien_seed` over `llm`.
- The public identity of a node is its `code`; integer ids are internal. A
  retired node keeps its code; codes are never reused.
- Provision-level repeal shows as "a repeal relation targets this provision",
  not as a changed stamp, until `provision_validity` has evidence.
- `as_of` applies document-level dates only. Responses say so.
- Everything that changes over time belongs to the release: schema version,
  dense recipe (model, dimensions, pooling, prompt, quantization), lexical
  recipe (tokenizer version, normalization, BM25 parameters, vocabulary hash),
  taxonomy version, source manifest, evaluation. The server holds no corpus
  configuration except the active release pointer.

## Auth database `auth` (written when serving)

| Table                                | Content                                                                                       |
| ------------------------------------ | --------------------------------------------------------------------------------------------- |
| `account`                            | `id`, `google_sub` unique, current `email`, `blocked_at`, `created_at`, `last_login_at`, `delete_requested_at`, `auth_epoch` |
| `web_session`                        | `token_hash`, `account_id`, `created_at`, `expires_at`, `revoked_at`                          |
| `oauth_client`, `oauth_grant`, `oauth_code`, `oauth_token` | as defined by the copied `oauthsrv` store, pinned to one aboutme commit; refresh and access tokens stored hashed with `revoked_at` |
| `personal_token`                     | `token_hash`, `account_id`, `name`, `last_used_at`, `revoked_at`                              |
| `corpus_release`                     | `country`, `active_db`, `previous_db`, `activated_at`, `activated_by`                         |

Rules:

- No table stores question text. Logs record `account_id`, error code and
  latency, never the question or returned text; kept 30 days.
- Every credential is stored as a hash. `auth_epoch` increments on block,
  delete request and auth restore; tokens issued before the epoch are invalid.
- Account deletion revokes all sessions, tokens and grants in one transaction
  and removes the row after 7 days.

## Local workshop database (MacBook)

| Table        | Content                                                                                         |
| ------------ | ----------------------------------------------------------------------------------------------- |
| `run`        | `id`, `country`, `rel`, `started_at`, `finished_at`, `state`, Temporal workflow id               |
| `run_item`   | `run_id`, `stage`, `item_key`, `input_hash`, `recipe_hash`, `state` (pending, done, gap, failed), `attempts`, `message` |
| `submission` | `run_id`, `stage`, `provider`, `idempotency_key`, `provider_job_id`, `submitted_at`, `state`     |
| `artifact`   | `stage`, `item_key`, `input_hash`, `recipe_hash`, `path`, `sha256`, `written_at`                 |
| bronze, silver, gold tables | fetched metadata, extracted trees, relations, assignments, chunks and vectors feeding `build` |

Publication is blocked while any expected `run_item` is `pending` or `failed`.
