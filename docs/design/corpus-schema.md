# Corpus schema version 3

Proposed exact template contract for `internal/corpusdb/schema.sql`, embedded by
`schema.go` in root module `danny.vn/trachuan`. The workshop keeps goose
migrations under `apps/corpus/migrations/`. Corpus databases are rebuilt, never
migrated in place. [Data](data.md) owns the overview; [build](index.md) owns
population and recovery. This template is not an applied migration. Version 3
adds membership class and validity corrections per
[scope adoption](scope-adoption.md); earlier dumps remain unchanged and require
their compatible reader.

## Conventions and identity

All columns below are NOT NULL unless marked `?`. All tables are in `public`.
IDs are `bigint`, assigned by build, with `CHECK(id > 0)`. Primary keys and
unique constraints create btree indexes. Foreign keys use NO ACTION, never
cascade. No user functions, triggers, executable defaults or stored questions.
Text equality uses deterministic `C` collation for identities and ordering.
SHA-256 text columns have `CHECK(value ~ '^[0-9a-f]{64}$')`.

Build assigns document ids by byte-sorted key; provision ids by document id and
tree preorder; chunk ids by document id and artifact ordinal. Other ids follow
their table's unique tuple in byte order. IDs are release-local. Public
provision identity is `(country,document.key,provision.anchor)`. Anchor is the
exact extraction `path`, including any `~N` suffix; never renumber it.

Install `vector` and `ltree` extensions through the local database template.
Each provision's ltree path is its actual parent chain of `n<preorder>` labels,
including itself; preorder is one-based within its document. It is an internal
ancestry index, not the public extraction anchor. This avoids interpreting
slashes or Vietnamese letters as ltree syntax. Taxonomy ltree labels are `n<id>`
along its parent chain.

`schema.go` exports `SchemaVersion = 3` and
`Apply(ctx context.Context, db DB, dimensions int) error`; DB exposes
`Exec(context.Context,string,...any)`. Its return values are
`(pgconn.CommandTag,error)`, matching pgx connections and transactions. The only
template substitution is the decimal vector dimension, validated against the
supported recipe table before substitution. Version 2 permits only the ADR 0009
dimension. Future dimensions require an explicitly supported recipe and
compatible schema version; never take SQL identifiers from JSON. Search reads
dimensions from `release.dense_recipe`, checks the actual column typmod, and
rejects a mismatch. No server dimension constant.

## Documents and source evidence

`document` columns:

- `id bigint PRIMARY KEY`; `country text CHECK(country = 'vn')`;
  `key text UNIQUE CHECK(key LIKE 'vn:%')`; `number text`; `number_norm text`;
  `title text`; `doc_type text`; `issuer text`.
- `class text CHECK(class IN ('primary','relation_context','secondary'))`,
  copied from final run membership, with no default. It describes inclusion, not
  legal type or ranking; supporting evidence stays in the build inventory.
- `issue_date date?`, `eff_from date?`, `eff_to date?`; `raw_issue_date text`,
  `raw_eff_from text`, `raw_eff_to text`.
- `status_code text`, `status_name text`, `status_class text` constrained to
  `in_force,partial,expired,not_yet,suspended,unknown`; `is_current boolean`.
  CHECK current equals status_class in (`in_force`,`partial`).
