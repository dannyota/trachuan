# Release journal check

The updater refuses a `release_journal` row, or a pointer or retired name, that
does not have its exact shape
([database roles](../design/system.md#database-roles)). That kind's phase then
fails on every run. The result shows `invalid release journal row` (step
`recovery`, `activation` or `prune`), or `invalid corpus database name` or
`invalid frameworks database name` (step `manifest`, `activation` or `prune`).
Treat it as a possible compromise of the server. These queries print counts
only. Run them as `trachuan`, connected as `auth_owner`, never as `postgres`:

```sh
auth_check() {
  podman exec -i trachuan-postgres sh -c 'export PGPASSWORD
    PGPASSWORD=$(cat /run/secrets/auth_db_password)
    exec env PGREQUIREAUTH=scram-sha-256 PGOPTIONS="-c search_path=pg_catalog" \
      psql --no-password -X -v ON_ERROR_STOP=1 -h 127.0.0.1 -U auth_owner -d auth'
}
```

Every `bad_*` column must be 0. Journal rows, latest per operation:

```sh
auth_check <<'SQL'
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

Pointers and retired names:

```sh
auth_check <<'SQL'
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

A schema named `activator` would be searched before `public` by the updater's
sessions and hide what these queries read, so this count must be 0:

```sh
auth_check <<'SQL'
SELECT pg_catalog.count(*) AS activator_schema
FROM pg_catalog.pg_namespace
WHERE nspname OPERATOR(pg_catalog.=) 'activator';
SQL
```
