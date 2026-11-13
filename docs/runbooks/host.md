# Run the production host

Phase 1 serves only the owner. Set the owner's verified email in
`SERVER_SIGNUP_EMAILS` on the host, never in Git. Use a server image containing
that gate and prove a previously unused second Google account is rejected before
announcing the site. Keep Google's consent screen in [Testing][google-testing]
as an additional measure; it does not enforce owner-only access. Before
admitting another account, finish the signed deployment, Vault, backups, alarms,
data volume and infrastructure work in the
[deployment design](../design/deployment.md).

## Build and install

Run the `Images` workflow on `main`, or use a protected `v*` tag. The primary
checks green CI on the exact commit and its three build attestations, then
selects digest references from the successful workflow summary. Make the GHCR
packages public before anonymous pulls. Keep weights and secrets out of images.
Copy the bundle from the MacBook:

```sh
rsync -a deploy/host/ opc@trachuan-host:/tmp/trachuan-host/
```

On the host, fill the three digests in `/tmp/trachuan-host/images.env`, and the
public `GOOGLE_CLIENT_ID` and private owner `SERVER_SIGNUP_EMAILS` in that
bundle's `server.env`. Register Google redirect URI
`https://trachuan.vn/api/v1/auth/google/callback`. From the `opc` login:

```sh
sudo bash /tmp/trachuan-host/install.sh /tmp/trachuan-host
```

The installer targets OL9.8 arm64, uses only OL9 BaseOS and AppStream, requires
SELinux enforcing, cgroup v2 and Podman >= 4.4, and verifies generated units. It
enables lingering and opens 80/443 TCP and 443 UDP. It preserves installed env
files under `/srv/trachuan/host/`; update those files on an existing host. The
server's pre-start guard rejects a missing or empty signup list without logging
its value. Leave `SERVER_GOOGLE_ISSUER` and `SERVER_OAUTH_CLIENTS` unset.

Phase 1 has no instance dynamic group or IAM policy, so the instance principal
grants nothing. Before any Vault policy exists, block metadata access at
`169.254.169.254` for `trachuan` and its mapped container UIDs.

The renderer writes literal digests into the user's [Quadlet
directory][quadlet]. All containers use host networking. Only Caddy binds public
addresses on 80/443 TCP and 443 UDP. Go binds `127.0.0.1:8080`, Nitro
`127.0.0.1:3000`, queryembed `127.0.0.1:8091`, PostgreSQL `127.0.0.1:5432`, and
Caddy health `127.0.0.1:8081`. Both trusted-proxy variables are `127.0.0.1/32`.
Go needs outbound HTTPS for Google and arbitrary CIMD hosts; PostgreSQL and
queryembed need none. The workflow verifies Nitro's actual image listener
without runtime overrides; `NITRO_HOST` and `NITRO_PORT` are explicit in the
image and env file.

## Provision the model

On the MacBook, set `BUNDLE` to the verified bundle used for the evaluated
corpus. Copy only these serving files, preserving relative paths:

```sh
BUNDLE=work/models/vi_v2-bundle
rsync -aR "$BUNDLE/./bundle.json" "$BUNDLE/./config.json" \
  "$BUNDLE/./tokenizer.json" "$BUNDLE/./tokenizer_config.json" \
  "$BUNDLE/./special_tokens_map.json" "$BUNDLE/./sentencepiece.bpe.model" \
  "$BUNDLE/./onnx/model_int8.onnx" opc@trachuan-host:/tmp/vi_v2-bundle/
```

On the host, before starting queryembed:

```sh
sudo rsync -a --chown=trachuan:trachuan /tmp/vi_v2-bundle/ \
  /srv/trachuan/models/vi_v2-bundle/
sudo find /srv/trachuan/models/vi_v2-bundle -type d -exec chmod 0755 {} +
sudo find /srv/trachuan/models/vi_v2-bundle -type f -exec chmod 0644 {} +
sudo restorecon -RF /srv/trachuan/models
```

Queryembed mounts `/models` read-only with a private SELinux label. Startup
verifies every loaded model and tokenizer file against `bundle.json`. Weights
stay off images; the native libraries are verified against `native.lock` at
build time. Changing the bundle requires a maintenance window with matching
images and corpus. Preserve the bundle's contents and manifest.

## Create secrets

The owner uses an interactive Bash terminal. Silent reads keep values out of
arguments, repository files and history. Keep a recovery copy outside the host.

