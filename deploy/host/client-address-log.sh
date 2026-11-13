#!/usr/bin/env bash
# Keep proof configuration and logs in the user's runtime directory.
set -euo pipefail
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
runtime=${XDG_RUNTIME_DIR:-/run/user/$(id -u)}
[[ -d $runtime && ! -L $runtime ]] || { echo 'client-address-log: user runtime directory is unavailable' >&2; exit 1; }
proof=$runtime/trachuan-caddy-proof
logs=$runtime/trachuan-caddy-proof-logs
active=$proof/client-address-check.caddy
prepare() {
 local directory
 for directory in "$proof" "$logs"; do
  [[ -d $directory ]] || mkdir -m 0755 "$directory"
 done
}
disable() {
 local status=0
 rm -f "$active"
 if ! systemctl --user restart trachuan-caddy.service; then
  systemctl --user stop trachuan-caddy.service || true
  status=1
 fi
 # Podman's user namespace can remove logs owned by Caddy's mapped UID.
 podman unshare sh -c 'rm -f -- "$1"/client-address-check*' sh "$logs" || status=1
 return "$status"
}
case ${1:-} in
 prepare) prepare ;;
 enable)
  prepare
  install -m 0644 "$here/client-address-check.caddy" "$active"
  if ! systemctl --user restart trachuan-caddy.service; then
   disable || true
   exit 1
  fi
  ;;
 disable) disable ;;
 *) echo 'usage: client-address-log.sh prepare|enable|disable' >&2; exit 1 ;;
esac
