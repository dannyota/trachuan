#!/usr/bin/env bash
set -euo pipefail
[[ $# == 1 && $1 =~ ^[0-9]{8}-[0-9]{6}F(_[0-9]{8}-[0-9]{6}[DI])?$ ]] || exit 1
runtime=${RESTORE_RUNTIME:-/run/restore-check}
data=${PGDATA:-/var/lib/postgresql/18/docker}
mkdir -p "$runtime"
chmod 0700 "$runtime"
printf 'local all postgres peer\n' >"$runtime/pg_hba.conf"
pgbackrest --stanza=trachuan --pg1-path="$data" --set="$1" --db-include=auth \
 --type=immediate --target-action=promote restore >/dev/null
postgres -D "$data" -c listen_addresses= -c archive_mode=off \
 -c "unix_socket_directories=$runtime" -c unix_socket_permissions=0700 \
 -c "hba_file=$runtime/pg_hba.conf" -c shared_buffers=128MB \
 -c max_connections=5 -c shared_preload_libraries= > /dev/null 2>&1 &
server=$!
cleanup() {
 kill -TERM "$server" 2>/dev/null || true
 wait "$server" 2>/dev/null || true
}
trap cleanup EXIT
trap 'exit 1' HUP INT TERM
deadline=$((SECONDS+1200))
ready=false
while ((SECONDS < deadline)); do
 if [[ $(psql -XAt --no-password --host="$runtime" --username=postgres --dbname=postgres \
  --set=ON_ERROR_STOP=1 --command='SELECT pg_is_in_recovery()' 2>/dev/null) == f ]]; then
  ready=true
  break
 fi
 sleep 1
done
[[ $ready == true ]]
psql -XAt --no-password --host="$runtime" --username=postgres --dbname=auth --set=ON_ERROR_STOP=1 \
 --command="SELECT max(version_id) FROM goose_db_version WHERE is_applied;
 SELECT count(*) FROM account; SELECT count(*) FROM web_session;
 SELECT count(*) FROM oauth_grant; SELECT count(*) FROM oauth_token; SELECT count(*) FROM personal_token;
 SET enable_seqscan=off; SELECT id FROM account ORDER BY id LIMIT 1;" >/dev/null
