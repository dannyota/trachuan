# Run the server locally

Start native PostgreSQL and the query service as described in the
[corpus runbook](corpus-local.md). Build and evaluate a corpus database before
selecting it. Create a separate local auth database and use a database role with
access to it. Use a corpus role with read-only access to the corpus database.

Create a
[Google OAuth web application client](https://developers.google.com/identity/protocols/oauth2/web-server#creatingcred)
with redirect URI `http://localhost:8080/api/v1/auth/google/callback`. Configure
the consent screen and test users if Google requires them. Put the client ID and
secret in the ignored `.env` as `GOOGLE_CLIENT_ID` and `GOOGLE_CLIENT_SECRET`.
Do not print the secret. Load `.env` into the shell before starting the server:

```sh
set -a; . ./.env; set +a
```

Set these other environment variables in your shell. Supply database credentials
by your usual secure method, without saving them in this repository.

```sh
export SERVER_AUTH_DATABASE_URL='postgres://AUTH_ROLE@127.0.0.1:25433/auth'
export SERVER_CORPUS_DATABASE_URL='postgres://CORPUS_READER@127.0.0.1:25433/{database}'
export SERVER_PUBLIC_URL='http://localhost:8080'
```

`SERVER_LISTEN` defaults to `127.0.0.1:8080` and `SERVER_QUERY_EMBEDDER_URL`
defaults to `http://127.0.0.1:8091`. The corpus URL must contain `{database}` as
its database name. Configure authentication for both roles outside the URLs if
possible. `SERVER_PUBLIC_URL` must be an HTTPS origin, or an HTTP origin on
localhost or a loopback address. Its path must be empty or `/`, with no query or
fragment. `serve` requires both Google variables and `SERVER_PUBLIC_URL`;
`migrate` and `admin` do not. `SERVER_TRUSTED_PROXIES` is an optional
comma-separated list of proxy CIDRs; it defaults to empty.
`SERVER_OAUTH_CLIENTS` optionally names a JSON file of pre-registered clients;
when absent, no clients are pre-registered.

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

`http://localhost` is a
[secure context](https://developer.mozilla.org/en-US/docs/Web/Security/Defenses/Secure_Contexts),
and supporting browsers allow
[Secure cookies on localhost](https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/Set-Cookie).
Check the `__Host-` login and session cookies in Chrome and Firefox during the
end-to-end test. If a browser rejects them, use
[Caddy's local HTTPS proxy](https://caddyserver.com/docs/quick-starts/reverse-proxy):

```sh
caddy reverse-proxy --from localhost:8443 --to 127.0.0.1:8080
```

Trust Caddy's local CA when prompted, register
`https://localhost:8443/api/v1/auth/google/callback` with the Google client, set
`SERVER_PUBLIC_URL=https://localhost:8443`, and restart the server.

## Connect Claude Code

Add the local server using the
[Claude Code MCP command](https://code.claude.com/docs/en/mcp):

```sh
claude mcp add --transport http trachuan http://localhost:8080/mcp
```

In Claude Code, open `/mcp`, select trachuan and follow the browser login. Sign
in with Google and approve the consent page. The local acceptance run must
confirm the connection and tool calls.

To revoke access, use the signed-in account API: `GET /api/v1/me/grants` lists
clients and `DELETE /api/v1/me/grants/{id}` revokes one. Send the session
cookie, matching `Origin` and CSRF token with the DELETE request. Removing the
server from Claude Code alone does not revoke the grant.
