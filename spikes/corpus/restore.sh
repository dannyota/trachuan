#!/usr/bin/env bash
# Throwaway spike: restore banhmi's Vietnam corpus dump into local PostgreSQL.
# pg_restore runs inside the container so the client matches the server.
set -Eeuo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/../.."
DUMP=banhmi-v049.dump
mkdir -p work/dumps
if [[ ! -f "work/dumps/$DUMP" ]]; then
  aws s3 cp "s3://danny-banhmi-data-vn/dumps/$DUMP" "work/dumps/$DUMP"
fi
pg() { podman exec -i trachuan_postgres_1 "$@"; }
pg dropdb -U trachuan --if-exists banhmi_vn
pg createdb -U trachuan banhmi_vn
pg psql -U trachuan -d banhmi_vn -v ON_ERROR_STOP=1 -c 'create extension if not exists vector'
pg pg_restore -U trachuan --no-owner --no-privileges -j 4 -d banhmi_vn "/dumps/$DUMP"
