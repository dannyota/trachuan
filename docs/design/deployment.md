# Deployment and operations

Decision: [ADR 0006](../adr/0006-single-host-oci-podman.md). Reused from
aboutme: OpenTofu structure, host bundle, Quadlet templates, pgBackRest config,
alert scripts, image build with signed provenance, Playwright harness with the
fake Google provider.

## Host

Use the OCI A1 shape and region in ADR 0006. Recommend the standard Oracle Linux
9.8 arm64 platform image, updated from the supported repositories. Oracle's
[arm64 AppStream](https://yum.oracle.com/repo/OracleLinux/OL9/appstream/aarch64/index.html)
lists Podman 5.8.2 for OL9.8, above the
[4.4 Quadlet introduction](https://github.com/podman-container-tools/podman/releases/tag/v4.4.0).
Before production, verify installed versions, cgroup v2, subordinate UID/GID
ranges, SELinux volume labels and rootless user lingering on the actual host.
[Oracle's Quadlet guide](https://docs.oracle.com/en/operating-systems/oracle-linux/podman/quadlets.html)
documents rootless units; use only keys supported by the installed version.

Keep rootless Podman and `net.ipv4.ip_unprivileged_port_start=80`. Replace gp3
with
[Balanced OCI volumes](https://docs.oracle.com/en-us/iaas/Content/Block/Concepts/blockvolumeperformance.htm):
a boot volume and a separate persistent data volume, sized in
[host budgets](budgets-host.md#disk-budget-host-estimates). Balanced performance
scales with size, so validate restore and database I/O rather than assuming
gp3-equivalent IOPS. Use one reserved public IPv4 address, no CDN or load
balancer. Port the OpenTofu root to `deploy/oci/`; retain the Cloudflare
provider and `deploy/host/` bundle.

## Domain and DNS

The public origin is `https://trachuan.vn`: the website, `/api/v1`, `/mcp`, the
OAuth endpoints and `/.well-known/*` all live on the apex; `www.trachuan.vn`
redirects to it in Caddy. DNS is a Cloudflare zone on the Free plan used for DNS
only, as for aboutme.vn and danny.vn: every record has `proxied = false`, so
browsers and MCP clients reach Caddy directly and Caddy obtains the TLS
certificates. The records are managed with Cloudflare's official
[`cf` CLI](https://developers.cloudflare.com/cf/) (owner decision, 2026-11-13):
A records for the apex and `www` pointing at the reserved public IP
`129.221.172.197`, and CAA records `0 issue "letsencrypt.org"` and
`0 issuewild ";"`, because Caddy is configured with Let's Encrypt as its only
issuer. Universal SSL is disabled on the zone, since no record is proxied;
otherwise Cloudflare adds CAA records for its own certificate authorities.

## Images and deploy

GitHub Actions builds linux/arm64 images pinned by digest. Choose public GHCR
images and a public GHCR artifact carrying the image release manifest and its
signature bundle:
[anonymous pulls are supported](https://docs.github.com/en/packages/working-with-a-github-packages-registry/working-with-the-container-registry).
Keep public layers and metadata free of secrets and personal data. Images and
the host bundle carry only production commands; test commands such as
`apps/server/cmd/fakegoogle`, `apps/server/cmd/fakeembed` and the fixture loader
are never built into them, and the release check lists the binaries in each
image and fails on any other. Private images would need a rotating read-only
host secret. The code manifest is separate from the corpus manifest below.

A release tag deploys without SSH: GitHub signs a release manifest with
[cosign keyless signing through GitHub OIDC](https://docs.sigstore.dev/quickstart/quickstart-ci/),
and a root timer on the host verifies it and the image attestations before it
changes anything. No inbound deploy listener is opened. Repository settings, the
manifest, host policy, activation, rollback and proofs are in
[pull deployment](pull-deployment.md).

## Secrets

Use regional OCI Vault secrets in vault `trachuan-vault` with the
software-protected key `trachuan-secrets`. Each Vault secret has the name of the
Podman secret it becomes: `google_client_secret`, `smtp_username`,
`smtp_password`, `account_keys`, `postgres_password`, `auth_db_password`,
`corpus_reader_password`, `activator_db_password`, `backup_s3_key`,
`backup_s3_key_secret` and `server_signup_emails` (an env secret for
`SERVER_SIGNUP_EMAILS`, so the owner email stays out of Git). The fetcher
derives `server_auth_database_url` and `server_corpus_database_url` from the
passwords as the [host runbook](../runbooks/host.md#create-secrets) does.

The [updater](pull-deployment.md#updater) reconciles secrets at most hourly and
before every activation, under its locks. Through the host's
[instance principal](https://docs.oracle.com/en-us/iaas/Content/Identity/Tasks/callingservicesfrominstances.htm)
and the installed OCI Python SDK it reads the current
[secret bundle](https://docs.oracle.com/en-us/iaas/Content/secret-management/Tasks/get-secrets-contents.htm)
version of each named secret. A secret needs work when its version differs from
the recorded one or its Podman secret is missing. For each such secret the
updater pipes the value into `podman secret create --replace` as `trachuan`,
rebuilds the derived URL secrets that depend on it, and recreates the containers
that use it with the activation procedure (PostgreSQL under the backup lock,
readiness before Caddy). The version is recorded in `/var/lib/trachuan-deploy/`
(names and versions only) only after every dependent secret and container is
reconciled; anything less leaves it pending for the next run. Values travel
through pipes and are stored only by Podman's default file driver under the
`trachuan` account
([podman secret create](https://docs.podman.io/en/v5.8.2/markdown/podman-secret-create.1.html));
never in other files, logs, command arguments, images or OpenTofu state. A
missing or unreadable Vault secret keeps the current Podman secret and records
`secrets_failed`, alarmed like `deploy_failed`. Changing a database role
password is an owner runbook step (`ALTER ROLE`, then the Vault version); the
updater alters only the `activator` role it provisions.

Dynamic group `trachuan-host` (this instance only) gets `read secret-bundles`
where
[`target.secret.name`](https://docs.oracle.com/en-us/iaas/Content/Identity/Reference/keypolicyreference.htm)
is one of the names above, `read objects` in bucket `trachuan-releases`, and
`use log-content` for its log; no secret management or tenancy-wide grants.
Instance identity is a host-wide privilege: application containers cannot reach
instance metadata (host firewall rule for `trachuan` and its subordinate UIDs).
Keep an owner recovery copy outside the host. The server holds no token signing
key: access and refresh tokens are opaque database rows, and the consent form is
bound to the session's CSRF secret.

## Object Storage

Use private Standard-tier buckets in the host region for releases, backups and
pipeline checkpoints/intent logs, with separate access policies. The
[IPv4-only S3 endpoint](https://docs.oracle.com/en-us/iaas/Content/Object/Concepts/use-ipv6-urls.htm)
is `https://<namespace>.compat.objectstorage.<region>.oraclecloud.com`, using
the region in ADR 0006. Use IPv4 connectivity, HTTPS, Signature Version 4 and
path-style addressing. Authenticate with an OCI IAM user's
[customer secret key](https://docs.oracle.com/en-us/iaas/Content/Identity/access/working-with-customer-secret-keys.htm)
(access-key/secret-key pair), not an OCI API signing key. The host's release
downloads are the one exception: they use its instance principal. Use separate
service users per purpose; their bucket policies, not the key itself, constrain
access. Keep the MacBook uploader key in the owner's credential store; host keys
come from Vault. Rotate by installing a replacement, proving access, then
revoking the old key.

[pgBackRest configuration](https://pgbackrest.org/configuration.html) uses
`repo1-type=s3`, `repo1-s3-endpoint` set to the endpoint hostname without
`https://`, `repo1-s3-region` set to the host region, `repo1-s3-uri-style=path`,
`repo1-s3-key-type=shared`, `repo1-s3-bucket` and `repo1-path`. Render
`repo1-s3-key` and `repo1-s3-key-secret` values into a private runtime
configuration from Podman secret mounts; keep TLS verification enabled. These
documented generic S3 options are a compatibility proposal, not a completed OCI
restore test.

Bucket `trachuan-releases` holds corpus releases
([corpus release](corpus-release.md#upload)). IAM user `trachuan-release-upload`
(MacBook) may create and overwrite objects there; `trachuan-release-verify`
(GitHub environment `corpus-release`) may only read them; the host reads them
through its instance principal and needs no key. Backup credentials access only
`trachuan-backups`, including deletion needed for pgBackRest expiry. A separate
pipeline identity writes checkpoints and intent logs; exclude the never-deleted
intent log from expiry policies. Never put credentials in manifests. Prove
multipart upload, download and hash verification with the pinned clients before
the first release.

## Serving bundle and release activation

A serving bundle is the server image, embedder image and recipe, and the active
corpus release. Upload, signing, verification, restore, activation, resume,
prune and recipe changes are in [corpus release](corpus-release.md).

The server serves only the active release; a retained database is a rollback
target that the server never opens. `lexical_recipe` identifies the algorithm
(tokenizer version, normalization, BM25 parameters) and must match the runtime;
`lexical_stats` (vocabulary, document frequencies, length normalization) belongs
to each release, changes with every corpus refresh, and is loaded for the
release being searched.

A recipe change stops traffic at Caddy while the new server and embedder and the
matching release are activated together
([recipe changes](corpus-release.md#recipe-changes)). Rolling a bundle back
re-signs the previous corpus release and uses a signed
[rollback](pull-deployment.md#rollback-workflow) of the images.

## Backups

pgBackRest ([backups](backups.md)) to the private backup bucket: continuous WAL,
weekly full, daily differential, weekly verify, 30-day retention. Use only
[default Object Storage encryption](https://docs.oracle.com/en-us/iaas/Content/Object/Tasks/encryption.htm)
with Oracle-managed keys. Configure no Vault bucket key, SSE-C key or pgBackRest
client-side encryption (`repo1-cipher-type=none`). Anyone who can read the
bucket can read the backups, including account email and auth data. Limit
read/write/delete permissions to the backup identity and owner restore access;
release upload/download and application identities get none. Keep the bucket
private, prohibit public/pre-authenticated sharing, and protect its customer
secret key in Vault. Vault still holds application and storage credentials; it
does not provide backup encryption. Alarm when the newest backup is older than
26 hours. Monthly automated restore into a scratch database with smoke queries.
A copied or leaked backup is readable; IAM cannot revoke that copy. Auth restore
follows the epoch rule in [auth](auth.md). Targets: RPO minutes, RTO about one
hour manual.

## Observability

journald 30 days; health and readiness endpoints; Prometheus metrics scraped
locally; a `watch` systemd timer and Telegram alerts; external uptime check; OCI
Monitoring alarms replace CloudWatch:

- Enable the Compute Instance Monitoring agent for
  [`oci_computeagent.MemoryUtilization`](https://docs.oracle.com/en-us/iaas/Content/Compute/References/computemetrics.htm).
- Publish filesystem used percentage and free bytes for both mounts as
  [custom metrics](https://docs.oracle.com/en-us/iaas/Content/Monitoring/Tasks/publishingcustommetrics.htm)
  from the host timer. Disk I/O metrics do not measure filesystem capacity.
- Alarm on nonzero
  [`oci_compute_instance_health.instance_accessibility_status`](https://docs.oracle.com/en-us/iaas/Content/Compute/References/compute-health-metrics.htm)
  and
  [`oci_compute_infrastructure_health.instance_status`](https://docs.oracle.com/en-us/iaas/Content/Compute/References/infrastructurehealthmetrics.htm),
  plus missing heartbeat/metrics; a stopped instance may emit no status value.

Host jobs (deploy, release, secrets, backups) write result JSON under
`/srv/trachuan/state/`; the metrics timer publishes `deploy_failed`,
`release_failed`, `secrets_failed` and the backup ages, and each has an alarm.
Their log files under `/var/log/trachuan/` go to custom log `trachuan-host-jobs`
in log group `trachuan-logs` through the
[Custom Logs Monitoring plugin](https://docs.oracle.com/en-us/iaas/Content/Logging/Concepts/agent_management.htm),
which reads files from local directories, so results and logs are readable
without SSH. Log lines carry status, never secret values or account data.

Set thresholds from [host budgets](budgets-host.md#host-alarms-proposed).
Deliver OCI alarms through
[Notifications](https://docs.oracle.com/en-us/iaas/Content/Notification/Concepts/notificationoverview.htm)
email independently of the host. Retain the host timer's Telegram alerts and
external uptime check; do not rely on the failed host to relay cloud alarms.
Alerts contain service status, never account email, queries or secrets. Public
SSR pages are cached so Node does not compete with the embedder.

## Residency and transition

Keep the account database, volumes, Vault and backup/corpus-release buckets in
the host region selected in ADR 0006; Oracle
[identifies that region as Singapore](https://docs.oracle.com/en-us/iaas/Content/General/Concepts/regions.htm).
Public code images and deployment manifests in GHCR contain no account data.
Keep
[cross-region object replication](https://docs.oracle.com/en-us/iaas/Content/Object/Tasks/usingreplication.htm)
disabled. The privacy notice should retain Singapore and replace AWS with OCI,
explicitly including account email in backups. This describes application data
at rest, not a promise that Google login or provider account metadata stays in
Singapore. Never send application personal data to alerts or CI artifacts.

After independent review confirms the revision: provision/import infrastructure,
configure secrets and the host pull policy, then prove upload/restore, rollback
and off-host alarms before any account other than the owner's signs in. DNS
already points at the host: the first production phase serves only the owner,
with images pulled by digest over SSH and secrets entered as Podman secrets.
Pull deployment, Vault secrets and corpus release activation replace both, and
then the deploy SSH key is removed
([remote execution](pull-deployment.md#remote-execution)). Reuse the measurement
instance only after inventory and OpenTofu import; do not destroy it to recreate
capacity. If AWS data exists, stop writes for a final backup, verify the copied
objects and restore in OCI, then cut over. Keep old data and decryption keys
until OCI restore proof and the retention period finish. Do not run two auth
writers or roll back to a stale auth database after accepting new writes;
reconcile those writes and apply the existing auth restore epoch rule first.
