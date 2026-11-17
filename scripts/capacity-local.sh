#!/usr/bin/env bash
set -euo pipefail
umask 077
unset PGHOSTADDR PGHOST PGPORT PGSERVICE PGSERVICEFILE PGSYSCONFDIR PGDATABASE PGOPTIONS
cd "$(dirname "${BASH_SOURCE[0]}")/.."

out_dir=""
phases="A,B,C,D"
short_smoke=0
while (( $# )); do
  case "$1" in
    --out) [[ $# -ge 2 ]] || { echo 'capacity: --out requires a directory' >&2; exit 1; }; out_dir="$2"; shift 2 ;;
    --phases) [[ $# -ge 2 ]] || { echo 'capacity: --phases requires a value' >&2; exit 1; }; phases="$2"; shift 2 ;;
    --short-smoke) short_smoke=1; shift ;;
    *) echo 'capacity: unknown option' >&2; exit 1 ;;
  esac
done
if [[ -z "$out_dir" ]]; then
  echo 'capacity: --out is required' >&2
  exit 1
fi
mkdir -p "$out_dir"
out_dir="$(cd "$out_dir" && pwd -P)"
for name in queries.txt samples.jsonl requests.jsonl report.json gates.json server.log web.log google.log edge.log sampler.log; do
  if [[ -e "$out_dir/$name" || -L "$out_dir/$name" ]]; then
    echo 'capacity: output already exists' >&2
    exit 1
  fi
done
export CORPUS_TEST_ADMIN_URL="${CORPUS_TEST_ADMIN_URL:-postgres://trachuan@127.0.0.1:25433/postgres}"
server_port="${E2E_SERVER_PORT:-28080}"
google_port="${E2E_GOOGLE_PORT:-28081}"
edge_port="${E2E_EDGE_PORT:-28083}"
web_port="${E2E_WEB_PORT:-28084}"
ports=("$server_port" "$google_port" "$edge_port" "$web_port" 8091)
for port in "${ports[@]}"; do
  if [[ ! "$port" =~ ^[1-9][0-9]{0,4}$ ]] || (( port > 65535 )); then
    echo 'capacity: invalid port' >&2
    exit 1
  fi
done
for ((i = 0; i < ${#ports[@]}; i++)); do
  for ((j = i + 1; j < ${#ports[@]}; j++)); do
    if [[ "${ports[i]}" == "${ports[j]}" ]]; then
      echo 'capacity: ports must differ' >&2
      exit 1
    fi
  done
done

db_name="trachuan_capacity_$(od -An -N6 -tx1 /dev/urandom | tr -d ' \n')"
if [[ ! "$db_name" =~ ^trachuan_capacity_[0-9a-f]{12}$ ]]; then
  echo 'capacity: invalid temporary database name' >&2
  exit 1
fi
database_url() {
  python3 - "$1" "$2" <<'PY'
import ipaddress
import os
import sys
from urllib.parse import parse_qsl, urlencode, urlsplit, urlunsplit
try:
    url = urlsplit(os.environ["CORPUS_TEST_ADMIN_URL"])
    host = url.hostname or ""
    loopback = host == "localhost" or ipaddress.ip_address(host).is_loopback
    params = parse_qsl(url.query, keep_blank_values=True, strict_parsing=True)
    if url.scheme not in ("postgres", "postgresql") or not loopback or any(k not in {"sslmode", "connect_timeout"} for k, _ in params):
        raise ValueError()
    if sys.argv[2] == "read-only":
        params.append(("default_transaction_read_only", "on"))
    print(urlunsplit((url.scheme, url.netloc, "/" + sys.argv[1], urlencode(params), "")))
except (ValueError, KeyError):
    sys.exit("capacity: invalid local administrator URL")
PY
}
auth_url="$(database_url "$db_name" write)"
corpus_url="$(database_url '{database}' read-only)"
query_url="$(database_url corpus_vn_20261117_7 read-only)"
port_available() {
  python3 - "$1" <<'PY'
import errno
import socket
import sys
with socket.socket() as connection:
    connection.settimeout(0.2)
    result = connection.connect_ex(("127.0.0.1", int(sys.argv[1])))
sys.exit(0 if result == errno.ECONNREFUSED else 1)
PY
}
require_available_port() {
  if ! port_available "$1"; then
    echo 'capacity: port in use' >&2
    exit 1
  fi
}
for port in "$server_port" "$google_port" "$edge_port" "$web_port"; do
  require_available_port "$port"
done
if ! command -v lsof >/dev/null 2>&1; then
  echo 'capacity: lsof is required' >&2
  exit 1
fi
if ! curl -fsS --max-time 3 'http://127.0.0.1:8091/readyz' >/dev/null; then
  echo 'capacity: query embedder is not ready' >&2
  exit 1
fi
tmp_dir="$(mktemp -d "${TMPDIR:-/tmp}/trachuan-capacity.XXXXXXXX")"
if [[ ! "$tmp_dir" =~ ^[a-zA-Z0-9_./-]+$ ]]; then
  rmdir -- "$tmp_dir"
  echo 'capacity: temporary path contains unsupported characters' >&2
  exit 1
fi
server_pid=""
google_pid=""
web_pid=""
edge_pid=""
sampler_pid=""
db_created=0
cleanup() {
  local status=$?
  trap - EXIT
  for pid in "$sampler_pid" "$edge_pid" "$web_pid" "$server_pid" "$google_pid"; do
    if [[ -n "$pid" ]]; then kill -TERM "$pid" 2>/dev/null || true; wait "$pid" 2>/dev/null || true; fi
  done
  if [[ "$db_created" == 1 && "$db_name" =~ ^trachuan_capacity_[0-9a-f]{12}$ ]]; then
    if ! printf 'DROP DATABASE IF EXISTS :"db" WITH (FORCE)\n' | psql -X "$CORPUS_TEST_ADMIN_URL" -v ON_ERROR_STOP=1 -v db="$db_name"; then
      echo 'capacity: database cleanup failed' >&2
      status=1
    fi
  fi
  rm -r -- "$tmp_dir" || status=1
  exit "$status"
}
trap cleanup EXIT
trap 'exit 1' INT TERM

go build -o "$tmp_dir/capacity-server" ./apps/server/cmd/server
go build -o "$tmp_dir/fakegoogle" ./apps/server/cmd/fakegoogle
go build -o "$tmp_dir/loadcheck" ./apps/server/cmd/loadcheck
printf 'CREATE DATABASE :"db"\n' | psql -X "$CORPUS_TEST_ADMIN_URL" -v ON_ERROR_STOP=1 -v db="$db_name"
db_created=1
export SERVER_AUTH_DATABASE_URL="$auth_url"
export SERVER_CORPUS_DATABASE_URL="$corpus_url"
export SERVER_LISTEN="127.0.0.1:$server_port"
export SERVER_TRUSTED_PROXIES='127.0.0.1/32'
export SERVER_PUBLIC_URL="http://localhost:$edge_port"
export SERVER_QUERY_EMBEDDER_URL='http://127.0.0.1:8091'
export SERVER_PUBLIC_SUMMARIES=true
export SERVER_ACCOUNT_KEYS_FILE="$tmp_dir/account_keys"
export SERVER_MAIL_CAPTURE_DIR="$tmp_dir/mail"
export SERVER_TOTP_ENROLLMENT=true
unset SERVER_SMTP_HOST SERVER_SMTP_USERNAME SERVER_SMTP_PASSWORD
openssl rand -base64 32 | tr '/+' '_-' > "$SERVER_ACCOUNT_KEYS_FILE"
chmod 600 "$SERVER_ACCOUNT_KEYS_FILE"
mkdir -m 700 "$SERVER_MAIL_CAPTURE_DIR"
export SERVER_GOOGLE_ISSUER="http://127.0.0.1:$google_port"
export GOOGLE_CLIENT_ID='e2e-client'
export GOOGLE_CLIENT_SECRET='e2e-secret'
"$tmp_dir/capacity-server" migrate
printf "INSERT INTO corpus_release (country,active_db,activated_at,activated_by,summary_database,summary_projection,headings_approved) VALUES ('vn', 'corpus_vn_20261117_7', now(), 'capacity-local', 'corpus_vn_20261117_7', 1, true)\n" | psql -X "$auth_url" -v ON_ERROR_STOP=1
cat > "$tmp_dir/activate" <<'ACTIVATE'
#!/usr/bin/env bash
set -euo pipefail
current="$(psql -X "$SERVER_AUTH_DATABASE_URL" -At -v ON_ERROR_STOP=1 -c "SELECT active_db FROM corpus_release WHERE country = 'vn'")"
case "$current" in
  corpus_vn_20261117_7) target='corpus_vn_20261117_5' ;;
  corpus_vn_20261117_5) target='corpus_vn_20261117_7' ;;
  *) echo 'capacity: unexpected active release' >&2; exit 1 ;;
esac
changed="$(psql -X "$SERVER_AUTH_DATABASE_URL" -At -v ON_ERROR_STOP=1 -v expected="$current" -v target="$target" <<'SQL'
WITH swapped AS (
  UPDATE corpus_release SET active_db = :'target', summary_database = :'target', summary_projection = 1, headings_approved = true, activated_at = now(), activated_by = 'capacity-local'
  WHERE country = 'vn' AND active_db = :'expected' AND active_db = summary_database AND summary_projection = 1 AND headings_approved = true
  RETURNING 1
)
SELECT count(*) FROM swapped;
SQL
)"
if [[ "$changed" != 1 ]]; then
  echo 'capacity: release compare-and-swap failed' >&2
  exit 1
fi
ACTIVATE
chmod 700 "$tmp_dir/activate"
wait_for() {
  local url="$1" pid="$2" port="$3" owners lsof_status attempt=0
  while (( attempt < 60 )); do
    attempt=$((attempt + 1))
    if ! kill -0 "$pid" 2>/dev/null; then return 1; fi
    lsof_status=0
    owners="$(lsof -nP -iTCP:"$port" -sTCP:LISTEN -t 2>/dev/null)" || lsof_status=$?
    if (( lsof_status > 1 )) || { (( lsof_status != 0 )) && [[ -n "$owners" ]]; } || [[ -n "$owners" && "$owners" != "$pid" ]]; then return 1; fi
    if curl -fsS --max-time 2 "$url" >/dev/null 2>&1; then
      lsof_status=0
      owners="$(lsof -nP -iTCP:"$port" -sTCP:LISTEN -t 2>/dev/null)" || lsof_status=$?
      if (( lsof_status == 0 )) && [[ "$owners" == "$pid" ]] && kill -0 "$pid" 2>/dev/null; then return 0; fi
      return 1
    fi
    sleep 0.5
  done
  return 1
}
require_available_port "$google_port"
"$tmp_dir/fakegoogle" -listen "127.0.0.1:$google_port" -client-id "$GOOGLE_CLIENT_ID" -client-secret "$GOOGLE_CLIENT_SECRET" -redirect-uri "$SERVER_PUBLIC_URL/api/v1/auth/google/callback" >"$out_dir/google.log" 2>&1 &
google_pid=$!
wait_for "$SERVER_GOOGLE_ISSUER/.well-known/openid-configuration" "$google_pid" "$google_port" || { echo 'capacity: fake Google failed' >&2; exit 1; }
require_available_port "$server_port"
"$tmp_dir/capacity-server" serve >"$out_dir/server.log" 2>&1 &
server_pid=$!
wait_for "http://127.0.0.1:$server_port/healthz" "$server_pid" "$server_port" || { echo 'capacity: server failed' >&2; exit 1; }
if [[ "${E2E_WEB_PREBUILT:-0}" != 1 ]]; then (cd apps/web && npm run build); fi
ln -s "$(command -v node)" "$tmp_dir/capacity-web"
require_available_port "$web_port"
(cd apps/web && exec env NITRO_HOST=127.0.0.1 NITRO_PORT="$web_port" NUXT_API_ORIGIN="http://127.0.0.1:$server_port" NUXT_TRUSTED_PROXIES=127.0.0.1/32 "$tmp_dir/capacity-web" .output/server/index.mjs) >"$out_dir/web.log" 2>&1 &
web_pid=$!
wait_for "http://127.0.0.1:$web_port/" "$web_pid" "$web_port" || { echo 'capacity: web failed' >&2; exit 1; }
require_available_port "$edge_port"
(cd apps/web && exec env E2E_EDGE_PORT="$edge_port" E2E_SERVER_PORT="$server_port" E2E_WEB_PORT="$web_port" node e2e/edge.mjs) >"$out_dir/edge.log" 2>&1 &
edge_pid=$!
wait_for "http://127.0.0.1:$edge_port/" "$edge_pid" "$edge_port" || { echo 'capacity: edge failed' >&2; exit 1; }
"$tmp_dir/loadcheck" queries --corpus-db "$query_url" --suite apps/corpus/internal/evaluation/testdata/vn --out "$out_dir/queries.txt"
"$tmp_dir/loadcheck" sample --pg "$auth_url" --proc 'server=(^|/)capacity-server$' --proc 'web=(^|/)capacity-web$' --proc 'postgres=(^|/)postgres$' --out "$out_dir/samples.jsonl" >"$out_dir/sampler.log" 2>&1 &
sampler_pid=$!
sampler_ready=0
for ((attempt = 0; attempt < 50; attempt++)); do
  if ! kill -0 "$sampler_pid" 2>/dev/null; then break; fi
  if [[ -s "$out_dir/samples.jsonl" ]]; then sampler_ready=1; break; fi
  sleep 0.1
done
if (( ! sampler_ready )); then
  echo 'capacity: sampler failed to start' >&2
  exit 1
fi
run_args=(run --pages "$SERVER_PUBLIC_URL" --api "$SERVER_PUBLIC_URL" --public-url "$SERVER_PUBLIC_URL" --google "$SERVER_GOOGLE_ISSUER" --emails 'one@example.invalid,two@example.invalid,three@example.invalid' --queries "$out_dir/queries.txt" --phases "$phases" --out "$out_dir" --activate "$tmp_dir/activate")
if (( short_smoke )); then run_args+=(--duration-a 5s --duration-c 5s --duration-d 5s); fi
"$tmp_dir/loadcheck" "${run_args[@]}"
sleep 61
kill -TERM "$sampler_pid"
wait "$sampler_pid"
sampler_pid=""
if [[ ",$phases," == *",C,"* ]] && ! grep -Eq '"msg"[[:space:]]*:[[:space:]]*"summary render"' "$out_dir/web.log"; then
  echo 'capacity: summary render log missing' >&2
  exit 1
fi
"$tmp_dir/loadcheck" report --run "$out_dir" --samples "$out_dir/samples.jsonl" --web-log "$out_dir/web.log" --server-log "$out_dir/server.log" --out "$out_dir/gates.json"
