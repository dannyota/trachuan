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
control=$(LC_ALL=C pg_controldata "$data")
settings=()
while IFS='|' read -r option label minimum; do
 value=$(awk -F: -v label="$label" '$1 == label {gsub(/ /, "", $2); print $2}' <<<"$control")
 [[ $value =~ ^[0-9]+$ ]] || { echo 'restore-auth: invalid recovery settings' >&2; exit 1; }
 if ((value < minimum)); then value=$minimum; fi
 settings+=(-c "$option=$value")
done <<'SETTINGS'
max_connections|max_connections setting|50
max_worker_processes|max_worker_processes setting|8
max_wal_senders|max_wal_senders setting|10
max_prepared_transactions|max_prepared_xacts setting|0
max_locks_per_transaction|max_locks_per_xact setting|64
SETTINGS
postgres -D "$data" "${settings[@]}" -c listen_addresses= -c archive_mode=off \
 -c "unix_socket_directories=$runtime" -c unix_socket_permissions=0700 \
 -c "hba_file=$runtime/pg_hba.conf" -c shared_buffers=128MB \
 -c hot_standby=off -c shared_preload_libraries= -c logging_collector=off -c log_destination=stderr >"$runtime/postgres.log" 2>&1 &
server=$!
cleanup() {
 code=$?
 kill -TERM "$server" 2>/dev/null || true
 wait "$server" 2>/dev/null || true
 if ((code != 0)); then tail -n 40 "$runtime/postgres.log" >&2; fi
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
 kill -0 "$server" 2>/dev/null || exit 1
 sleep 1
done
[[ $ready == true ]]
psql -XAt --no-password --host="$runtime" --username=postgres --dbname=auth --set=ON_ERROR_STOP=1 \
 --command="SELECT max(version_id) FROM goose_db_version WHERE is_applied;
 SELECT count(*) FROM account; SELECT count(*) FROM web_session;
 SELECT count(*) FROM oauth_grant; SELECT count(*) FROM oauth_token; SELECT count(*) FROM personal_token;
 SET enable_seqscan=off; SELECT id FROM account ORDER BY id LIMIT 1;" >/dev/null
