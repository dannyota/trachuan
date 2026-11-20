# Repair access to auth

A compromised or misconfigured server can lock the updater out of `auth`:
deploys stop and the alarm fires. Treat it as a possible compromise: stop the
server first. Then repair from database `postgres`, without connecting to
`auth`, and let the next timer run confirm it:

```sh
podman exec -i --env PGOPTIONS='-c search_path=pg_catalog' \
  --env PGREQUIREAUTH=scram-sha-256 trachuan-postgres \
  /usr/local/bin/postgres-admin.sh \
  psql -X -v ON_ERROR_STOP=1 -h 127.0.0.1 -U postgres -d postgres <<'SQL'
ALTER DATABASE auth ALLOW_CONNECTIONS true CONNECTION LIMIT -1;
ALTER DATABASE auth RESET ALL;
ALTER ROLE auth_owner RESET ALL;
ALTER ROLE auth_owner IN DATABASE auth RESET ALL;
SQL
```

If its password was changed, rotate it as in the
[host runbook](host.md#create-secrets).