```sh
sudo -iu trachuan
export XDG_RUNTIME_DIR=/run/user/$(id -u)
set -euo pipefail
set +x
set +o history
secret() {
  local value
  IFS= read -rs -p "$1: " value </dev/tty; printf '\n' >&2
  [[ -n $value ]] || return 1
  printf '%s' "$value" | podman secret create "$1" - >/dev/null
  unset value
}
db_secret() {
  local name=$1 role=$2 database=$3 url_secret=$4 value
  IFS= read -rs -p "$name: " value </dev/tty; printf '\n' >&2
  [[ -n $value ]] || return 1
  printf '%s' "$value" | podman secret create "$name" - >/dev/null
  printf '%s' "$value" | python3 -c '
import sys, urllib.parse
password = urllib.parse.quote(sys.stdin.read(), safe="")
sys.stdout.write("postgres://" + sys.argv[1] + ":" + password +
                 "@127.0.0.1:5432/" + sys.argv[2] + "?sslmode=disable")
' "$role" "$database" | podman secret create "$url_secret" - >/dev/null
  unset value
}
secret google_client_secret
secret postgres_password
db_secret auth_db_password auth_owner auth server_auth_database_url
db_secret corpus_reader_password corpus_reader '{database}' server_corpus_database_url
unset -f secret db_secret
```

Use nonempty passwords without newlines. The derived URL secrets supply the
server's existing variables. No token signing key exists. To rotate passwords,
stop affected containers, update database roles and both corresponding secrets,
then recreate containers. Initialization secrets do not change existing roles.

## First start and database authentication

Continue as `trachuan`. Pull the five pinned images without sourcing the file:

```sh
while IFS='=' read -r key image; do
  case "$key" in *_IMAGE) podman pull "$image" ;; esac
done </srv/trachuan/host/images.env
systemctl --user daemon-reload
systemctl --user start trachuan-postgres
/srv/trachuan/host/check-postgres-auth.sh
systemctl --user start trachuan-caddy
systemctl --user status trachuan-postgres trachuan-migrate \
  trachuan-queryembed trachuan-server trachuan-web trachuan-caddy
```

Initialization requires SCRAM for every TCP and socket connection. Bootstrap
creates `auth`, `auth_owner` and `corpus_reader` using mounted passwords. The
check first proves authenticated access, then proves that passwordless
`postgres` connections to `127.0.0.1:5432` and the socket fail specifically for
lack of a password. Migrations require that check before `server migrate` runs.
All admin commands below use `postgres-admin.sh`, which reads the mounted
password into its environment. Do not substitute password arguments or trust.

The named PostgreSQL volume initially uses rootless storage on boot disk. Never
remove it to retry initialization. Move data to the separate volume before
another account signs in. `/readyz` stays 503 until an active corpus passes its
recipe check; [server-local.md](server-local.md) owns server command behavior.

### Repair an initialized cluster

Changing init flags does not repair an existing cluster. For a cluster made by
an earlier bundle, keep traffic stopped and run the installed helper as
`trachuan`. It atomically rewrites `pg_hba.conf` to SCRAM and reloads
PostgreSQL. The mounted passwords must match existing roles. See the [HBA reload
rules][hba].

```sh
systemctl --user stop trachuan-caddy trachuan-web trachuan-server trachuan-migrate
podman exec trachuan-postgres /usr/local/bin/repair-postgres-auth.sh
/srv/trachuan/host/check-postgres-auth.sh
systemctl --user start trachuan-server trachuan-web
curl --fail http://127.0.0.1:8080/readyz
systemctl --user start trachuan-caddy
```

## Restore and select a corpus release

Use a trusted, evaluated PostgreSQL 18 dump whose recorded checksum the primary
has verified. Stop on unexpected functions, event triggers or extensions in the
TOC. Create a fresh database; do not overwrite an existing release:

```sh
systemctl --user stop trachuan-caddy
DUMP=/path/to/verified/corpus.dump
DB=corpus_vn_20261112_2
podman exec -i trachuan-postgres pg_restore --list <"$DUMP" >/tmp/corpus-toc.txt
```

Inspect `/tmp/corpus-toc.txt` before continuing. The restore uses 2 GB and two
maintenance workers because 512 MB spilled in the measured HNSW rebuild:

```sh
podman exec trachuan-postgres /usr/local/bin/postgres-admin.sh createdb -U postgres -O postgres "$DB"
podman exec -i \
  --env 'PGOPTIONS=-c maintenance_work_mem=2GB -c max_parallel_maintenance_workers=2' \
  trachuan-postgres /usr/local/bin/postgres-admin.sh pg_restore -U postgres \
  --exit-on-error --no-owner --no-privileges --dbname "$DB" <"$DUMP"
podman exec trachuan-postgres bash /usr/local/bin/grant-corpus.sh "$DB"
podman exec -i trachuan-postgres /usr/local/bin/postgres-admin.sh \
  psql -X -v ON_ERROR_STOP=1 -U postgres -d auth -v database="$DB" <<'SQL'
INSERT INTO corpus_release (country, active_db, activated_at, activated_by)
VALUES ('vn', :'database', now(), 'owner')
ON CONFLICT (country) DO UPDATE SET active_db = EXCLUDED.active_db,
activated_at = EXCLUDED.activated_at, activated_by = EXCLUDED.activated_by;
SQL
curl --fail http://127.0.0.1:8080/readyz
systemctl --user start trachuan-caddy
```

