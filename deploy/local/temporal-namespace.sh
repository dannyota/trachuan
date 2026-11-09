#!/usr/bin/env bash
# Create the corpus namespace with 30-day retention when it is missing.
set -Eeuo pipefail
image=docker.io/temporalio/admin-tools:1.32.2
network="${CORPUS_COMPOSE_NETWORK:-trachuan_default}"
for _ in $(seq 1 30); do
  if podman run --rm --network "$network" "$image" temporal operator cluster health --address temporal:7233 >/dev/null 2>&1; then
    break
  fi
  sleep 2
done
podman run --rm --network "$network" "$image" sh -ec '
  temporal operator namespace describe --namespace corpus --address temporal:7233 >/dev/null 2>&1 ||
    temporal operator namespace create --namespace corpus --retention 30d --address temporal:7233
'
echo "temporal-namespace: ok"
