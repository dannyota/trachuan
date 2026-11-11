# Index construction

Proposed baseline implementation under ADRs
[0009](../adr/0009-dense-embedding-recipe.md) and
[0010](../adr/0010-retrieval-evaluation-for-agents.md). The sequence is `chunk`,
`embed`, `build`, `eval` after ingest. The contracts are [chunk](chunk.md),
[embedding](embedder.md), [corpus schema](corpus-schema.md),
[lexical encoding](lexical.md), [retrieval recipe](retrieval-recipe.md) and
[evaluation](evaluation.md). Limits have one home in [budgets](budgets.md).

This work builds recipe 1 before tuning, on our document/provision identities.
Taxonomy import, seeds, subject-filter ordering, new development cases,
tuning experiments, signing, upload and activation remain separate work.
Taxonomy tables are empty and `taxonomy_version` is empty. A build is local
evidence, not permission to publish. The host work designs manifest production
and verification together; no partial signed-manifest contract is introduced.

## Build stage

`apps/corpus/internal/stage/build/` separates `build.go`, `inventory.go`,
`normalize.go`, `load.go`, `database.go`, `dump.go`, `resolve.go`. Reusable
schema lives in `internal/corpusdb`, retrieval in `internal/retrieval` and
evaluation in `apps/corpus/internal/evaluation`. The stage uses the existing
`Stage` interface; `Rows` does not create workshop vector or corpus tables.

