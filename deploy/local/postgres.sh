#!/usr/bin/env bash
# Native PostgreSQL 18 for local development: one instance for this project,
# data in TRACHUAN_PG_DATA, listening on 127.0.0.1:25433. Usage: postgres.sh up|down|status
set -Eeuo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/../.."
bin="${TRACHUAN_PG_BIN:-/opt/homebrew/opt/postgresql@18/bin}"
data="${TRACHUAN_PG_DATA:-$HOME/.local/share/trachuan-pg18}"
[ -x "$bin/pg_ctl" ] || { echo "postgres: $bin/pg_ctl missing; brew install postgresql@18 pgvector" >&2; exit 1; }
case "${1:-}" in
up)
  if [ ! -f "$data/PG_VERSION" ]; then
    set -a
    # shellcheck disable=SC1091
    . ./.env
    set +a
    : "${POSTGRES_PASSWORD:?set POSTGRES_PASSWORD in .env}"
    mkdir -p "$(dirname "$data")"
    "$bin/initdb" -D "$data" -U trachuan --auth-local=trust --auth-host=scram-sha-256 -E UTF8 \
      --locale-provider=builtin --builtin-locale=C.UTF-8 --pwfile=<(printf '%s' "$POSTGRES_PASSWORD") >/dev/null
    cat >>"$data/postgresql.conf" <<'CONF'

# trachuan local development (deploy/local/postgres.sh)
listen_addresses = '127.0.0.1'
port = 25433
max_connections = 200
shared_buffers = 1GB
# Parallel HNSW builds allocate shared memory up to maintenance_work_mem.
maintenance_work_mem = 2GB
max_parallel_maintenance_workers = 4
CONF
  fi
  "$bin/pg_ctl" -D "$data" status >/dev/null 2>&1 || "$bin/pg_ctl" -D "$data" -l "$data/server.log" -w start >/dev/null
  "$bin/pg_isready" -q -h 127.0.0.1 -p 25433
  echo "postgres: up ($data)"
  ;;
down)
  if "$bin/pg_ctl" -D "$data" status >/dev/null 2>&1; then "$bin/pg_ctl" -D "$data" -m fast -w stop >/dev/null; fi
  echo "postgres: down"
  ;;
status) "$bin/pg_ctl" -D "$data" status ;;
*) echo "usage: postgres.sh up|down|status" >&2; exit 2 ;;
esac
