#!/usr/bin/env bash
set +x
set -euo pipefail
export LC_ALL=C
fail() { echo "auth-psql: $*" >&2; exit 2; }
session() {
 local attach=(exec)
 [[ -t 0 ]] || attach+=(-i)
 # shellcheck disable=SC2016
 podman "${attach[@]}" trachuan-postgres sh -c \
  'set +x; set -eu; export PGPASSWORD; PGPASSWORD=$(cat /run/secrets/auth_db_password); [ -n "$PGPASSWORD" ]; exec "$@"' sh \
  timeout --signal=TERM --kill-after=5s 60s env PGCONNECT_TIMEOUT=5 \
  'PGOPTIONS=-c search_path=pg_catalog,public -c statement_timeout=30s -c lock_timeout=5s -c idle_in_transaction_session_timeout=30s -c client_connection_check_interval=2s' \
  psql --no-password -XqAt "$@" -h 127.0.0.1 -U auth_owner -d auth -v ON_ERROR_STOP=1
}
if [[ ${1:-} == active-db ]]; then
 [[ $# == 1 ]] || fail 'active-db takes no arguments'
 name=$(session -c "SELECT active_db FROM public.corpus_release WHERE country = 'vn';" </dev/null | head -c 128)
 [[ ${#name} -le 63 && $name =~ ^corpus_[a-z]{2}_[0-9]{8}(_[1-9][0-9]*)?$ ]] || { echo 'auth-psql: invalid active database' >&2; exit 1; }
 printf '%s\n' "$name"
 exit 0
fi
[[ $# -gt 0 || ! -t 0 ]] || fail 'give -c SQL or pipe SQL on stdin'
rest=("$@")
while ((${#rest[@]})); do
 case ${rest[0]} in
  -c|-v) ((${#rest[@]} >= 2)) || fail "${rest[0]} needs a value"; rest=("${rest[@]:2}") ;;
  --command=*|--set=*|--variable=*) rest=("${rest[@]:1}") ;;
  *) fail 'allowed arguments: -c SQL, --command=SQL, -v name=value, --set=name=value, --variable=name=value' ;;
 esac
done
session "$@"
