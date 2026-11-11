# Run the server locally

Start native PostgreSQL and the query service as described in the
[corpus runbook](corpus-local.md). Build and evaluate a corpus database before
selecting it. Create a separate local auth database and use a database role with
access to it. Use a corpus role with read-only access to the corpus database.

Set these environment variables in your shell. Supply local credentials by your
usual secure method, without saving them in this repository.

```sh
export SERVER_AUTH_DATABASE_URL='postgres://AUTH_ROLE@127.0.0.1:25433/auth'
export SERVER_CORPUS_DATABASE_URL='postgres://CORPUS_READER@127.0.0.1:25433/{database}'
export SERVER_PUBLIC_URL='http://127.0.0.1:8080'
```

`SERVER_LISTEN` defaults to `127.0.0.1:8080` and `SERVER_QUERY_EMBEDDER_URL`
defaults to `http://127.0.0.1:8091`. The corpus URL must contain `{database}` as
its database name. Configure authentication for both roles outside the URLs if
possible.

Apply auth migrations and select the built database. Replace the database name
with the actual local release name before executing the SQL.

```sh
go run ./apps/server/cmd/server migrate
psql "$SERVER_AUTH_DATABASE_URL" -c "INSERT INTO corpus_release (country, active_db, activated_at, activated_by) VALUES ('vn', 'corpus_vn_20990101', now(), 'local') ON CONFLICT (country) DO UPDATE SET active_db = EXCLUDED.active_db, activated_at = EXCLUDED.activated_at, activated_by = EXCLUDED.activated_by"
make server-run
```

Create an account and a personal token from another terminal using the same
environment. The account command prints its numeric ID. The token command prints
the token once; keep it out of logs and shell history. Replace the placeholders.

```sh
go run ./apps/server/cmd/server admin account create --google-sub SUBJECT --email EMAIL
go run ./apps/server/cmd/server admin token create --account ACCOUNT_ID --name local --days 90
```

Use `admin account block --account ACCOUNT_ID` to block an account and
`admin token revoke --hash TOKEN_SHA256` to revoke a token. The hash is the
lowercase SHA-256 digest of the full token. Do not put a token on a command
line. Revocation reaches other server processes within ten seconds.

`GET /healthz` checks that the HTTP server is running. `GET /readyz` checks the
auth database and opens the active release engine, including its query recipe
check. `/mcp` requires the bearer token.