Plan one item keyed `<country>/<rel>`. Refuse planning while any predecessor
item is pending or failed. Gap items are allowed for diagnostic builds and
remain visible; release still requires acceptance. Read one consistent workshop
snapshot to produce the upstream inventory defined in
[scope adoption](scope-adoption.md#inventory): every predecessor item through
`embed`, expected keys, the source-list hash and gaps. Gap acceptance is a
release gate, not a build input, so it is never hashed. `Process` recomputes the
input hash from the inventory and pinned map and fails with non-retryable
`inventory_changed` when it differs from the planned input hash. Do not hash
workshop sequence IDs, mutable paths or generation numbers.

For inventory diagnostics, derive `identity_inventory_hash` from the identity
projection in [scope adoption](scope-adoption.md#inventory). This hash does not
bind the identity map or gate planning.
[Evaluation](evaluation.md#fixtures-and-identity-adjudication) owns map
production, review and per-case identity validation. Recipe freezing follows the
[pipeline snapshot](pipeline.md#shape); ingest can finish before index recipes
are appended. The reviewed map is input data, not a recipe.

Freeze the evaluation civil date from the first eight digits of `rel`, parsed as
a real calendar date; an invalid date is a configuration failure. The same date
feeds calibration and eval, including retries after midnight.

Build input hash is
`stage.RecipeHash("build-input",label,country,rel, source_manifest_hash,decimal_schema_version,identity_map_sha256)`,
label per inventory version. Pin the reviewed map bytes at planning and verify
their SHA-256 before use. Changing the map changes build input and invalidates
dependent evaluation, without changing a frozen build recipe. Never read changed
map bytes under the planned input hash. Verify all referenced artifact hashes
and safe paths before decoding payloads; check byte sizes where declared, and
before decoding stat each artifact file against the per-file cap in
[budgets](budgets.md#indexing); a file above it fails the item. Artifacts are
immutable, content addressed and written by our own stages, so a file growing
between the size check and the read is outside the threat model; the manifest
hash still guards the bytes decoded. Never use the workshop's latest row for a
document outside this inventory.

Build recipe hash uses `stage.RecipeHash("build","corpus/1",recipe_json)`. Its
fixed fields are `schema_version`, `schema_sha256`, `chunk_recipe_hash`,
`dense_recipe`, `lexical_recipe`, `retrieval_policy`, `normalizer_version`,
`status_codes_sha256`, `validity_rules_version`, `calibration_policy`,
`legacy_cases_sha256`, PostgreSQL major, extension versions and tool versions.
No identity map hash is a recipe field. The calibration policy is fixed before
build; its numeric threshold is recorded in the final retrieval recipe.
Source-list status codes come from the run snapshot. Changing any of these
requires a new run recipe, not modification of a frozen run. The date is
run-specific input derived from `rel`, not a value read inside
`Recipe(Snapshot)`. Keep fixture bytes and runtime reports pinned by their
recipe hashes; never read a newly edited fixture under an old frozen hash.

## Inclusion and losses

- Include one metadata document per final active member with a done fetch. Class
  and source follow [scope adoption](scope-adoption.md#loading). Include
  expired, suspended, not-yet and unknown statuses so historical retrieval can
  work. Include no-tree documents as metadata with coverage gaps; they have no
  provisions or chunks. Keep all valid extracted provisions even when their
  chunks were omitted or rejected.
- Exclude `withdrawn`, `scope_removed`, `source_replaced` and fetch-gap
  documents from searchable documents. Preserve keys and gap details in
  `coverage_gap`; do not carry stale text from a previous run. English
  renditions never enter the expected set. References to excluded documents
  remain unresolved.
- Insert only chunks with a matching successful vector and verified input hash.
  Require `chunk count = embedded count + rejected count` before exclusions.
  Require inserted chunk count equals inserted dense vector count, with one
  sparse vector per inserted chunk. No zero-vector placeholder. A document with
  all vectors rejected is `embedding_gap`; one with no emitted chunks is
  `no_chunks`; `no_tree` takes precedence; otherwise `indexed`.
- Project every upstream gap and every chunk omission into coverage metadata.
  Unknown relation endpoints remain in `relation`, including edges whose source
  is excluded. Do not discard a fact merely because the target lacks text.

An extract source gap leaves a metadata-only `no_tree` document. Invalid
ancestry is such a gap: retain the raw artifact for diagnosis, not invalid
provision rows. A chunk source gap leaves valid provisions with `no_chunks`. A
missing done embed artifact is not a vector rejection; preserve its stage gap
and classify the document `embedding_gap`. Reconciliation counts name these
unavailable artifacts separately from per-chunk rejections.

This favors honest coverage over inflated searchable counts. Reports list
excluded keys, missing citation numbers and rejected ordinals. Nothing from the
workshop is deleted when a corpus excludes it.

## Database lifecycle

Final name is `corpus_<cc>_<rel>` using the existing two-lowercase-letter
country and `[0-9]{8}(_[0-9]{1,2})?` release grammar. Temporary name is
`corpus_tmp_<cc>_<first-24-hex-of-build-input-hash>`. Quote identifiers through
pgx identifier handling; never use shell interpolation. The full input/recipe
hashes and run key form a database comment ownership marker. A prefix collision
with a different full marker fails; never adopt it.

The current `pipeline` role lacks CREATEDB. Proposed owner approval: add a local
`corpus_builder` login with CREATEDB, NOSUPERUSER, NOCREATEROLE, owning only
build databases, and no auth/workshop privileges. The worker retains its
separate `pipeline` connection for workshop rows. PostgreSQL CREATEDB is not
name-scoped; therefore keep this credential local and enforce name/ownership
guards in the builder. Local bootstrap provisions a template database with
`vector` and `ltree`; build clones it, never needs superuser to install them. Do
not widen `pipeline` or reuse host activation credentials.

Every database mutation (create, load, dump, restore, rename) runs under a
session advisory lock keyed by country and rel on the builder's maintenance
connection. `Process` and `EnsureDatabase` each take it for their own duration
and re-validate database state under it; no lock spans two calls, and a lost
connection releases the lock and fails the operation. Check run generation
before expensive work and before publication. A stale attempt may leave an
orphan database but cannot commit its run item. A retry adopts a valid existing
build artifact first. Without an artifact, recreate only a temporary database
bearing this exact ownership marker and no foreign connections. A mismatched
final database is an error, never an automatic drop.

Populate template tables, normalize, copy rows in stable order, train BM25 in
two streaming passes, build HNSW with `max_parallel_maintenance_workers=0`,
ANALYZE, and validate counts and constraints. Use a transaction-local staging
table for chunks until sparse encoding is ready; final NOT NULL constraints must
never require dummy vectors or a weakened shared schema. Calibrate the
provisional threshold using the shared evaluation helper on this temporary
database as specified in [evaluation](calibration.md). The helper takes explicit
recipes and does not require a published release row. Insert the final release
row with that threshold; then prohibit further writes. Dump before renaming,
using subprocess arguments, not a shell:
`pg_dump -Fc --no-owner --no-privileges --no-comments --dbname <temporary-db>`;
the corpus schema carries no comments, and dropping them keeps extension
comments, which a nonsuperuser cannot restore, out of the dump. Database
identity never comes from a request body or a dump's embedded name.

## Build artifact and resolve

Publish `corpus.dump`, `build.json`, `inputs.json`, `calibration.json`,
`validity-review.json` and `MANIFEST.sha256` through the existing immutable
artifact writer. `inputs.json` is the upstream inventory. `build.json` fields:
`version`, `country`, `rel`, `database_name`, `schema_version`,
`build_input_hash`, `build_recipe_hash`, `source_manifest_hash`,
`identity_inventory_hash`, `identity_map_sha256`, all four recipes,
`lexical_stats`, `taxonomy_version`, `counts` per table and exclusion reason,
`dump:{sha256,bytes,format:"custom"}`, `postgres_version`, `pgvector_version`,
`ltree_version`, `pg_dump_version`, `worker_version`, `built_at`,
`durations_ms`, `relation_bytes`, `index_bytes`. No credentials, file system
paths, user questions or host signatures.

`calibration.json` holds the reproducible scores and selected threshold, linked
to fixture hashes. It uses case IDs only, not question text. The dump and build
record must agree on every release field. Dump byte identity, not a later
re-dump, is the future upload candidate. Restore can rebuild ANN index pages, so
eval records and measures the restored database too.

`Resolve` verifies the entire artifact, then calls the exported idempotent
`EnsureDatabase`, which ensures the final database exists: if absent, create the
owned temporary database from `corpus_template` (extensions already installed),
list the dump's table of contents with `pg_restore -l`, drop its `EXTENSION`
entries, refuse any other extension, function or event trigger entry, restore
the rest as `corpus_builder` with
`pg_restore --no-owner --no-privileges --exit-on-error --use-list`, validate
release fields and counts, disconnect builder sessions, and rename it. The eval
stage and `corpus eval` reach it through `EnsureForRun`, which looks up the
run's committed done build artifact and calls `EnsureDatabase` with it.
`EnsureDatabase` itself takes a verified artifact, not a run row, because
`Resolve` runs before the item and its artifact row commit; so a database lost
after evaluation returns through `corpus eval` without rerunning the run.
Integration tests prove dump and restore under a nonsuperuser builder role whose
template is provisioned the same way. After a fresh build, the same resolve path
renames the already validated temporary database. If final exists, require its
ownership marker, release hashes and counts to match. If a crash left no marker
after restore, only the owned temporary database may be recreated. Never rewrite
a final database to repair a mismatch.

No SQL transaction spans CREATE DATABASE, restore or rename. Artifact
publication precedes workshop commit, so recovery after a lost workshop or
database can adopt the dump without re-embedding. `Resolve` returns run gaps
only after this idempotent database step. Check cancellation during copy, index
creation, dump and restore; heartbeats continue throughout. Failed
infrastructure leaves the item failed, not an owner-acceptable data gap.

## Release order and evidence

Update the shared normalizer and schema readers, reuse compatible chunk and
embed artifacts, then build and evaluate with the revised map and validity
contracts. Frozen recipes and published databases stay immutable; use a new run
when an included recipe changes. Retain old artifacts for rollback. Reprocessing
uses the existing budgets and services; measure its time, disk and new coverage
before release.

Implement shared schema and lexical contracts, chunk, service and embed, shared
retrieval, calibration helper and build, then eval adapters. The primary can
split this dependency order into tasks; no implementation is authorized by this
proposal alone. Required checks include manifest corruption, lost local DB,
restored workshop, crash before/after rename, stale generation, hostile names,
missing endpoints, all-vector-rejected document, empty taxonomy and counts.

Owner approval is needed for the local CREATEDB role and the proposed resource
ceilings after local measurements. Accept the baseline's explicit short-text and
duplicate-citation losses, and family collapse remaining off until alignment is
evidenced, with the design. Source-date boundary verification and the host ltree
allowlist remain release blockers, not silent implementation choices. Host
policy, taxonomy, new cohorts and tuning follow separately. External command
behavior is not verified in this network-disabled design; the primary checks the
pinned tools against
[pg_dump](https://www.postgresql.org/docs/current/app-pgdump.html),
[pg_restore](https://www.postgresql.org/docs/current/app-pgrestore.html) and
[CREATE ROLE](https://www.postgresql.org/docs/current/sql-createrole.html).
