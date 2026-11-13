#!/usr/bin/env bash
set -euo pipefail
container=${1:?usage: wait-healthy.sh container}
deadline=$((SECONDS + 120))
while ((SECONDS < deadline)); do
 if podman healthcheck run "$container" >/dev/null 2>&1; then exit 0; fi
 sleep 1
done
echo "health check timed out: $container" >&2
exit 1
