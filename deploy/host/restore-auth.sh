#!/usr/bin/env bash
set -euo pipefail
[[ $# == 1 && $1 =~ ^[0-9]{8}-[0-9]{6}F(_[0-9]{8}-[0-9]{6}[DI])?$ ]] || exit 1
runtime=${RESTORE_RUNTIME:-/run/restore-check}
data=${PGDATA:-/var/lib/postgresql/18/docker}
mkdir -p "$runtime"
chmod 0700 "$runtime"
printf 'local all postgres peer\nlocal auth auth_owner peer map=restore\n' >"$runtime/pg_hba.conf"
printf 'restore postgres auth_owner\n' >"$runtime/pg_ident.conf"
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
# shellcheck disable=SC2016
for setting in archive_command= archive_library= archive_cleanup_command= recovery_end_command= \
 session_preload_libraries= local_preload_libraries= oauth_validator_libraries= \
 ssl=off ssl_passphrase_command= jit=off jit_provider=llvmjit \
 'dynamic_library_path=$libdir' 'extension_control_path=$system' output_plugin_libraries=pgoutput,test_decoding \
 event_triggers=off external_pid_file= "data_directory=$data" \
 "restore_command=pgbackrest --pg1-path=$data --stanza=trachuan archive-get %f \"%p\""; do
 settings+=(-c "$setting")
done
postgres -D "$data" "${settings[@]}" -c listen_addresses= -c archive_mode=off \
 -c "unix_socket_directories=$runtime" -c unix_socket_permissions=0700 \
 -c "hba_file=$runtime/pg_hba.conf" -c "ident_file=$runtime/pg_ident.conf" -c shared_buffers=128MB \
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
  --set=ON_ERROR_STOP=1 --command='SELECT pg_catalog.pg_is_in_recovery()' 2>/dev/null) == f ]]; then
  ready=true
  break
 fi
 kill -0 "$server" 2>/dev/null || exit 1
 sleep 1
done
[[ $ready == true ]]
PGOPTIONS='-c search_path=pg_catalog,public -c statement_timeout=120s -c lock_timeout=5s -c idle_in_transaction_session_timeout=120s -c client_connection_check_interval=2s' \
timeout --signal=TERM --kill-after=5s 300s psql -XAt --no-password --host="$runtime" --username=auth_owner --dbname=auth --set=ON_ERROR_STOP=1 \
 --command="SELECT pg_catalog.max(version_id) FROM public.goose_db_version WHERE is_applied;
 SELECT pg_catalog.count(*) FROM public.account; SELECT pg_catalog.count(*) FROM public.web_session;
 SELECT pg_catalog.count(*) FROM public.oauth_grant; SELECT pg_catalog.count(*) FROM public.oauth_token;
 SELECT pg_catalog.count(*) FROM public.personal_token;
 SET enable_seqscan=off; SELECT id FROM public.account ORDER BY id LIMIT 1;" >/dev/null
