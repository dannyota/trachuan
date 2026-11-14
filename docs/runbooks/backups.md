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
`verify`, `health`, `restore-check`) and one journald line. Read them with:

```sh
for f in /srv/trachuan/state/backup/*.json; do echo "$f"; cat "$f"; echo; done
journalctl --user -u 'trachuan-backup-*' --since today
podman exec trachuan-postgres pgbackrest --stanza=trachuan info
```

No file means the unit never ran; `running` past the unit's deadline means it
crashed or hung; `failed` and `unknown` carry a `reason`. The metrics publisher
reads `status` and `last_success_at`; leave the files' mode and ACL alone.

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
`SELECT pg_is_in_recovery()` returns false. Then, in one committed transaction
in the `auth` database, increment every account's `auth_epoch` and revoke all
sessions and tokens ([auth](../design/auth.md#account-lifecycle)), so
credentials revoked after the backup cannot return. Start `trachuan-server` and
`trachuan-web`, [prewarm](release-restore.md#prewarm-a-corpus-database) the
active corpus database, check `curl --fail http://127.0.0.1:8080/readyz`, and
only then start Caddy.

Before the one-hour RTO is claimed, rehearse this procedure once into a scratch
directory and port on the host (never the live data directory), time each step,
and record the total here.
