# Corpus schema migration

Part of [corpus schema](corpus-schema.md).

Restore requires the same PostgreSQL major and compatible extension versions
recorded in build metadata. Existing workshop artifacts remain authoritative;
adding the chunk projection must not rewrite ingest rows. A schema change
rebuilds a new corpus database; retain old artifacts for diagnosis and rollback.
Version 2 added `document.status_basis`. Version 3 adds class, source and
validity decisions. Version 4 adds `document.text_source`,
`document.text_complete` and `provision.locator`. Version 5 fills the taxonomy
tables and adds `taxonomy_node.ordinal`, `provision_count` and `document_count`.
Version 6 adds `document.source_withdrawn_since` and `source_withdrawn_rel`.
Version 7 adds the `text_source` value `pdf_text`
([PDF text layer](pdf-text.md)) and the taxonomy method `inherited`
([taxonomy](taxonomy.md#classification-layers)); no column changes. Always
rebuild from preserved source artifacts, never backfill an old release with an
assumed value. Readers must support a version before a release of it is
activated, so the server deploys first; earlier dumps remain unchanged. The
server reads versions {4, 5, 6, 7}: a version-4 release serves no nodes, the
server reads the source-withdrawn columns only from version 6 and later, since a
version-5 release has none and no pinned documents, and it serves `pdf_text` and
`inherited` values as stored. The order for a version change is in
[corpus release](corpus-release-changes.md#schema-version-changes). A newer
server keeps serving the previous release until a newer release is activated,
and rolling back is activating the previous release.

Host restore permits exactly `vector` and `ltree`, at the versions the
PostgreSQL image provides
([corpus release](corpus-release.md#activation-steps)).

First-party references, not fetched during this offline design:
[constraints](https://www.postgresql.org/docs/current/ddl-constraints.html),
[ltree](https://www.postgresql.org/docs/current/ltree.html),
[pgvector](https://github.com/pgvector/pgvector). The primary must verify pinned
extension support and restore behavior before implementation and deployment.

The version after 7 adds `document_summary`
([serving public summaries](web-public-documents-serving.md)). A release of an
earlier version stays a valid rollback target: it lacks the summary capability,
so summaries are unavailable while it is active and the reader is unaffected.
