#!/usr/bin/env bash
set -euo pipefail
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
if [[ $# == 0 ]]; then
 exec python3 "$here/backup.py" restore-check
fi
[[ $# == 3 && $1 == --locked && $2 =~ ^[0-9]{8}-[0-9]{6}F(_[0-9]{8}-[0-9]{6}[DI])?$ && $3 =~ ^sha256:[0-9a-f]{64}$ ]] || exit 1
label=$2
image=$3
name=trachuan-restore-check
data=/srv/trachuan/data/restore-check
cleanup() {
 podman rm --force --ignore "$name" >/dev/null && podman unshare find "$data" -mindepth 1 -maxdepth 1 -exec rm -rf -- {} +
}
finish() {
 code=$?
 trap - EXIT
 if ((code != 0)); then
  podman stop --time=5 "$name" >/dev/null 2>&1 || true
  podman logs --tail=40 "$name" >&2 || true
 fi
 if ! cleanup; then code=1; fi
 exit "$code"
}
trap finish EXIT
trap 'exit 1' HUP INT TERM
cleanup
podman run --detach --name "$name" --network=pasta:--ipv4-only,--dns-forward,1.1.1.1,--dns-host,1.1.1.1,--no-map-gw,--map-guest-addr,none,--map-host-loopback,none,-t,none,-u,none,-T,none,-U,none --dns=1.1.1.1 --no-hosts --memory=2g --cpus=1 \
 --user=999:999 --read-only --cap-drop=ALL --security-opt=no-new-privileges --log-driver=k8s-file --log-opt=max-size=1mb \
 --mount type=tmpfs,destination=/run/pgbackrest,tmpfs-mode=0700,U=true,noexec,nosuid,nodev \
 --mount type=tmpfs,destination=/run/restore-check,tmpfs-mode=0700,U=true,noexec,nosuid,nodev \
 --tmpfs /tmp:rw,noexec,nosuid,nodev,mode=1777 \
 --volume "$data:/var/lib/postgresql:Z,U" \
 --volume /etc/pgbackrest/pgbackrest.conf:/etc/pgbackrest/pgbackrest.conf:ro,z \
 --volume "$here/pgbackrest-entrypoint.sh:/usr/local/bin/pgbackrest-entrypoint.sh:ro,z" \
 --volume "$here/restore-auth.sh:/usr/local/bin/restore-auth.sh:ro,z" \
 --env PGBACKREST_CONFIG_INCLUDE_PATH=/run/pgbackrest/conf.d \
 --env PGBACKREST_SPOOL_PATH=/run/pgbackrest/spool \
 --env PGBACKREST_LOCK_PATH=/run/pgbackrest/lock \
 --env PGBACKREST_PG1_SOCKET_PATH=/run/restore-check \
 --secret=backup_s3_key,type=mount,uid=999,gid=999,mode=0400 \
 --secret=backup_s3_key_secret,type=mount,uid=999,gid=999,mode=0400 \
 --entrypoint /usr/local/bin/pgbackrest-entrypoint.sh "$image" \
 --repository-only /usr/local/bin/restore-auth.sh "$label" >/dev/null
[[ $(podman wait "$name") == 0 ]]
