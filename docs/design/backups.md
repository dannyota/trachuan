# Backups with pgBackRest

Proposed. How the [backup rules](deployment.md#backups) run on the host:
pgBackRest to the private backup bucket over the
[Object Storage](deployment.md#object-storage) S3 endpoint, continuous WAL,
weekly full, daily differential, weekly verify, 30-day retention, a monthly auth
restore check, health results and a production restore procedure.

## Version and image

pgBackRest 2.60.0 (released 2026-10-18), the current stable release; PostgreSQL
18 support arrived in 2.55.0
([release notes](https://pgbackrest.org/release.html)). The project announced
the end of maintenance on 2026-04-27 and its continuation with sponsors on
2026-05-18 ([news](https://pgbackrest.org/news.html)); that risk was weighed and
accepted, and a later release goes through the same review.

`archive_command` runs inside the PostgreSQL container, so the binary must be
there; the upstream pgvector image has none. The Images workflow builds
`trachuan-postgres` from the pinned pgvector digest (Debian 13, PostgreSQL 18.7)
plus the PGDG package `pgbackrest=2.60.0-1.pgdg13+1` (arm64 `.deb` SHA-256
`ac2348d57b62f8ae02bb012d7b776c7aff10aa9d798b03dcde0e05ad6f53ae4e`, checked
before install). The image keeps user 999 and the upstream entrypoint, passes
the workflow's binary-list check, and is pinned by digest and attested like the
other images.

## Configuration and secrets

Static options live in `/etc/pgbackrest/pgbackrest.conf` in the host bundle:
stanza `trachuan`, `pg1-path` and `pg1-socket-path` for the live cluster;
`repo1-type=s3`, `repo1-s3-endpoint`
`axj58m4cpgbr.compat.objectstorage.ap-singapore-1.oraclecloud.com`,
`repo1-s3-region=ap-singapore-1`, `repo1-s3-bucket=trachuan-backups`,
`repo1-s3-uri-style=path`, `repo1-s3-key-type=shared`,
`repo1-path=/pgbackrest/trachuan`, `repo1-cipher-type=none`,
`repo1-retention-full-type=time`, `repo1-retention-full=30`,
`compress-type=zst`, `archive-async=y` with `spool-path` on
`/srv/trachuan/data/pgbackrest`, `process-max=2`, `start-fast=y`, TLS
verification on.

No credential enters that file, an image, an environment value or a command
line. The PostgreSQL unit mounts the Podman secrets `backup_s3_key`,
`backup_s3_key_secret` and `postgres_password`. Before PostgreSQL starts, a
wrapper writes `repo1-s3-key` and `repo1-s3-key-secret` to
`/run/pgbackrest/conf.d/secret.conf` and a password file to
`/run/pgbackrest/pgpass`, both on a private tmpfs, owner 999, mode 0600; the
container environment carries only the paths
(`PGBACKREST_CONFIG_INCLUDE_PATH=/run/pgbackrest/conf.d`, `PGPASSFILE`), so
`podman exec` sees them and connections keep SCRAM. A missing or empty secret
stops the container.

## WAL archiving and its failure

PostgreSQL runs with `archive_mode=on`,
`archive_command='pgbackrest --stanza=trachuan archive-push %p'` and
`archive_timeout=300` (RPO minutes). `archive-push-queue-max` stays unset: when
the bucket or credentials fail, WAL accumulates in `pg_wal` on the data volume
rather than being dropped, and PostgreSQL stops only if that volume fills. The
hourly check reads `pg_stat_archiver` and the size of `pg_wal`: archiving
failing for more than 30 minutes, or `pg_wal` above 10 GB, is unhealthy. The
runbook's outage response is to restore bucket access first; when the volume
nears full, the owner may set a no-op `archive_command` (losing point-in-time
recovery for the gap) and must take a full backup once archiving returns.

## Schedules and results

Host user units run `podman exec trachuan-postgres pgbackrest …`, so backups use
the archiving binary, configuration and data directory. All share one lock; a
run that cannot take it records `skipped`.

| Unit               | Schedule (Asia/Ho_Chi_Minh)  | Command                                  | Result file          |
| ------------------ | ---------------------------- | ---------------------------------------- | -------------------- |
| Full               | Sunday 02:00                 | `backup --type=full`                     | `backup.json`        |
| Differential       | Monday to Saturday 02:00     | `backup --type=diff`                     | `backup.json`        |
| Verify             | Wednesday 04:00              | `verify --output=text --verbose`         | `verify.json`        |
| Health             | hourly                       | `info --output=json`, `pg_stat_archiver` | `health.json`        |
| Auth restore check | first day of the month 05:00 | below                                    | `restore-check.json` |

Timers use `Persistent=true`. Expiry runs after every successful backup;
differentials remain until their full expires. `stanza-create` and `check` run
once from the runbook. After each corpus release activation the runbook takes a
full backup, so later differentials do not each carry the new corpus database.

Each unit keeps one JSON file under `/srv/trachuan/state/backup/`, written
atomically, and logs one journald line per change. The file holds the last
attempt (`started_at`, `finished_at` or null, `status`, `reason`) and
`last_success_at`. The unit writes `running` with `started_at` before it starts
work and its final status when it ends. Statuses: `ok`; `failed` (it ran and
found a problem, or its command failed); `unknown` (storage unreachable, output
malformed or a timestamp invalid; never treated as ok); `skipped` (another unit
held the lock). A backup that pgBackRest records with an error, including page
checksum errors, is `failed` and does not count as the newest good backup.
Verify is `ok` only when its verbose text output reports status ok for the
stanza and no invalid files. Health is `failed` when the newest good backup is
older than 26 hours or archiving is unhealthy as above.

For alarms: no file means the unit has never run; `running` past the unit's
deadline means it crashed or hung; a last `started_at` older than twice the
unit's period means it stopped running; `failed`, `unknown` and repeated
`skipped` mean it ran without success, and `last_success_at` tells how long ago
it last succeeded. This note adds no alarm transport.

## Auth restore check

`restore-check.sh` proves that the newest backup restores the auth database. It
is not a recovery-time measurement. It takes the backup lock, removes any
leftover `trachuan-restore-check` container and directory first, then runs that
container from the same image with only `/srv/trachuan/data/restore-check`
mounted as its data directory, its own socket directory and spool, no TCP
listener (`listen_addresses=''`), the repository keys as its only secrets,
`archive_mode=off`, 2 GB memory and one CPU. It starts PostgreSQL with its own
`hba_file` that allows only `local` peer authentication for the `postgres` role
on that private socket (the container runs as user 999, the image's `postgres`),
so it needs no database password and the restored SCRAM rules never apply to it.
It restores with
`--set=<newest good backup label> --db-include=auth --type=immediate --target-action=promote`
(other databases are restored as sparse zeroed files and never opened), waits up
to 20 minutes for recovery to end and `pg_is_in_recovery()` to return false,
runs smoke queries (auth schema version; row counts of accounts, sessions,
grants and tokens; one index read), records the elapsed time and result, and
always stops the container and deletes the directory through a trap on exit and
signals. It never touches the live cluster, its socket or archive.

## Production restore

The runbook restores the whole cluster in this order: stop Caddy, web and
server; stop PostgreSQL; `pgbackrest restore --delta` to the chosen target;
start PostgreSQL and wait for recovery to finish and promote; in one committed
transaction increment every `auth_epoch` and revoke all sessions and tokens
([auth](auth.md)); start server and web; prewarm the active corpus database
([runbook](../runbooks/release-restore.md#prewarm-a-corpus-database)); pass
readiness; then start Caddy. Before the one-hour RTO is claimed, a timed
whole-cluster rehearsal of this procedure restores into a scratch directory and
port on the host and its time is recorded.

## Size

The cluster backup includes the corpus databases (4.0 GB for vn/20261114_3, plus
the retained release), and restoring a release archives several GB of WAL.
Time-based retention of 30 days keeps five or six weekly fulls (one older than
30 days anchors the window) with their differentials and WAL. At roughly 4 to 6
GB per compressed full that is about 25 to 36 GB of fulls; differentials and WAL
add more, more again in a week with a release activation before its full. Expect
40 to 70 GB, above the 50 GB Object Storage estimate; record the measured size
in [host budgets](budgets-host.md) after the first month (0.0255 USD per
GB-month above the free amount). Moving corpus databases to a separate
unarchived cluster is the alternative if growth demands it.
