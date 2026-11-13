#!/usr/bin/env bash
# Run inside PostgreSQL with the admin wrapper; bootstrap uses the Unix socket.
set -euo pipefail
host=${1:-127.0.0.1}
timeout=${2:-120}
[[ $# -le 2 && $timeout =~ ^(0|[1-9][0-9]{0,2})$ && $timeout -le 120 ]] || {
 echo 'postgres-wait: timeout must be between 0 and 120 seconds' >&2; exit 1;
}
deadline=$((SECONDS + timeout))
while ((SECONDS < deadline)); do
 if pg_isready --host "$host" --port 5432 --username postgres --dbname postgres --timeout 2 >/dev/null 2>&1; then
  exit 0
 fi
 sleep 1
done
echo 'postgres-wait: readiness timed out' >&2
exit 1
