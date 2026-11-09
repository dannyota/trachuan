#!/usr/bin/env bash
# Create or upgrade the Temporal schemas with admin-tools. Idempotent.
set -Eeuo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/../.."
set -a
# shellcheck disable=SC1091
. ./.env
set +a
: "${TEMPORAL_DB_PASSWORD:?set TEMPORAL_DB_PASSWORD in .env}"
export SQL_PASSWORD="$TEMPORAL_DB_PASSWORD"
image=docker.io/temporalio/admin-tools:1.32.2
network="${CORPUS_COMPOSE_NETWORK:-trachuan_default}"
schema=/etc/temporal/schema/postgresql/v12
for target in temporal:temporal visibility:temporal_visibility; do
  dir=${target%%:*}
  db=${target#*:}
  podman run --rm --network "$network" -e SQL_PASSWORD "$image" sh -ec "
    tool='temporal-sql-tool --plugin postgres12 --ep postgres -p 5432 -u temporal --db $db'
    \$tool setup-schema -v 0.0 >/dev/null 2>&1 || true
    \$tool update-schema -d $schema/$dir/versioned
  "
done
echo "temporal-schema: ok"
