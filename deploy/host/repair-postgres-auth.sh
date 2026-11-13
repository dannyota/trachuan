#!/usr/bin/env bash
# Run as postgres inside the existing container during a maintenance window.
set -euo pipefail
pgdata=${PGDATA:?PGDATA must name the PostgreSQL data directory}
[[ -f $pgdata/PG_VERSION && $(cat "$pgdata/PG_VERSION") == 18 && \
 -f $pgdata/pg_hba.conf && ! -L $pgdata/pg_hba.conf ]] || {
 echo 'postgres-auth: expected an initialized PostgreSQL 18 cluster' >&2; exit 1;
}
staged=$(mktemp "$pgdata/.pg_hba.XXXXXX")
trap 'rm -f "$staged"' EXIT
cat >"$staged" <<'HBA'
local all all scram-sha-256
host all all 127.0.0.1/32 scram-sha-256
host all all ::1/128 scram-sha-256
HBA
chmod 0600 "$staged"
mv -f "$staged" "$pgdata/pg_hba.conf"
pg_ctl reload -D "$pgdata"
