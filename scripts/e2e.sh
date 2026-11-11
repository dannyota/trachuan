#!/usr/bin/env bash
set -euo pipefail
unset PGHOSTADDR PGHOST PGPORT PGSERVICE PGSERVICEFILE PGSYSCONFDIR PGDATABASE PGOPTIONS
cd "$(dirname "${BASH_SOURCE[0]}")/.."

set -a
if [[ -f .env ]]; then
  # shellcheck disable=SC1091
  . ./.env
fi
set +a
unset PGHOSTADDR PGHOST PGPORT PGSERVICE PGSERVICEFILE PGSYSCONFDIR PGDATABASE PGOPTIONS
if [[ -z "${PGPASSWORD:-}" && -n "${POSTGRES_PASSWORD:-}" ]]; then
  export PGPASSWORD="$POSTGRES_PASSWORD"
fi
export CORPUS_TEST_ADMIN_URL="${CORPUS_TEST_ADMIN_URL:-postgres://trachuan@127.0.0.1:25433/postgres}"
server_port="${E2E_SERVER_PORT:-28080}"
google_port="${E2E_GOOGLE_PORT:-28081}"
embed_port="${E2E_EMBED_PORT:-28082}"
for port in "$server_port" "$google_port" "$embed_port"; do
  if [[ ! "$port" =~ ^[1-9][0-9]{0,4}$ ]] || (( port > 65535 )); then
    echo "e2e: invalid port" >&2
    exit 1
  fi
done
if [[ "$server_port" == "$google_port" || "$server_port" == "$embed_port" || "$google_port" == "$embed_port" ]]; then
  echo "e2e: ports must differ" >&2
  exit 1
fi

db_name="trachuan_e2e_$(od -An -N6 -tx1 /dev/urandom | tr -d ' \n')"
database_url() {
  python3 - "$1" <<'PY'
import ipaddress
import os
import sys
from urllib.parse import parse_qsl, urlsplit, urlunsplit
try:
    url = urlsplit(os.environ["CORPUS_TEST_ADMIN_URL"])
    host = url.hostname or ""
    loopback = host == "localhost" or ipaddress.ip_address(host).is_loopback
    allowed = {"sslmode", "connect_timeout"}
    params = parse_qsl(url.query, keep_blank_values=True, strict_parsing=True)
    if url.scheme not in ("postgres", "postgresql") or not loopback or any(k not in allowed for k, _ in params):
        raise ValueError()
    print(urlunsplit((url.scheme, url.netloc, "/" + sys.argv[1], url.query, "")))
except (ValueError, KeyError):
    sys.exit("e2e: invalid local administrator URL")
PY
}
auth_url="$(database_url "$db_name")"
corpus_url="$(database_url '{database}')"
port_available() {
  python3 - "$1" <<'PYCODE'
import errno
import socket
import sys

with socket.socket() as connection:
    connection.settimeout(0.2)
    result = connection.connect_ex(("127.0.0.1", int(sys.argv[1])))
sys.exit(0 if result == errno.ECONNREFUSED else 1)
PYCODE
}
require_available_port() {
  if ! port_available "$1"; then
    echo "${2:-e2e: port in use}" >&2
    exit 1
  fi
}
require_available_port "$server_port"
require_available_port "$google_port"
require_available_port "$embed_port"
if ! command -v lsof >/dev/null 2>&1; then
  echo "e2e: port not owned by child" >&2
  exit 1
fi
tmp_dir="$(mktemp -d "${TMPDIR:-/tmp}/trachuan-e2e.XXXXXXXX")"
server_pid=""
google_pid=""
embed_pid=""
fixture_name=""
fixture_key=""
db_created=0
cleanup() {
  local status=$?
  if [[ -n "$server_pid" ]]; then kill "$server_pid" 2>/dev/null || true; wait "$server_pid" 2>/dev/null || true; fi
  if [[ -n "$google_pid" ]]; then kill "$google_pid" 2>/dev/null || true; wait "$google_pid" 2>/dev/null || true; fi
  if [[ -n "$embed_pid" ]]; then kill "$embed_pid" 2>/dev/null || true; wait "$embed_pid" 2>/dev/null || true; fi
  if [[ -n "$fixture_name" && -n "$fixture_key" ]]; then
    if ! "$tmp_dir/e2efixture" drop "$fixture_name" "$fixture_key"; then
      echo "e2e: fixture cleanup failed" >&2
      status=1
    fi
  fi
  if [[ "$db_created" == 1 && "$db_name" =~ ^trachuan_e2e_[0-9a-f]{12}$ ]]; then
    if ! printf 'DROP DATABASE IF EXISTS :"db" WITH (FORCE)\n' | psql -X "$CORPUS_TEST_ADMIN_URL" -v ON_ERROR_STOP=1 -v db="$db_name"; then
      echo "e2e: database cleanup failed" >&2
      status=1
    fi
  fi
  rm -r -- "$tmp_dir" || status=1
  exit "$status"
}
trap cleanup EXIT
trap 'exit 1' INT TERM

