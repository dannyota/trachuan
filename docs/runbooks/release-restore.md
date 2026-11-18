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
compatibility endpoint. Python 3.11 or later, `flock`, `curl`, `oras`, signed-in
`gh` and `cosign` v3.1.4 are also required. The uploader runs
`cosign verify-blob` to verify the production and previous releases' signed
manifests. On macOS, install `flock` with `brew install flock` if absent
([Homebrew](https://formulae.brew.sh/formula/flock)). The uploader prints its
AWS CLI version. The workflow installs and requires AWS CLI 2.40.0 and prints
its version.

A release compares with the production release from the moment its run is
created. Before the first `corpus start` with `--previous-report`, restart the
corpus worker on code that has the flag, so no older worker picks up the run.
Pass the uploaded `report.json` of the production release, exactly as its signed
`<cc>-production` manifest binds it. Never pass a stage evaluation artifact from
the artifact store. `corpus start` freezes that choice as the
[selected report](../design/release-evidence.md#selected-report) decision; a
different previous release needs a new run and `rel`.

When the run is `evaluated`, accept its reviewed gaps with `corpus accept-gaps`.
The report to upload is `report.json` of the run's `eval` artifact. A
`corpus eval` report cannot publish. The upload requires completed quality
checks, passing required gating comparisons in every product cohort with cases,
and no research override. All cohorts must be present; empty cohorts require no
comparisons.

Prepare a separate upload directory so the immutable build artifact stays
unchanged. Copy its dump, original manifest and build record without
regenerating the manifest. With `CORPUS_DATABASE_URL`,
`CORPUS_BUILDER_DATABASE_URL` and an absolute `CORPUS_ARTIFACTS` configured:

```sh
CC=vn
REL=20261120_1
BUILD=/path/to/verified-build-artifact
UPLOAD=/path/to/new-upload-directory
PREVIOUS=/path/to/production-upload-directory/report.json
corpus start "$CC" "$REL" --previous-report "$PREVIOUS"
```

After the run is `evaluated`:

```sh
ARTIFACT=$(psql "$CORPUS_DATABASE_URL" -At -v cc="$CC" -v rel="$REL" <<'SQL'
SELECT a.path FROM run r
JOIN run_item i ON i.run_id = r.id
JOIN artifact a ON a.id = i.artifact_id
WHERE r.country = :'cc' AND r.rel = :'rel'
  AND i.stage = 'eval' AND i.state = 'done';
SQL
)
REPORT=$CORPUS_ARTIFACTS/$ARTIFACT/report.json
mkdir "$UPLOAD"
cp "$BUILD/corpus.dump" "$BUILD/MANIFEST.sha256" "$BUILD/build.json" "$UPLOAD/"
corpus release-row "$CC" "$REL" --out "$UPLOAD/release.json"
bash scripts/corpus-upload.sh --dry-run "$CC" "$REL" "$UPLOAD" "$REPORT"
bash scripts/corpus-upload.sh "$CC" "$REL" "$UPLOAD" "$REPORT"
```

The older path is for `vn/20261118_1` only, whose run predates this rule: it has
no decision, so evaluate with
`corpus eval <cc> <rel> --out <directory> --previous <report.json>`, using the
production release's uploaded report, and upload that report. Its `release.json`
has no selected report hashes; upload refuses that shape for every other
release.

Until the sidecar gate ships, upload and signing refuse a release whose
`release.json` carries the selected report hashes, so no prefix is spent on a
release that cannot be signed.

The upload refuses unless production is settled: the report's previous `rel` and
SHA-256 must equal `rel` and `report.sha256` of the verified `<cc>-production`
manifest, and `release` of `https://trachuan.vn/api/v1/public/status` must be
that `rel`. A release still activating, waiting on recipes, failed or rolled
back by hand blocks it. Only `vn` can upload until that endpoint takes a
country. If production has moved on, the release needs a new run and `rel`.

Dry-run validates local files and prints the upload and dispatch plan without
calling AWS or GitHub. A real upload holds
`${HOME}/.cache/trachuan/corpus-upload.lock`, shared by all checkouts for the
same owner. The uploader creates the cache directory with mode `0700`, checks
the prefix is empty, uploads every object (`build.json` too from schema 8), then
rechecks every size and ETag against its upload receipt. It keeps the uploaded
report as `$UPLOAD/report.json` before dispatching `corpus-release.yml` from
`main`. Use that file as `--previous-report` for the next release after this
release becomes active. If dispatch alone fails, dispatch it manually with the
same country and release; do not upload again. A partial upload reserves that
prefix; use a new release id for a rebuilt corpus. The signed manifest
authorizes release; the report's `release_eligible` remains false. Host
activation follows
[corpus release](../design/corpus-release.md#host-activation).

Signing repeats the production comparison before it signs or pushes anything, so
a refusal leaves no tag and no signature. A release whose `<cc>-<rel>` manifest
already exists with a sequence at or below production's is exempt, which keeps
retry and rollback working. Dispatching the same release again retries a failed
activation. Signing an existing `<cc>-<rel>` again keeps every field of its
signed manifest except `commit`, `sequence` and `heading_review`, and refuses
before signing when the bucket's objects would change one. A release without
selected report hashes below `vn/20261118_1` signs only when its manifest
already exists.

When an earlier run pushed the release but failed to move `<cc>-production`, the
comparison applies to the retry only while production's sequence is below the
pushed one. Once another release holds production with a higher sequence, the
pushed release is exempt too: signing it moves the tag, the host refuses it
because its `previous` is not the active release, and uploads stay blocked.
Recover by signing the active release again:

```sh
gh workflow run corpus-release.yml --ref main -f country=vn -f rel=<active rel>
```

### First release with reviewed knowledge

Before uploading the first release whose `sources/<cc>-knowledge.yaml` holds a
`provision_effect` or `relation` entry, confirm that production runs v0.1.19 or
later, which admits relation method `reviewed`, and that the
[release floor](../design/pull-deployment.md#release-floor) is at or above that
image, so no rollback can reach an image that drops the warning.

### Roll back a corpus release

Request a lasting rollback by signing the older release again, within its
retention window:

```sh
gh workflow run corpus-release.yml --ref main -f country=vn -f rel=<older rel>
```

The new signature gives that release a new sequence, `<cc>-production` follows
it, and the host reactivates the retained database. A manual swap of
`corpus_release.active_db` leaves the tag behind, and the next upload refuses
until the two agree.

Check a rollback target ahead of need, from its upload directory. The command
needs `oras` and `cosign`, signs and pushes nothing, prints each signed field
that would change and exits 3 when any would:

```sh
python3 deploy/github/corpus_production.py resign-check vn <rel> <upload-dir>
```

`no signed field would change` does not prove the rollback will sign. Signing
also runs `check_remote`, which fetches the previous release's report and
recomputes the comparison with today's rules. It needs the S3 key, so
`resign-check` does not run it.

### Refresh the home sample

The home page shows one stored search result whose stamp states its release
date. After the host activates a new corpus release (`corpus_status` on
`https://trachuan.vn` names it), refresh the sample from the MacBook with a
personal token, typed at a hidden prompt rather than on the command line:

```sh
read -rs TRACHUAN_TOKEN && export TRACHUAN_TOKEN
TRACHUAN_URL=https://trachuan.vn node apps/web/scripts/make-sample.mjs
unset TRACHUAN_TOKEN
```

The script rewrites `apps/web/app/data/sample-result.json` and
`sample-result.meta.json`. Review the diff and open a web pull request with both
files; the sample reaches the site with the next tag.

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

Rollback to a hand-restored release within its seven days is the owner's manual
compare-and-swap of `corpus_release.active_db`.

Prune drops only databases listed in `retired_release`. A database restored by
hand that never became active through the pipeline, such as
`corpus_vn_20261112_2`, has no row there and stays until it is dropped once by
hand. Confirm it is not `active_db`, not in `retired_release`, and has no
connections, then drop it:

```sh
DB=corpus_vn_20261112_2
podman exec -i trachuan-postgres /usr/local/bin/postgres-admin.sh psql -X -At \
  -U postgres -d auth -v db="$DB" <<'SQL'
SELECT (SELECT count(*) FROM corpus_release WHERE active_db = :'db'),
       (SELECT count(*) FROM retired_release WHERE db_name = :'db'),
       (SELECT count(*) FROM pg_stat_activity WHERE datname = :'db');
SQL
podman exec trachuan-postgres /usr/local/bin/postgres-admin.sh dropdb -U postgres "$DB"
```

Drop only when all three counts are 0. The release replaced by the first
pipeline activation (`corpus_vn_20261114_3`) has a row with a NULL digest, so
prune drops it after its seven days.

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
