#!/usr/bin/env bash
# Keep the administration password in the container environment, never arguments.
set -euo pipefail
[[ $# -gt 0 ]] || { echo 'usage: postgres-admin.sh command [arguments]' >&2; exit 1; }
export PGPASSWORD PGUSER=postgres PGPASSFILE=/dev/null PGSERVICEFILE=/dev/null
unset PGSERVICE PGHOSTADDR
PGPASSWORD=$(cat /run/secrets/postgres_password)
[[ -n $PGPASSWORD ]] || { echo 'postgres-admin: missing password' >&2; exit 1; }
exec "$@"