go build -o "$tmp_dir/server" ./apps/server/cmd/server
go build -o "$tmp_dir/fakegoogle" ./apps/server/cmd/fakegoogle
go build -o "$tmp_dir/fakeembed" ./apps/server/cmd/fakeembed
go build -tags integration -o "$tmp_dir/e2efixture" ./apps/server/cmd/e2efixture
# psql variable substitution is performed by the input parser.
printf 'CREATE DATABASE :"db"\n' | psql -X "$CORPUS_TEST_ADMIN_URL" -v ON_ERROR_STOP=1 -v db="$db_name"
db_created=1
require_available_port "$server_port" "e2e: port not owned by child"
export SERVER_AUTH_DATABASE_URL="$auth_url"
export SERVER_CORPUS_DATABASE_URL="$corpus_url"
export SERVER_LISTEN="127.0.0.1:$server_port"
export SERVER_PUBLIC_URL="http://127.0.0.1:$server_port"
export SERVER_QUERY_EMBEDDER_URL="http://127.0.0.1:$embed_port"
export SERVER_GOOGLE_ISSUER="http://127.0.0.1:$google_port"
export GOOGLE_CLIENT_ID=e2e-client
export GOOGLE_CLIENT_SECRET=e2e-secret
export E2E_SERVER_URL="$SERVER_PUBLIC_URL"
export E2E_GOOGLE_URL="$SERVER_GOOGLE_ISSUER"
"$tmp_dir/server" migrate
read -r fixture_name fixture_key < <("$tmp_dir/e2efixture" create)
if [[ ! "$fixture_name" =~ ^corpus_vn_2099[0-9]{4}(_[0-9]+)?$ || "$fixture_key" != "vn/${fixture_name#corpus_vn_}" ]]; then
  echo "e2e: invalid fixture identity" >&2
  exit 1
fi
printf "INSERT INTO corpus_release VALUES ('vn', :'db', now(), 'e2e')\n" | psql -X "$auth_url" -v ON_ERROR_STOP=1 -v db="$fixture_name"
require_available_port "$google_port" "e2e: port not owned by child"
"$tmp_dir/fakegoogle" -listen "127.0.0.1:$google_port" -client-id "$GOOGLE_CLIENT_ID" -client-secret "$GOOGLE_CLIENT_SECRET" -redirect-uri "$SERVER_PUBLIC_URL/api/v1/auth/google/callback" >"$tmp_dir/google.log" 2>&1 &
google_pid=$!
wait_for() {
  local url="$1" pid="$2" port="$3" ready_line="${4:-}" log_file="${5:-$tmp_dir/google.log}"
  local attempt=0 owners lsof_status
  while (( attempt < 60 )); do
    attempt=$((attempt + 1))
    if ! kill -0 "$pid" 2>/dev/null; then
      echo "e2e: port not owned by child" >&2
      return 1
    fi
    lsof_status=0
    owners="$(lsof -nP -iTCP:"$port" -sTCP:LISTEN -t 2>/dev/null)" || lsof_status=$?
    if (( lsof_status > 1 )) || { (( lsof_status != 0 )) && [[ -n "$owners" ]]; } || [[ -n "$owners" && "$owners" != "$pid" ]]; then
      echo "e2e: port not owned by child" >&2
      return 1
    fi
    if curl -fsS "$url" >/dev/null 2>&1; then
      lsof_status=0
      owners="$(lsof -nP -iTCP:"$port" -sTCP:LISTEN -t 2>/dev/null)" || lsof_status=$?
      if (( lsof_status != 0 )) || [[ "$owners" != "$pid" ]] || ! kill -0 "$pid" 2>/dev/null; then
        echo "e2e: port not owned by child" >&2
        return 1
      fi
      if [[ -z "$ready_line" ]] || grep -Fxq "$ready_line" "$log_file"; then
        return 0
      fi
    fi
    sleep 0.5
  done
  return 1
}
if ! wait_for "$SERVER_GOOGLE_ISSUER/.well-known/openid-configuration" "$google_pid" "$google_port" "fakegoogle listening on 127.0.0.1:$google_port"; then
  echo "e2e: fake Google did not start" >&2
  exit 1
fi
require_available_port "$embed_port" "e2e: port not owned by child"
"$tmp_dir/fakeembed" -listen "127.0.0.1:$embed_port" >"$tmp_dir/embed.log" 2>&1 &
embed_pid=$!
if ! wait_for "$SERVER_QUERY_EMBEDDER_URL/readyz" "$embed_pid" "$embed_port" "fakeembed listening on 127.0.0.1:$embed_port" "$tmp_dir/embed.log"; then
  echo "e2e: fake embedder did not start" >&2
  exit 1
fi
require_available_port "$server_port" "e2e: port not owned by child"
"$tmp_dir/server" serve >"$tmp_dir/server.log" 2>&1 &
server_pid=$!
if ! wait_for "$SERVER_PUBLIC_URL/healthz" "$server_pid" "$server_port"; then
  echo "e2e: server did not start" >&2
  exit 1
fi
go test -tags e2e -count=1 ./apps/server/e2e/...
# Future hook: start Nuxt and run Playwright here.
