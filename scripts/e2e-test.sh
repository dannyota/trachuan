#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

test_dir="$(mktemp -d "${TMPDIR:-/tmp}/trachuan-e2e-test.XXXXXXXX")"
listener_pid=""
cleanup() {
  if [[ -n "$listener_pid" ]]; then
    kill "$listener_pid" 2>/dev/null || true
    wait "$listener_pid" 2>/dev/null || true
  fi
  rm -r -- "$test_dir"
}
trap cleanup EXIT
trap 'exit 1' INT TERM
mkdir -p "$test_dir/scripts" "$test_dir/bin"
cp scripts/e2e.sh "$test_dir/scripts/e2e.sh"
cat > "$test_dir/bin/go" <<'STUB'
#!/usr/bin/env bash
exit 0
STUB
cat > "$test_dir/bin/psql" <<'STUB'
#!/usr/bin/env bash
touch "$E2E_TEST_PSQL_CALLED"
exit 1
STUB
chmod +x "$test_dir/bin/go" "$test_dir/bin/psql"
python3 - "$test_dir/port" <<'PY' &
import socket
import sys

with socket.socket() as listener:
    listener.bind(("127.0.0.1", 0))
    listener.listen()
    with open(sys.argv[1], "w", encoding="utf-8") as port_file:
        port_file.write(str(listener.getsockname()[1]))
    while True:
        connection, _ = listener.accept()
        connection.close()
PY
listener_pid=$!
for _ in {1..100}; do
  [[ -s "$test_dir/port" ]] && break
  if ! kill -0 "$listener_pid" 2>/dev/null; then
    echo "e2e-test: listener failed" >&2
    exit 1
  fi
  sleep 0.05
done
if [[ ! -s "$test_dir/port" ]]; then
  echo "e2e-test: listener did not start" >&2
  exit 1
fi
server_port="$(cat "$test_dir/port")"
if [[ "$server_port" == 28081 ]]; then google_port=28082; else google_port=28081; fi
export E2E_TEST_PSQL_CALLED="$test_dir/psql-called"
if PATH="$test_dir/bin:$PATH" E2E_SERVER_PORT="$server_port" E2E_GOOGLE_PORT="$google_port" \
  CORPUS_TEST_ADMIN_URL='postgres://localhost/postgres' bash "$test_dir/scripts/e2e.sh" >"$test_dir/output" 2>&1; then
  echo "e2e-test: occupied port was accepted" >&2
  exit 1
fi
if ! grep -Fxq 'e2e: port in use' "$test_dir/output"; then
  echo "e2e-test: missing occupied port error" >&2
  exit 1
fi
if [[ -e "$E2E_TEST_PSQL_CALLED" ]]; then
  echo "e2e-test: psql ran before the port check" >&2
  exit 1
fi
echo 'e2e-test: ok'
