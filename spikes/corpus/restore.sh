#!/usr/bin/env bash
# Throwaway spike: restore banhmi's Vietnam corpus dump into local PostgreSQL.
# The native PostgreSQL 18 client tools match the local server.
set -Eeuo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/../.."
DUMP=banhmi-v049.dump
mkdir -p work/dumps
if [[ ! -f "work/dumps/$DUMP" ]]; then
  aws s3 cp "s3://danny-banhmi-data-vn/dumps/$DUMP" "work/dumps/$DUMP"
fi
set -a
# shellcheck disable=SC1091
. ./.env
set +a
export PGPASSWORD="$POSTGRES_PASSWORD" PGHOST=127.0.0.1
pg() { PATH="/opt/homebrew/opt/postgresql@18/bin:$PATH" "$@"; }
pg dropdb -U trachuan --if-exists banhmi_vn
pg createdb -U trachuan banhmi_vn
pg psql -U trachuan -d banhmi_vn -v ON_ERROR_STOP=1 -c 'create extension if not exists vector'
pg pg_restore -U trachuan --no-owner --no-privileges -j 4 -d banhmi_vn "work/dumps/$DUMP"
