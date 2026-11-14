# Backups with pgBackRest

Proposed. How the [backup rules](deployment.md#backups) run on the host:
pgBackRest to the private backup bucket over the
[Object Storage](deployment.md#object-storage) S3 endpoint, continuous WAL,
weekly full, daily differential, weekly verify, 30-day retention, a monthly
restore proof and a backup-age check.

## Version and image

pgBackRest 2.60.0 (released 2026-10-18), the current stable release; PostgreSQL
18 support arrived in 2.55.0
([release notes](https://pgbackrest.org/release.html)). The project announced
the end of maintenance on 2026-04-27 and its continuation with sponsors on
2026-05-18 ([news](https://pgbackrest.org/news.html)); a later release goes
through the same review.

`archive_command` runs inside the PostgreSQL container, so the binary must be
there. The upstream pgvector image has none. The Images workflow therefore
builds `trachuan-postgres`: the pinned pgvector image (Debian 13, PostgreSQL
18.7) plus the PGDG package `pgbackrest=2.60.0-1.pgdg13+1` (arm64 `.deb` SHA-256
`ac2348d57b62f8ae02bb012d7b776c7aff10aa9d798b03dcde0e05ad6f53ae4e`), verified
before install, with the image's user and entrypoint unchanged. It is pinned by
digest and attested like the other images, and replaces the upstream reference
in `images.env`.

## Configuration

Static options live in `/etc/pgbackrest/pgbackrest.conf` in the host bundle:
stanza `trachuan` with `pg1-path` set to the data directory and the local
socket; `repo1-type=s3`, `repo1-s3-endpoint`
`axj58m4cpgbr.compat.objectstorage.ap-singapore-1.oraclecloud.com`,
`repo1-s3-region=ap-singapore-1`, `repo1-s3-bucket=trachuan-backups`,
`repo1-s3-uri-style=path`, `repo1-s3-key-type=shared`,
`repo1-path=/pgbackrest/trachuan`, `repo1-cipher-type=none`,
`repo1-retention-full-type=time`, `repo1-retention-full=30`,
`compress-type=zst`, `archive-async=y` with `spool-path` on
`/srv/trachuan/data/pgbackrest`, `process-max=2`, `start-fast=y`, TLS
verification on.

The two keys never enter that file, an image, an environment variable or a
command line. The PostgreSQL unit mounts the Podman secrets `backup_s3_key` and
`backup_s3_key_secret`; before PostgreSQL starts, a small wrapper writes
`repo1-s3-key` and `repo1-s3-key-secret` into `/run/pgbackrest/secret.conf` on a
private tmpfs (mode 0600, owner 999), read through `--config-include-path` (set
as `PGBACKREST_CONFIG_INCLUDE_PATH` in the container environment, so
`podman exec` sees it too). A missing or empty secret stops the container.

## WAL and schedules

PostgreSQL runs with `archive_mode=on`,
`archive_command='pgbackrest --stanza=trachuan archive-push %p'` and
`archive_timeout=300`, so a quiet database still closes a segment every five
minutes (RPO minutes).

Host user units run `pgbackrest` inside the PostgreSQL container with
`podman exec`, so backups use the same binary, configuration and data directory
as archiving:

| Unit          | Schedule (Asia/Ho_Chi_Minh)  | Command                                      |
| ------------- | ---------------------------- | -------------------------------------------- |
| Full          | Sunday 02:00                 | `backup --type=full`                         |
| Differential  | Monday to Saturday 02:00     | `backup --type=diff`                         |
| Verify        | Wednesday 04:00              | `verify`                                     |
| Backup age    | hourly                       | `info --output=json`, compared with 26 hours |
| Restore proof | first day of the month 05:00 | the script below                             |

Timers use `Persistent=true` and a lock so runs never overlap. `stanza-create`
and `check` run once at install, from the runbook. Expiry follows the retention
after each full backup.

## Restore proof

`restore-proof.sh` restores the newest backup into a scratch data directory
under `/srv/trachuan/data/restore-proof` with `--db-include=auth` (system
databases come too; corpus databases are restorable from the release bucket),
starts a scratch PostgreSQL from the same image on a loopback-only port with
`archive_mode=off`, waits for recovery to reach a consistent state, runs smoke
queries (the auth schema version; row counts of accounts, sessions, grants and
tokens; one read through an index), records the elapsed time against the
one-hour RTO, then stops the scratch instance and deletes its directory. It
never touches the live cluster, its port or its archive.

## Results for alarms

The age check and the restore proof each write one JSON result under
`/srv/trachuan/state/backup/` (`age.json`: newest backup type, stop time, age in
seconds, status; `restore-proof.json`: time, backup label, duration, smoke
results, status) and log one line to journald. Alarm work reads these files;
this note adds no alarm transport.

## Restore of production

The runbook covers point-in-time restore of the whole cluster into the data
directory with traffic stopped, then the [auth epoch rule](auth.md) (increment
every `auth_epoch`, revoke all sessions and tokens) before Caddy starts, then
the [prewarm](../runbooks/release-restore.md#prewarm-a-corpus-database) of the
active corpus database.

## Size

The cluster backup includes the corpus databases (4.0 GB for vn/20261114_3, plus
the retained release), and restoring a release writes several GB of WAL that is
archived. With zstd compression, a weekly full of roughly 4 to 6 GB, 30 days of
retention keep four to five fulls plus differentials and WAL, about 30 to 45 GB:
most of the 50 GB Object Storage estimate in [host budgets](budgets-host.md).
Measure the first month and raise the estimate (0.0255 USD per GB-month above
the free amount) rather than drop corpus databases from the cluster; moving
corpus databases to a separate unarchived cluster is the alternative if growth
demands it.
