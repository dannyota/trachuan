# Restore and select a corpus release

On the [production host](host.md), after first start.

Use a trusted, evaluated PostgreSQL 18 dump whose recorded checksum the primary
has verified. Stop on unexpected functions, event triggers or extensions in the
TOC. Create a fresh database; do not overwrite an existing release:

```sh
systemctl --user stop trachuan-caddy
DUMP=/path/to/verified/corpus.dump
DB=corpus_vn_20261112_2
podman exec -i trachuan-postgres pg_restore --list <"$DUMP" >/tmp/corpus-toc.txt
```

Inspect `/tmp/corpus-toc.txt` before continuing. The restore uses 2 GB and two
maintenance workers because 512 MB spilled in the measured HNSW rebuild:

```sh
podman exec trachuan-postgres /usr/local/bin/postgres-admin.sh createdb -U postgres -O postgres "$DB"
podman exec -i \
  --env 'PGOPTIONS=-c maintenance_work_mem=2GB -c max_parallel_maintenance_workers=2' \
  trachuan-postgres /usr/local/bin/postgres-admin.sh pg_restore -U postgres \
  --exit-on-error --no-owner --no-privileges --dbname "$DB" <"$DUMP"
podman exec trachuan-postgres bash /usr/local/bin/grant-corpus.sh "$DB"
```

[Prewarm](#prewarm-a-corpus-database) `$DB`, then select it:

```sh
podman exec -i trachuan-postgres /usr/local/bin/postgres-admin.sh \
  psql -X -v ON_ERROR_STOP=1 -U postgres -d auth -v database="$DB" <<'SQL'
INSERT INTO corpus_release (country, active_db, activated_at, activated_by)
VALUES ('vn', :'database', now(), 'owner')
ON CONFLICT (country) DO UPDATE SET active_db = EXCLUDED.active_db,
activated_at = EXCLUDED.activated_at, activated_by = EXCLUDED.activated_by;
SQL
curl --fail http://127.0.0.1:8080/readyz
systemctl --user start trachuan-caddy
```

Start Caddy only after readiness passes. On a recipe mismatch, keep traffic
stopped and select the previous compatible database or matching serving bundle.
Keep the previous database. This manual phase does not implement signed release
activation. PostgreSQL has 3 GiB of shared-memory capacity within its existing 8
GiB memory cap; shared memory is charged to that cap, not an extra allowance.

Rollback to a hand-restored release within its seven days is the owner's manual
compare-and-swap of `corpus_release.active_db`.

## Prewarm a corpus database

Run this for `$DB` before Caddy starts: after a restore, and after every
PostgreSQL restart (a digest deploy) for the active database. It reads every
table, index and TOAST relation (3.6 GB for vn/20261112_2) into the 4 GiB buffer
cache, then drops the extension so the release keeps no extra object. Record the
printed time and the first search's latency.

```sh
podman exec -i trachuan-postgres /usr/local/bin/postgres-admin.sh \
  psql -X -v ON_ERROR_STOP=1 -U postgres -d "$DB" <<'SQL'
CREATE EXTENSION pg_prewarm;
\timing on
SELECT sum(pg_prewarm(c.oid)) AS blocks FROM pg_class c
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname IN ('public', 'pg_toast') AND c.relkind IN ('r', 'i', 't');
DROP EXTENSION pg_prewarm;
SQL
```
