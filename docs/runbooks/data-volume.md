# Prepare and migrate the data volume

The primary runs these commands on the host after attaching the 60 GB volume.
Keep the previous host bundle available for rollback. For a new host without an
existing PostgreSQL cluster, prepare the volume and continue with
[host installation](host.md#build-and-install); skip migration.

## Prepare the volume

As `opc`, inspect the device and stop if it is not the attached empty volume.
The guard refuses any recognized filesystem, partition table or other signature.
Do not proceed if inspection fails or shows unexpected contents.

```sh
DEVICE=/dev/oracleoci/oraclevdb
sudo lsblk -f "$DEVICE"
sudo wipefs -n "$DEVICE"
if test -b "$DEVICE" && signatures=$(sudo wipefs -n --noheadings --output TYPE "$DEVICE") &&
  test -z "$signatures"; then
  sudo mkfs.xfs -L trachuandata "$DEVICE"
else
  echo 'Device inspection failed or found signatures; stop' >&2
fi
sudo mkdir -p /srv/trachuan/data
sudo blkid -s UUID -o value "$DEVICE"
```

Continue only if formatting succeeded. Add one line to `/etc/fstab`, replacing
`<UUID>` with the printed UUID. `nofail` keeps the host bootable and reachable
over SSH if the volume is missing; PostgreSQL then refuses to start because its
unit checks the mount:

```text
UUID=<UUID> /srv/trachuan/data xfs defaults,nofail,x-systemd.device-timeout=30s 0 2
```

Mount and label the filesystem. The installer creates the private directories.

```sh
sudo systemctl daemon-reload
sudo mount /srv/trachuan/data
mountpoint -q /srv/trachuan/data
sudo restorecon -RF /srv/trachuan/data
```

## Migrate an existing cluster

As `trachuan`, stop traffic and writers, record the active release and database
sizes, then stop PostgreSQL. Save the output outside Git for comparison.

```sh
sudo -iu trachuan
export XDG_RUNTIME_DIR=/run/user/$(id -u)
systemctl --user stop trachuan-caddy trachuan-web trachuan-server trachuan-queryembed trachuan-migrate
podman exec trachuan-postgres /usr/local/bin/postgres-admin.sh psql -X -At \
  -U postgres -d auth -c "SELECT active_db FROM corpus_release WHERE country = 'vn'"
podman exec trachuan-postgres /usr/local/bin/postgres-admin.sh psql -X -At \
  -U postgres -d postgres -c 'SELECT datname, pg_database_size(oid) FROM pg_database ORDER BY datname'
systemctl --user stop trachuan-postgres
systemctl --user is-active trachuan-postgres && echo 'PostgreSQL still running; stop' >&2
podman volume inspect trachuan-postgres --format '{{.Mountpoint}}'
exit
```

As `opc`, copy the stopped cluster as root. Set `OLD` to the inspected path and
confirm it is the named volume's mountpoint before copying. The block stops at
the first failed check: the data mount, a leftover `postmaster.pid`, a non-empty
target, the copy, or any regular file whose path or size differs. The `:U` mount
option re-owns the copy when PostgreSQL starts.

```sh
OLD=/path/from/volume-inspect
set -e
mountpoint -q /srv/trachuan/data
sudo test ! -e "$OLD/data/postmaster.pid" -a ! -e "$OLD/18/docker/postmaster.pid"
sudo install -d -o trachuan -g trachuan -m 0700 /srv/trachuan/data/postgres /srv/trachuan/data/pgbackrest
sudo test -z "$(sudo ls -A /srv/trachuan/data/postgres)"
sudo cp -a "$OLD"/. /srv/trachuan/data/postgres/
diff <(sudo find "$OLD" -type f -printf '%P %s\n' | sort) \
  <(sudo find /srv/trachuan/data/postgres -type f -printf '%P %s\n' | sort)
sudo restorecon -RF /srv/trachuan/data
sudo bash /tmp/trachuan-host/install.sh /tmp/trachuan-host
set +e
```

The installer renders and reloads the units. As `trachuan`, render and reload
again, then start in the deploy order:

```sh
sudo -iu trachuan
export XDG_RUNTIME_DIR=/run/user/$(id -u)
bash /srv/trachuan/host/render-units.sh /srv/trachuan/host/images.env \
  /home/trachuan/.config/containers/systemd
systemctl --user daemon-reload
systemctl --user start trachuan-postgres
podman exec trachuan-postgres /usr/local/bin/postgres-admin.sh /usr/local/bin/wait-postgres.sh
systemctl --user start trachuan-queryembed trachuan-migrate
systemctl --user start trachuan-server trachuan-web
curl --fail http://127.0.0.1:8080/readyz
DB=$(podman exec trachuan-postgres /usr/local/bin/postgres-admin.sh psql -X -At \
  -U postgres -d auth -c "SELECT active_db FROM corpus_release WHERE country = 'vn'")
printf '%s\n' "$DB"
podman exec trachuan-postgres /usr/local/bin/postgres-admin.sh psql -X -At \
  -U postgres -d postgres -c 'SELECT datname, pg_database_size(oid) FROM pg_database ORDER BY datname'
df -h / /srv/trachuan/data
```

Confirm the active release is unchanged and each database size matches the
recorded value. Investigate any difference before accepting traffic.
[Prewarm](release-restore.md#prewarm-a-corpus-database) `$DB`, then start Caddy:

```sh
systemctl --user start trachuan-caddy
```

Keep the old named volume until the next successful deploy. Then remove it as
`trachuan` with `podman volume rm trachuan-postgres`.

## Roll back

Before accepting new writes, stop the units in the same order as above. Restore
the previous bundle's PostgreSQL container and volume Quadlets, render the
previous bundle, reload the user daemon and start in the same order. Verify
readiness and prewarm before starting Caddy. The old named volume remains
untouched by the copy. Once the new cluster has accepted writes, do not switch
to the stale named volume without reconciling those writes.
