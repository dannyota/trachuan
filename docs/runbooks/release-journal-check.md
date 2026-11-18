# Release journal check

The updater refuses a `release_journal` row, or a pointer or retired name, that
does not have its exact shape
([database roles](../design/system.md#database-roles)). That kind's phase then
fails on every run. The result shows `invalid release journal row` (step
`recovery`, `activation` or `prune`), or `invalid corpus database name` or
`invalid frameworks database name` (step `manifest`, `activation` or `prune`).
Treat it as a possible compromise of the server. These queries print counts
only, as `|`-separated lines. The first prints a line only for a kind that has
journal rows, so an empty result is normal for a kind that has none. Run them as
`trachuan`; the script connects as `auth_owner`, never as `postgres`.

Journal rows, latest per operation. After the kind comes the row count, and
every later number must be 0:

```sh
bash /srv/trachuan/host/auth-psql.sh <<'SQL'
WITH latest AS (
  SELECT DISTINCT ON (corpus_kind, operation_id)
    corpus_kind, operation_id, country, rel, mode, database_name,
    manifest_hash, dump_sha256, expected_active_db, step, manifest
  FROM public.release_journal
  WHERE operation_id IS NOT NULL
  ORDER BY corpus_kind, operation_id, id DESC
)
SELECT corpus_kind,
  pg_catalog.count(*) AS latest_rows,
  pg_catalog.count(*) FILTER (WHERE (operation_id ~ '^[0-9a-f]{32}$') IS NOT TRUE) AS bad_operation_id,
  pg_catalog.count(*) FILTER (WHERE (CASE corpus_kind WHEN 'law' THEN country ~ '^[a-z]{2}$' WHEN 'frameworks' THEN country ~ '^vn$' END) IS NOT TRUE) AS bad_country,
  pg_catalog.count(*) FILTER (WHERE (CASE corpus_kind WHEN 'law' THEN rel ~ '^[0-9]{8}(?:_[1-9][0-9]*)?$' WHEN 'frameworks' THEN rel ~ '^[0-9]{8}(?:_[0-9]{1,2})?$' END) IS NOT TRUE) AS bad_rel,
  pg_catalog.count(*) FILTER (WHERE (mode IN ('new', 'retained', 'prune')) IS NOT TRUE) AS bad_mode,
  pg_catalog.count(*) FILTER (WHERE (CASE corpus_kind WHEN 'law' THEN database_name ~ '^corpus_[a-z]{2}_[0-9]{8}(?:_[1-9][0-9]*)?$' WHEN 'frameworks' THEN database_name ~ '^frameworks_[0-9]{8}(?:_[0-9]{1,2})?$' END) IS NOT TRUE) AS bad_database_name,
  pg_catalog.count(*) FILTER (WHERE (manifest_hash ~ '^[0-9a-f]{64}$' OR (mode = 'prune' AND manifest_hash IS NULL)) IS NOT TRUE) AS bad_manifest_hash,
  pg_catalog.count(*) FILTER (WHERE (dump_sha256 ~ '^[0-9a-f]{64}$' OR (mode = 'prune' AND dump_sha256 IS NULL)) IS NOT TRUE) AS bad_dump_sha256,
  pg_catalog.count(*) FILTER (WHERE (expected_active_db IS NULL OR CASE corpus_kind WHEN 'law' THEN expected_active_db ~ '^corpus_[a-z]{2}_[0-9]{8}(?:_[1-9][0-9]*)?$' WHEN 'frameworks' THEN expected_active_db ~ '^frameworks_[0-9]{8}(?:_[0-9]{1,2})?$' END) IS NOT TRUE) AS bad_expected_active_db,
  pg_catalog.count(*) FILTER (WHERE (step IN ('started', 'creating', 'created', 'restoring', 'restored', 'validated', 'checked', 'activated', 'reverted', 'aborted', 'pruning', 'pruned')) IS NOT TRUE) AS bad_step,
  pg_catalog.count(*) FILTER (WHERE (pg_catalog.jsonb_typeof(manifest) = 'object' OR (mode = 'prune' AND (manifest IS NULL OR pg_catalog.jsonb_typeof(manifest) = 'null'))) IS NOT TRUE) AS bad_manifest
FROM latest
GROUP BY corpus_kind
ORDER BY corpus_kind;
SQL
```

Pointers and retired names. After the kind the numbers are pointer rows, bad
pointers, retired rows and bad retired names, so the second and fourth must be
0:

```sh
bash /srv/trachuan/host/auth-psql.sh <<'SQL'
WITH names AS (
  SELECT 'law'::text AS corpus_kind, 'pointer'::text AS source, active_db AS name FROM public.corpus_release
  UNION ALL SELECT 'frameworks', 'pointer', active_db FROM public.frameworks_release
  UNION ALL SELECT corpus_kind, 'retired', db_name FROM public.retired_release
), kinds (corpus_kind, pattern) AS (
  VALUES ('law'::text, '^corpus_[a-z]{2}_[0-9]{8}(?:_[1-9][0-9]*)?$'::text), ('frameworks', '^frameworks_[0-9]{8}(?:_[0-9]{1,2})?$')
)
SELECT k.corpus_kind,
  pg_catalog.count(*) FILTER (WHERE n.source = 'pointer') AS pointer_rows,
  pg_catalog.count(*) FILTER (WHERE n.source = 'pointer' AND (n.name ~ k.pattern) IS NOT TRUE) AS bad_active_db,
  pg_catalog.count(*) FILTER (WHERE n.source = 'retired') AS retired_rows,
  pg_catalog.count(*) FILTER (WHERE n.source = 'retired' AND (n.name ~ k.pattern) IS NOT TRUE) AS bad_retired_db_name
FROM kinds k
LEFT JOIN names n ON n.corpus_kind = k.corpus_kind
GROUP BY k.corpus_kind
ORDER BY k.corpus_kind;
SQL
```

An older updater's sessions search a schema named `activator` before `public`,
so one could hide what these queries read. This count must be 0:

```sh
bash /srv/trachuan/host/auth-psql.sh <<'SQL'
SELECT pg_catalog.count(*) AS activator_schema
FROM pg_catalog.pg_namespace
WHERE nspname OPERATOR(pg_catalog.=) 'activator';
SQL
```