- `status_basis text` constrained to
  `source_status,derived_from_consolidated,derived_from_text,manual_correction`;
  `validity_conflict boolean`, with decisions in `validity_decision` per
  [validity corrections](validity-status.md).
  [Build normalization](index.md#normalization) owns assignment and derivation
  gaps. `status_code` and `status_name` retain the raw source values.
- `is_consolidated boolean`; `content_state text` in
  (`indexed`,`no_tree`,`no_chunks`,`embedding_gap`); `fetch_sha256 text`;
  `extract_sha256 text?`; `source_url text?`.
- CHECK `eff_to IS NULL OR eff_from IS NULL OR eff_to >= eff_from`.

Indexes: `document(number_norm)`, `document(status_class,eff_from,eff_to)`.
Number is not unique: distinct official documents can share a number. An
ambiguous number lookup must return all candidates or an explicit ambiguity,
never an arbitrary id. No title or issuer fuzzy identity matching.

`source_file`: `id bigint PRIMARY KEY`, `document_id bigint` FK document,
`source text`, `name text`, `url text?`, `hash text`, `size bigint >= 0`,
`role text` in (`main`,`appendix`,`scan`,`unknown`), `fetched_at timestamptz?`.
UNIQUE `(document_id,name,hash)`; index `document_id`. Unknown provenance stays
null or `unknown`. Existing fetch artifacts do not reliably retain URL, role or
fetch time: do not substitute artifact adoption time or guess from a suffix.
Record `source_provenance_missing` once per document with the missing fields.
`source` follows the provider values in [data](data.md); `name`, `hash` and
`size` come from the verified fetched-file record, not a file suffix.

## Provisions and chunks

`provision`: `id bigint PRIMARY KEY`, `document_id bigint` FK document,
`parent_id bigint?`, `node_id text`, `kind text` in
(`phan`,`chuong`,`muc`,`dieu`,`khoan`,`diem`,`other`), `label text`,
`heading text`, `ordinal integer > 0`, `preorder integer > 0`, `path ltree`,
`text text`, `anchor text`. UNIQUE `(document_id,id)`, `(document_id,node_id)`,
`(document_id,anchor)`, `(document_id,preorder)`, `(document_id,path)`.
Composite FK `(document_id,parent_id)` to provision `(document_id,id)`; CHECK
`parent_id IS NULL OR parent_id <> id`. Indexes:
`(document_id,parent_id,ordinal)`, GiST `(path)`. Build validates cycles, parent
path prefix and preorder before publication; cross-row ancestry is not expressed
as a CHECK constraint.

`provision_validity`: `id bigint PRIMARY KEY`, `provision_id bigint` FK
provision, `eff_from date?`, `eff_to date?`, `status_class text` with the
document status vocabulary, `evidence text CHECK(evidence <> '')`,
`source_url text`, `method text`, `observed_at timestamptz`. CHECK at least one
date is present and end is not before start. UNIQUE
`(provision_id,observed_at)`; index `(provision_id,eff_from,eff_to)`. Empty in
this release; relations alone never populate it.

`chunk`: `id bigint PRIMARY KEY`, `document_id bigint` FK document,
`provision_id bigint`, `ordinal integer >= 0`, `context_prefix text`,
`content text CHECK(content <> '')`, `spans jsonb` with array type check,
`input_sha256 text`, `token_count integer > 0`, `embedding vector(D)`,
`sparse sparsevec(S)`. D and S come from the supported dense and lexical
recipes. UNIQUE `(document_id,ordinal)`; composite FK
`(document_id,provision_id)` to provision `(document_id,id)`; indexes
`(provision_id)`, HNSW `(embedding vector_cosine_ops)` with construction
settings in [budgets](budgets.md#indexing). No sparse index. Build validates
actual token cap, finite components, vector norm and sparse nonzero bound before
insertion. The artifact ordinal remains unchanged when oversize chunks are
excluded, so corpus ordinals may have holes.

`lexical_df`: `token text PRIMARY KEY`, `dimension integer` constrained to
`1..S`, `df bigint > 0`. No index beyond the PK; collisions are permitted. Build
checks df <= chunk count. The complete encoding and statistics contract lives in
[lexical](lexical.md).

## Relations and gaps

`relation`: `id bigint PRIMARY KEY`, `source_document_id bigint?` FK document,
`target_document_id bigint?` FK document, `source_key text`, `target_key text?`,
`source_number text`, `target_number text`, `kind text`, `raw_code text`,
`raw_label text`, `source_provision_id bigint?`, `target_provision_id bigint?`,
`source_node_id text?`, `target_node_id text?`, `evidence text`, `method text`,
`artifact_sha256 text`, `ordinal integer >= 0`. UNIQUE
`(source_key,artifact_sha256,ordinal)`, because two documents can have
byte-identical relation artifacts. Composite FKs
`(source_document_id,source_provision_id)` and
`(target_document_id,target_provision_id)` reference provision
`(document_id,id)`. CHECK a non-null provision id requires its document id.
CHECK `target_key IS NOT NULL OR target_number <> '' OR kind = 'unknown'`.
Indexes on source and target document ids, and `target_key`.

Kinds are the frozen source list's relation vocabulary plus `unknown`; version 2
CHECK permits
`repeals,implements,legal_basis,cites,suspends, consolidates,guided_by,amends_supplements,replaces,references,unknown`.
Keep both source and target keys even when either FK is null. Resolve only exact
source IDs, or unique normalized number when the target has no ID. Never replace
an explicit missing ID with a number match. Keep unknown node references in the
raw fields and emit `unresolved_provision_relation`. Copy `source_number` from
the source's frozen fetch artifact even if its corpus row is excluded; use empty
only when unavailable, with a provenance gap.

Preserve source-to-target direction as supplied by `relate`; do not infer an
inverse legal fact from title wording. Incoming edges can say "amended by X"
when an `amends_supplements` source actually points at this document; outgoing
unresolved edges keep target number and evidence with "not in corpus". Show
unresolved source/target identity and raw evidence when direction is not
verified. Missing endpoints do not erase a relation or make it proof of repeal.
Consolidation families are diagnostic connected components of resolved
`consolidates` edges, not evidence of aligned or equivalent provisions.

`coverage_gap`: `id bigint PRIMARY KEY`, `kind text`, `document_id bigint?` FK
document, `document_key text?`, `node_id bigint?` FK taxonomy_node,
`stage text`, `item_key text`, `detail text`, `message text`,
`accepted_at timestamptz?`, `note text`. UNIQUE `(stage,item_key,kind,detail)`;
indexes `(kind,id)`, `document_id`, `node_id`. Missing documents are represented
by `document_key`, not fake document rows. `node_id` here means taxonomy node,
not a VBPL source node. Global gaps may have neither document nor taxonomy node.

## Empty taxonomy tables

Create these tables but insert no rows. `release.taxonomy_version` is the empty
string. Classification, seeds and subject filtering belong to later work.

`taxonomy_node`: `id bigint PRIMARY KEY`, `country text CHECK(country='vn')`,
`parent_id bigint?` FK taxonomy_node,
`level integer CHECK(level BETWEEN 1 AND 4)`, `code text UNIQUE`, `name text`,
`description text`, `origin text` in (`phapdien`,`custom`), `status text` in
(`active`,`retired`), `path ltree UNIQUE`. CHECK parent differs from id; indexes
`(parent_id,code)`, GiST `(path)`. Parent level/path and country are validated
on future import. Retired codes are retained; the import never reuses them.

`taxonomy_assignment`: `provision_id bigint` FK provision, `node_id bigint` FK
taxonomy_node, `family_id bigint` FK taxonomy_node, `method text` in
(`manual`,`phapdien_seed`,`llm`), `exclude boolean`,
`confidence double precision CHECK(confidence BETWEEN 0 AND 1)`,
`taxonomy_version text CHECK(taxonomy_version <> '')`. PK
`(provision_id,node_id)`; CHECK `NOT exclude OR method='manual'`; index
`(node_id,provision_id)`, `(provision_id,family_id)`. Future importer must
enforce the family precedence in [data](data.md) in one transaction and validate
family_id as the node's level-2 ancestor. Empty tables do not claim that
precedence is implemented or that taxonomy gates passed.

## Release row

`release`: `singleton boolean PRIMARY KEY CHECK(singleton)`, `country text`
CHECK country='vn', `rel text UNIQUE`, `schema_version integer CHECK(=2)`,
`taxonomy_version text`, `dense_recipe jsonb`, `chunk_recipe jsonb`,
`lexical_recipe jsonb`, `retrieval_recipe jsonb`, `lexical_stats jsonb`,
`source_manifest_hash text`, `build_input_hash text`, `build_recipe_hash text`,
`evaluation_summary jsonb`, `built_at timestamptz`. Every jsonb column has an
object type check. Exactly one row is a build invariant; singleton CHECK
prevents more than one. Recipes contain versioned values and hashes, not only
opaque algorithm names. `dense_recipe` is the compact JSON object
`{version:1,model,dimensions,document,document_hash,query,query_hash}`, where
`document` and `query` are the embedder recipe reports of the two runtimes
(`internal/embedding.Recipe`) and the hashes are their `Hash()`; search compares
`query_hash` and the full `query` report with the running query embedder, and
`dimensions` with the column typmod. `source_manifest_hash` hashes the frozen
upstream inventory defined in [build](index.md).

`evaluation_summary` is initially `{status:"not_run",release_eligible:false}`.
Eval reports are immutable sidecars referencing the dump hash; eval does not
update the dump. The later host/release design must bind the passed report to
that exact dump in the signed manifest, with the producer and verifier changed
together. No release exists in this work, and `not_run` cannot authorize one.

## Migration and dependencies

Restore requires the same PostgreSQL major and compatible extension versions
recorded in build metadata. Existing workshop artifacts remain authoritative;
adding the chunk projection must not rewrite ingest rows. A schema change
rebuilds a new corpus database; retain old artifacts for diagnosis and rollback.
Version 2 adds `document.status_basis`; rebuild from preserved source artifacts,
never backfill old releases with an assumed derivation. Readers must support
this version before activation. Existing version 1 dumps remain unchanged.

The host restore policy currently permits only pgvector extensions, while the
existing data design already requires ltree. This work uses both locally; host
activation remains blocked until its producer/verifier design explicitly handles
ltree. Do not silently widen the host allowlist in implementation.

First-party references, not fetched during this offline design:
[constraints](https://www.postgresql.org/docs/current/ddl-constraints.html),
[ltree](https://www.postgresql.org/docs/current/ltree.html),
[pgvector](https://github.com/pgvector/pgvector). The primary must verify pinned
extension support and restore behavior before implementation and deployment.
