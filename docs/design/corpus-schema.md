# Corpus schema version 6

Proposed exact template contract for `internal/corpusdb/schema.sql`, embedded by
`schema.go` in root module `danny.vn/trachuan`. The workshop keeps goose
migrations under `apps/corpus/migrations/`. Corpus databases are rebuilt, never
migrated in place. [Data](data.md) owns the overview; [build](index.md) owns
population and recovery. This template is not an applied migration. Version 3
adds membership class and validity corrections per
[scope adoption](scope-adoption.md); earlier dumps remain unchanged and require
their compatible reader. Version 4 adds the text source columns per
[text extraction](text-extraction.md). Version 5 fills the taxonomy tables and
adds node ordinals and counts per [taxonomy](taxonomy.md). Version 6 adds the
source-withdrawn columns per [pinned documents](pinned-documents.md).

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

`schema.go` exports `SchemaVersion = 6` and
`Apply(ctx context.Context, db DB, dimensions int) error`; DB exposes
`Exec(context.Context,string,...any)`. Its return values are
`(pgconn.CommandTag,error)`, matching pgx connections and transactions. The only
template substitution is the decimal vector dimension, validated against the
supported recipe table before substitution. Version 3 permits only the ADR 0009
dimension. Future dimensions require an explicitly supported recipe and
compatible schema version; never take SQL identifiers from JSON. Search reads
dimensions from `release.dense_recipe`, checks the actual column typmod, and
rejects a mismatch. No server dimension constant.

## Documents and source evidence

`document` columns:

- `id bigint PRIMARY KEY`; `country text CHECK(country = 'vn')`;
  `key text UNIQUE CHECK(key LIKE 'vn:%')`; `number text`; `number_norm text`;
  `title text`; `doc_type text`; `issuer text`.
