# Back up and restore PostgreSQL

pgBackRest on the [production host](host.md), as designed in
[backups](../design/backups.md). Commands run as the `trachuan` user unless
marked root:

```sh
sudo -u trachuan -i
export XDG_RUNTIME_DIR=/run/user/$(id -u)
```

## First setup

The Podman secrets `backup_s3_key` and `backup_s3_key_secret` exist, the image
in `images.env` is `trachuan-postgres` with pgBackRest, and `install.sh` has
run. Restart PostgreSQL so it starts archiving, then create the stanza and prove
the repository before the first backup:

```sh
systemctl --user restart trachuan-postgres
podman exec trachuan-postgres /usr/local/bin/postgres-admin.sh /usr/local/bin/wait-postgres.sh
podman exec trachuan-postgres pgbackrest --stanza=trachuan stanza-create
podman exec trachuan-postgres pgbackrest --stanza=trachuan check
systemctl --user start trachuan-backup-full.service
systemctl --user list-timers 'trachuan-backup-*'
```

`check` forces a WAL switch and confirms that the segment reached the bucket.
Record the time of the first full backup and the bucket size after it.

## Results

Each unit writes `/srv/trachuan/state/backup/<unit>.json` (`full`, `diff`,
`verify`, `health`, `restore-check`) and one journald line per change. Each
final result also appends one JSON line to `/var/log/trachuan/backup.log`,
shipped to OCI Logging. Read them with:

```sh
for f in /srv/trachuan/state/backup/*.json; do echo "$f"; cat "$f"; echo; done
journalctl --user -u 'trachuan-backup-*' --since today
tail -n 20 /var/log/trachuan/backup.log
podman exec trachuan-postgres pgbackrest --stanza=trachuan info
```

No file means the unit never ran; `running` past the unit's deadline means it
crashed or hung; `failed` and `unknown` carry a `reason`. The metrics publisher
reads `status` and `last_success_at`; leave the files' mode and ACL alone. Only
health can be `skipped`: another backup or the updater holds the lock. Other
units record `running` with reason `waiting for backup lock` while they wait.
Waiting counts toward their deadline; lock expiry records `failed` and sets that
job's `backup_failed` metric to 1. Command output is withheld from result
reasons and the JSON log.

## Recover a missed backup

Read `full.json` and `diff.json` and check `last_success_at`. If a full has ever
succeeded, start a differential:

```sh
systemctl --user start --no-block trachuan-backup-diff.service
journalctl --user -fu trachuan-backup-diff.service
```

Use `trachuan-backup-full.service` in both commands when no full has succeeded.

The unit waits for any backup or deployment to release the lock. Confirm `ok`
and a new `last_success_at` in the selected result file after completion. Do not
remove `backup.lock` or `active-job`.

## After a corpus release activation

Take a full backup so the following differentials do not each carry the new
corpus database:

```sh
systemctl --user start trachuan-backup-full.service
```

## Archiving outage

When `health.json` reports archiving failing or `pg_wal` above 10 GB:

1. Find the cause:
   `podman exec trachuan-postgres pgbackrest --stanza=trachuan check` and
   `journalctl --user -u trachuan-postgres --since -1h`. Usual causes are a
   rotated or revoked customer secret key, an Object Storage outage or the
   bucket policy.
2. Restore access. WAL waits in `pg_wal` and is pushed once archiving works;
   nothing is lost while the data volume has space (`df -h /srv/trachuan/data`).
3. Only if the volume is about to fill and access cannot be restored in time,
   the owner may set a no-op archive command, accepting a gap in point-in-time
   recovery:
   `podman exec trachuan-postgres /usr/local/bin/postgres-admin.sh psql -U postgres -d postgres -c "ALTER SYSTEM SET archive_command = '/bin/true'" -c "SELECT pg_reload_conf()"`.
   Undo it with `ALTER SYSTEM RESET archive_command` and a reload once access
   returns, then take a full backup at once.

## Rotate the backup key

Create a new customer secret key for the backup user, replace both Podman
secrets, restart PostgreSQL, run `check` and a differential backup, then revoke
the old key in OCI.

## Restore production

With traffic stopped, in this order:

```sh
systemctl --user stop trachuan-caddy trachuan-web trachuan-server
systemctl --user stop trachuan-postgres
podman run --rm --network=host --user=999:999 \
  --volume /srv/trachuan/data/postgres:/var/lib/postgresql:Z,U \
  --volume /etc/pgbackrest/pgbackrest.conf:/etc/pgbackrest/pgbackrest.conf:ro,z \
  --volume /srv/trachuan/host/pgbackrest-entrypoint.sh:/usr/local/bin/pgbackrest-entrypoint.sh:ro,z \
  --mount=type=tmpfs,destination=/run/pgbackrest,tmpfs-mode=0700,U=true,noexec,nosuid,nodev \
  --env PGBACKREST_CONFIG_INCLUDE_PATH=/run/pgbackrest/conf.d \
  --secret=backup_s3_key,type=mount,uid=999,gid=999,mode=0400 \
  --secret=backup_s3_key_secret,type=mount,uid=999,gid=999,mode=0400 \
  --entrypoint /usr/local/bin/pgbackrest-entrypoint.sh "$POSTGRES_IMAGE" \
  --repository-only pgbackrest --stanza=trachuan --delta restore
systemctl --user start trachuan-postgres
podman exec trachuan-postgres /usr/local/bin/postgres-admin.sh /usr/local/bin/wait-postgres.sh
```

Add `--type=time --target='<timestamp>'` to the restore for a point in time;
`POSTGRES_IMAGE` is the reference in `/srv/trachuan/host/images.env`. Wait until
`SELECT pg_is_in_recovery()` returns false. Run migrations with the pinned
server image through the host's migration unit first:

```sh
systemctl --user restart trachuan-migrate.service
```

Require a zero exit status before running the fence. Use `restart` because the
unit has `RemainAfterExit=true`; `start` could skip migrations after a restore.
The unit uses the server image pinned in `/srv/trachuan/host/images.env`, the
auth database secret and the host's PostgreSQL authentication check. Run the
fence with that same image as `SERVER_IMAGE`:

```sh
podman run --rm --network=host --user=65532:65532 \
  --read-only --cap-drop=ALL --security-opt=no-new-privileges \
  --secret=server_auth_database_url,type=env,target=SERVER_AUTH_DATABASE_URL \
  "$SERVER_IMAGE" admin restore-fence
```

Require a zero exit status before continuing. The command increments every
account's `auth_epoch`, revokes all credentials and outstanding login proofs,
and revokes every live framework entitlement with reason `restore`, with one
audit event per entitlement, in one committed transaction. Running it again is
safe: epochs advance again, with no duplicate revocation events. The operator
must allow entitlements again from the event history. Start `trachuan-server`
and `trachuan-web`,
[prewarm](corpus-restore-manual.md#prewarm-a-corpus-database) the active corpus
database, check `curl --fail http://127.0.0.1:8080/readyz`, and only then start
Caddy.

Before the one-hour RTO is claimed, rehearse this procedure once into a scratch
directory and port on the host (never the live data directory), time each step,
and record the total here.