Start Caddy only after readiness passes. On a recipe mismatch, keep traffic
stopped and select the previous compatible database or matching serving bundle.
Keep the previous database. This manual phase does not implement signed release
activation. PostgreSQL has 3 GiB of shared-memory capacity within its existing 8
GiB memory cap; shared memory is charged to that cap, not an extra allowance.

## Deploy a new digest

Check green CI for the exact commit and all three attestations. Keep the old
`images.env` outside Git for recovery. Edit `/srv/trachuan/host/images.env`:

```sh
bash /srv/trachuan/host/render-units.sh /srv/trachuan/host/images.env \
  /home/trachuan/.config/containers/systemd
while IFS='=' read -r key image; do
  case "$key" in *_IMAGE) podman pull "$image" ;; esac
done </srv/trachuan/host/images.env
systemctl --user stop trachuan-caddy trachuan-web trachuan-server
systemctl --user daemon-reload
systemctl --user restart trachuan-queryembed trachuan-migrate
systemctl --user start trachuan-server trachuan-web
curl --fail http://127.0.0.1:8080/readyz
systemctl --user start trachuan-caddy
```

Recipe changes also need matching models and corpus while traffic is stopped.
Database version changes need a separate upgrade. Check schema and recipe
compatibility before any rollback; an old digest alone is not a safe rollback.

## Health and logs

```sh
curl --fail http://127.0.0.1:8080/healthz
curl --fail http://127.0.0.1:8080/readyz
curl --fail http://127.0.0.1:8091/healthz
curl --fail http://127.0.0.1:8081/healthz
podman ps --format '{{.Names}} {{.Status}}'
sudo ss -lntup '( sport = :80 or sport = :443 or sport = :3000 or sport = :5432 or sport = :8080 or sport = :8081 or sport = :8091 )'
journalctl --user -u trachuan-server -u trachuan-queryembed -n 100
journalctl --user -u trachuan-postgres -u trachuan-migrate -n 100
```

Confirm Let's Encrypt issued the certificate, `www` preserves the path, and only
Caddy's 80/443 listeners use public addresses. Web uses TCP health on 3000 to
avoid Nitro admission; PostgreSQL uses authenticated `pg_isready`. The edge
preserves Host and Origin. Public `/healthz`, `/readyz` and unrelated
`/.well-known/*` paths must reach web's 404.

## Prove the client address

Access logging is disabled by default. As `opc`, run this proof from the same
IPv4 network as the SSH session. It temporarily enables the snippet, compares
Caddy's peer with the caller, then disables logging and removes the active and
all rotated logs, including compressed files, even when the comparison fails:

```sh
(
  set -euo pipefail
  trap 'sudo -iu trachuan /srv/trachuan/host/client-address-log.sh disable || exit 1' EXIT
  trap 'exit 1' HUP INT TERM
  sudo -iu trachuan /srv/trachuan/host/client-address-log.sh enable
  echo 'From the workstation: curl -4 -sS -o /dev/null https://trachuan.vn/__client-address-check'
  read -r -p 'Press Enter after the request: ' proof_ready </dev/tty
  caller_ip=${SSH_CONNECTION%% *}
  seen_ip=$(sudo -iu trachuan podman exec trachuan-caddy \
    cat /var/log/caddy-proof/client-address-check.json | python3 -c '
import json, sys
rows = [json.loads(line) for line in sys.stdin if line.strip()]
print(rows[-1]["request"]["remote_ip"])
')
  test -n "$caller_ip" && test "$seen_ip" = "$caller_ip"
  echo 'client address proof passed'
)
```

The web catch-all returns 404 for this probe. The snippet and logs live in the
user runtime directory and disappear on reboot. If the shell is killed without
cleanup, run `client-address-log.sh disable` as `trachuan`. Accept traffic only
after the comparison and cleanup succeed. Never copy addresses or logs into Git
or reports.

[quadlet]: https://docs.podman.io/en/latest/markdown/podman-systemd.unit.5.html
[hba]: https://www.postgresql.org/docs/18/auth-pg-hba-conf.html
[google-testing]:
  https://developers.google.com/identity/protocols/oauth2/production-readiness/overview