- `class text NOT NULL CHECK(class IN ('primary','relation_context','secondary'))`
  and `source text NOT NULL CHECK(source IN ('vbpl','chinhphu'))`, no defaults,
  assigned per [scope adoption](scope-adoption.md#loading). Class describes
  inclusion, not legal type or ranking; its evidence is the final membership
  snapshot the inventory commits to.
- `issue_date date?`, `eff_from date?`, `eff_to date?`; `raw_issue_date text`,
  `raw_eff_from text`, `raw_eff_to text`.
- `status_code text`, `status_name text`, `status_class text` constrained to
  `in_force,partial,expired,not_yet,suspended,unknown`; `is_current boolean`.
  CHECK current equals status_class in (`in_force`,`partial`).
- `status_basis text` constrained to
  `source_status,derived_from_consolidated,derived_from_text,manual_correction`;
  `validity_conflict boolean`, with decisions in `validity_decision` per
  [validity corrections](validity-status.md).
  [Build normalization](normalization.md) owns assignment and derivation gaps.
- `text_source text NOT NULL CHECK(text_source IN ('source_tree','html_transcription','ocr','none'))`
  and `text_complete boolean NOT NULL`, no defaults, assigned per
  [text extraction](text-extraction.md#what-is-recorded). CHECK `text_complete`
  is false when `text_source = 'none'`; build also sets it false for a document
  with no provision. `status_code` and `status_name` retain the raw source
  values.
- `is_consolidated boolean`; `content_state text` in
  (`indexed`,`no_tree`,`no_chunks`,`embedding_gap`); `fetch_sha256 text`;
  `extract_sha256 text?`; `source_url text?`.
- `source_withdrawn_since date?` and `source_withdrawn_rel text?`: set together
  for a [pinned](pinned-documents.md) document, both null otherwise.
- CHECK `eff_to IS NULL OR eff_from IS NULL OR eff_to >= eff_from`.

Indexes: `document(number_norm)`, `document(status_class,eff_from,eff_to)`.
Number is not unique: distinct official documents can share a number. An
ambiguous number lookup must return all candidates or an explicit ambiguity,
never an arbitrary id. No title or issuer fuzzy identity matching.

`source_file`: `id bigint PRIMARY KEY`, `document_id bigint` FK document,
`source text`, `name text`, `url text?`, `hash text`, `size bigint >= 0`,
`role text` in (`main`,`appendix`,`scan`,`unknown`), `fetched_at timestamptz?`.
UNIQUE `(document_id,name,hash)`; index `document_id`. Unknown provenance stays
null or `unknown`. VBPL fetch artifacts do not reliably retain URL or role, and
no bundle carries fetch time (it comes from the
[provenance snapshot](scope-adoption.md#provenance-snapshot)): do not substitute
artifact adoption time or guess from a suffix. Record
`source_provenance_missing` once per document with the missing fields. `source`
follows the provider values in [data](data.md); `name`, `hash` and `size` come
from the verified fetched-file record, not a file suffix.

## Provisions and chunks

`provision`: `id bigint PRIMARY KEY`, `document_id bigint` FK document,
`parent_id bigint?`, `node_id text`, `kind text` in
(`phan`,`chuong`,`muc`,`dieu`,`khoan`,`diem`,`other`), `label text`,
`heading text`, `ordinal integer > 0`, `preorder integer > 0`, `path ltree`,
`text text`, `anchor text`, `locator text?`. `locator` is NULL exactly for
documents whose `text_source` is `source_tree` (build validates this); otherwise
it is `block:<first>-<last>`, the HTML block ordinals of the provision's
paragraphs, or `line:<first>-<last>` for OCR text ([OCR store](ocr-store.md)).
UNIQUE `(document_id,id)`, `(document_id,node_id)`, `(document_id,anchor)`,
`(document_id,preorder)`, `(document_id,path)`. Composite FK
`(document_id,parent_id)` to provision `(document_id,id)`; CHECK
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
settings in [budgets](budgets.md#indexing). No sparse index. `sparse` uses
`STORAGE MAIN`, so wide rows move `content` and `spans` out of line first and
the sparse scan reads the heap only
([retrieval performance](retrieval-performance.md#sparse-arm)); build metrics
report `sparse_out_of_line`, the chunks whose vector is still stored out of
line. Build validates actual token cap, finite components, vector norm and
sparse nonzero bound before insertion. The artifact ordinal remains unchanged
when oversize chunks are excluded, so corpus ordinals may have holes.

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

Kinds are the frozen source list's relation vocabulary plus `unknown`; the CHECK
permits
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

## Taxonomy tables

The build fills these tables from the taxonomy inputs ([taxonomy](taxonomy.md));
without inputs they stay empty and `release.taxonomy_version` is the empty
string. Seeds and manual entries are the only methods until the LLM layer lands.

`taxonomy_node`: `id bigint PRIMARY KEY`, `country text CHECK(country='vn')`,
`parent_id bigint?` FK taxonomy_node,
`level integer CHECK(level BETWEEN 1 AND 4)`, `code text UNIQUE`, `name text`,
`description text`, `origin text` in (`phapdien`,`custom`), `status text` in
(`active`,`retired`), `path ltree UNIQUE`,
`ordinal integer CHECK(ordinal >= 1)`,
`provision_count integer CHECK(provision_count >= 0)`,
`document_count integer CHECK(document_count >= 0)`. UNIQUE NULLS NOT DISTINCT
`(parent_id,ordinal)`. CHECK parent differs from id; indexes `(parent_id,code)`,
GiST `(path)`. The build validates parent level, path and country, and computes
the counts over the subtree's effective assignments. Retired codes are retained;
the import never reuses them.

`taxonomy_assignment`: `provision_id bigint` FK provision, `node_id bigint` FK
taxonomy_node, `family_id bigint` FK taxonomy_node, `method text` in
(`manual`,`phapdien_seed`,`llm`), `exclude boolean`,
`confidence double precision CHECK(confidence BETWEEN 0 AND 1)`,
`taxonomy_version text CHECK(taxonomy_version <> '')`. PK
`(provision_id,node_id)`; CHECK `NOT exclude OR method='manual'`; index
`(node_id,provision_id)`, `(provision_id,family_id)`. The build enforces the
family precedence in [data](data.md) in one transaction and validates family_id
as the node's level-2 ancestor. `coverage_gap` kind `seed_article_missing` names
a document found without a seeded article, and `override_target_missing` a
manual entry whose document or anchor is not in the release; both set `node_id`.

## Validity decisions

`validity_decision`: `document_id bigint NOT NULL REFERENCES document(id)`,
`ordinal smallint NOT NULL CHECK(ordinal BETWEEN 1 AND 8)`,
`layer text NOT NULL CHECK(layer IN ('source','A','B','C','manual'))`,
`rule_version text NOT NULL`, `before jsonb NOT NULL`, `after jsonb NOT NULL`,
`actor_key text?`, `effect_date date?`,
`relation_id bigint? REFERENCES relation(id)`, `anchor text?`,
`quote text? CHECK(char_length(quote) <= 400)`, `artifact_sha256 text?` with the
SHA-256 CHECK. PRIMARY KEY `(document_id,ordinal)`; ordinals are contiguous from
1 and the highest one decided the stored values. `before` and `after` are
objects with exactly the keys `status_class`, `is_current`, `status_basis`,
`eff_from`, `eff_to`, `validity_conflict`; dates are `YYYY-MM-DD` strings or
null. `actor_key` and `effect_date` are required for layers `B` and `C`,
`relation_id` for `C`. [Validity corrections](validity-status.md) owns the
rules.

## Release row

`release`: `singleton boolean PRIMARY KEY CHECK(singleton)`, `country text`
CHECK country='vn', `rel text UNIQUE`, `schema_version integer CHECK(=6)`,
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
Version 2 added `document.status_basis`. Version 3 adds class, source and
validity decisions. Version 4 adds `document.text_source`,
`document.text_complete` and `provision.locator`. Version 5 fills the taxonomy
tables and adds `taxonomy_node.ordinal`, `provision_count` and `document_count`.
Version 6 adds `document.source_withdrawn_since` and `source_withdrawn_rel`.
Always rebuild from preserved source artifacts, never backfill an old release
with an assumed value. Readers must support a version before a release of it is
activated, so the server deploys first; earlier dumps remain unchanged. The
server reads versions {4, 5, 6}: a version-4 release serves no nodes, and the
server reads the source-withdrawn columns only from version 6, since a version-5
release has none and no pinned documents. A newer server keeps serving the
previous release until a newer release is activated, and rolling back is
activating the previous release.

Host restore permits exactly `vector` and `ltree`, at the versions the
PostgreSQL image provides
([corpus release](corpus-release.md#activation-steps)).

First-party references, not fetched during this offline design:
[constraints](https://www.postgresql.org/docs/current/ddl-constraints.html),
[ltree](https://www.postgresql.org/docs/current/ltree.html),
[pgvector](https://github.com/pgvector/pgvector). The primary must verify pinned
extension support and restore behavior before implementation and deployment.
