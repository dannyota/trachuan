# Restore a corpus manually

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

`auth` belongs to `auth_owner`, so read and write it as that role, never as
`postgres` ([database roles](../design/system.md#database-roles)):

```sh
bash /srv/trachuan/host/auth-psql.sh -v database="$DB" <<'SQL'
INSERT INTO public.corpus_release (country, active_db, activated_at, activated_by)
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

Prune drops databases the host retired itself, seven days later, unless the
tables still protect them; others are yours to drop. Separately, each activation
drops every `activator`-owned database that neither the tables nor the host
[protect](../design/activation-record.md#drops), and ends other `activator`
sessions, so a manual `activator` prewarm running then is ended. After a manual
swap to a pipeline release, re-sign that release the same day so the host adopts
it. A database restored by hand that never became active through the pipeline,
such as `corpus_vn_20261112_2`, has no row there and stays until it is dropped
once by hand. Confirm it is not `active_db`, not in `retired_release`, and has
no connections, then drop it:

```sh
DB=corpus_vn_20261112_2
bash /srv/trachuan/host/auth-psql.sh -v db="$DB" <<'SQL'
SELECT (SELECT count(*) FROM public.corpus_release WHERE active_db = :'db'),
       (SELECT count(*) FROM public.retired_release WHERE db_name = :'db'),
       (SELECT count(*) FROM pg_catalog.pg_stat_activity WHERE datname = :'db');
SQL
podman exec trachuan-postgres /usr/local/bin/postgres-admin.sh dropdb -U postgres "$DB"
```

Drop only when all three counts are 0. The release replaced by the first
pipeline activation (`corpus_vn_20261114_3`) has a row with a NULL digest and no
host record, so prune leaves it: drop it the same way once its seven days have
passed.

## Prewarm a corpus database

The updater prewarms a release when it activates it and the active database
after a PostgreSQL restart. By hand, for `$DB` before Caddy starts, use the path
that matches how the database was restored; the rule is in
[trust in the dump](../design/corpus-release-activation.md#trust-in-the-dump).
Either reads every table, index and TOAST relation (3.6 GB for vn/20261112_2)
into the 4 GiB buffer cache. Record the printed time and the first search's
latency.

Restored by the updater: run as `activator`, never as `postgres`. A database
activated before that rule has no `pg_prewarm`; the command then fails and the
cache refills from traffic and the page cache.

```sh
podman exec -i trachuan-postgres sh -c \
  'PGPASSWORD=$(cat /run/secrets/activator_db_password) \
   PGOPTIONS="-c search_path=pg_catalog" \
   exec psql -X -v ON_ERROR_STOP=1 -h 127.0.0.1 -U activator -d "$1"' sh "$DB" <<'SQL'
\timing on
SELECT pg_catalog.sum(public.pg_prewarm(c.oid::pg_catalog.regclass)) AS blocks
FROM pg_catalog.pg_class c
JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname IN ('public', 'pg_toast') AND c.relkind IN ('r', 'i', 't')
  AND pg_catalog.has_table_privilege(c.oid, 'SELECT');
SQL
```

After a [manual restore](#restore-a-corpus-manually) of a dump you trust, as
`postgres`, with a `$DB` you typed yourself, never one read from `auth`; the
extension is dropped so the release keeps no extra object:

```sh
podman exec -i --env PGOPTIONS='-c search_path=pg_catalog' trachuan-postgres \
  /usr/local/bin/postgres-admin.sh \
  psql -X -v ON_ERROR_STOP=1 -U postgres -d "$DB" <<'SQL'
CREATE EXTENSION pg_prewarm WITH SCHEMA public;
\timing on
SELECT pg_catalog.sum(public.pg_prewarm(c.oid::pg_catalog.regclass)) AS blocks
FROM pg_catalog.pg_class c
JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname IN ('public', 'pg_toast') AND c.relkind IN ('r', 'i', 't');
DROP EXTENSION pg_prewarm;
SQL
```
