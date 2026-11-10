#!/usr/bin/env bash
# Native Temporal development server for local runs: 127.0.0.1:27233, UI on 28233,
# state in a SQLite file under TRACHUAN_TEMPORAL_DIR, namespace corpus with 30-day
# retention. Usage: temporal.sh up|down|status
set -Eeuo pipefail
dir="${TRACHUAN_TEMPORAL_DIR:-$HOME/.local/share/trachuan-temporal}"
pidfile="$dir/server.pid"
command -v temporal >/dev/null || { echo "temporal: CLI missing; brew install temporal" >&2; exit 1; }
running() { [ -f "$pidfile" ] && kill -0 "$(cat "$pidfile")" 2>/dev/null; }
case "${1:-}" in
up)
  mkdir -p "$dir"
  if ! running; then
    # SQL visibility needs fresh search attributes on read in a single-node setup.
    nohup temporal server start-dev --ip 127.0.0.1 --port 27233 --ui-port 28233 \
      --db-filename "$dir/temporal.db" --namespace corpus --log-level warn \
      --dynamic-config-value system.forceSearchAttributesCacheRefreshOnRead=true \
      >>"$dir/server.log" 2>&1 &
    echo $! >"$pidfile"
  fi
  for _ in $(seq 1 60); do
    temporal operator cluster health --address 127.0.0.1:27233 >/dev/null 2>&1 && break
    sleep 1
  done
  temporal operator namespace update --namespace corpus --retention 720h --address 127.0.0.1:27233 >/dev/null
  echo "temporal: up (UI http://127.0.0.1:28233, state $dir)"
  ;;
down)
  if running; then kill "$(cat "$pidfile")"; fi
  rm -f "$pidfile"
  echo "temporal: down"
  ;;
status) running && echo "temporal: running (pid $(cat "$pidfile"))" || { echo "temporal: stopped"; exit 1; } ;;
*) echo "usage: temporal.sh up|down|status" >&2; exit 2 ;;
esac
