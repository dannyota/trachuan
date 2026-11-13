#!/usr/bin/env bash
# A refused connection is useful only after the same authenticated endpoint works.
set -euo pipefail
fail() { echo "postgres-auth: $*" >&2; exit 1; }
if ! podman exec trachuan-postgres /usr/local/bin/postgres-admin.sh /usr/local/bin/wait-postgres.sh; then
 fail 'database readiness failed'
fi
for host in 127.0.0.1 /var/run/postgresql; do
 if ! result=$(podman exec --env PGCONNECT_TIMEOUT=5 trachuan-postgres /usr/local/bin/postgres-admin.sh \
  psql -X -w -A -t --host "$host" --port 5432 --username postgres --dbname postgres --command 'SELECT 1' 2>/dev/null); then
  fail 'authenticated administration connection failed'
 fi
 [[ $result == 1 ]] || fail 'unexpected administration response'
 if result=$(podman exec --env PGPASSWORD= --env PGPASSFILE=/dev/null \
  --env PGSERVICE= --env PGSERVICEFILE=/dev/null --env PGHOSTADDR= \
  --env PGCONNECT_TIMEOUT=5 --env LC_ALL=C \
  trachuan-postgres psql -X -w -A -t --host "$host" --port 5432 \
  --username postgres --dbname postgres --command 'SELECT 1' 2>&1); then
  fail 'passwordless administration connection was accepted'
 fi
 [[ $result == *'no password supplied'* ]] || fail 'failure was not a missing-password rejection'
done
echo 'postgres-auth: authenticated TCP and socket work; passwordless connections rejected'
