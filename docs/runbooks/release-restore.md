# Restore and select a corpus release

On the [production host](host.md), after first start.

## Release a corpus

On the MacBook, configure AWS CLI v2 profile `trachuan-release-upload` with the
OCI IAM user `trachuan-release-upload` customer secret key. Run
`aws configure --profile trachuan-release-upload` yourself; the credential
fields are `aws_access_key_id` and `aws_secret_access_key`. Region is
`ap-singapore-1`. The uploader uses that named shared-credentials profile and
supplies a temporary configuration with path-style S3 addressing. It never reads
or prints the keys. Set `OCI_S3_ENDPOINT` to the releases bucket's HTTPS S3
compatibility endpoint. Python 3.11 or later and signed-in `gh` are also
required.

Evaluate with
`corpus eval <cc> <rel> --out <directory> --previous <report.json>`, using the
report of the release active in production. Accept the run's reviewed gaps with
`corpus accept-gaps`. The upload requires completed quality checks, passing
gating comparisons in every cohort, and no research override.

Prepare a separate upload directory so the immutable build artifact stays
unchanged. Copy its dump and original manifest without regenerating the
manifest. With `CORPUS_DATABASE_URL` and `CORPUS_BUILDER_DATABASE_URL`
configured:

```sh
CC=vn
REL=20261114_3
BUILD=/path/to/verified-build-artifact
UPLOAD=/path/to/new-upload-directory
REPORT=/path/to/evaluation/report.json
mkdir "$UPLOAD"
cp "$BUILD/corpus.dump" "$BUILD/MANIFEST.sha256" "$UPLOAD/"
corpus release-row "$CC" "$REL" --out "$UPLOAD/release.json"
bash scripts/corpus-upload.sh --dry-run "$CC" "$REL" "$UPLOAD" "$REPORT"
bash scripts/corpus-upload.sh "$CC" "$REL" "$UPLOAD" "$REPORT"
```

Dry-run validates local files and prints the upload and dispatch plan without
calling AWS or GitHub. A real upload checks the prefix is empty, uploads all
four objects, verifies their sizes, then dispatches `corpus-release.yml` from
`main`. If dispatch alone fails, dispatch it manually with the same country and
release; do not upload again. A partial upload reserves that prefix; use a new
release id for a rebuilt corpus. The signed manifest authorizes release; the
report's `release_eligible` remains false. Host activation follows
[corpus release](../design/corpus-release.md#host-activation).

## Restore a corpus manually

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
