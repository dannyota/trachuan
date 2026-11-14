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

A systemd timer fetches the production artifact over HTTPS. Its tag is only a
discovery hint; stage the manifest, bundle and images locally before checking
them. GitHub publishes with its job-scoped `GITHUB_TOKEN` and signs with
[cosign keyless signing through GitHub OIDC](https://docs.sigstore.dev/quickstart/quickstart-ci/).
No OCI or host credential is stored in GitHub. No inbound deploy listener is
opened; administrative SSH stays restricted to the owner's IP.

### Host trust policy

The owner installs a root-owned policy pinning the repository, release workflow
path, allowed protected release-tag pattern, image repository names and Sigstore
trust roots. Populate these values before enabling the timer. The release
workflow signs only after owner production approval and green CI for the exact
source commit; protect its file and tags against unauthorized edits, creation
and movement. Verify the repository plan supports the required
[environment protection](https://docs.github.com/en/actions/how-tos/deploy/configure-and-manage-deployments/manage-environments);
missing protection blocks rollout. A normal build signature is not promotion.

The signed manifest binds the repository, full source commit, protected source
ref, image names and SHA-256 digests, production target, promotion sequence and
optional rollback target manifest hash. These are required policy fields, not
instructions for executing shell text. Before activation the host must:

1. Use [cosign verification](https://docs.sigstore.dev/cosign/verifying/verify/)
   (`verify-blob --bundle`) on the exact manifest bytes, requiring issuer
   `https://token.actions.githubusercontent.com` and certificate identity
   `https://github.com/<repository>/<release-workflow>@<protected-ref>`.
   Construct the anchored identity constraint from local policy, never from
   untrusted manifest values alone. Verify the certificate chain and log proof;
   never skip signature, identity or transparency checks.
2. Require the signed repository/ref to match that policy and signer identity.
   Verify each image's
   [signed build attestation](https://docs.sigstore.dev/cosign/verifying/attestation/)
   against the pinned build workflow identity. Require the
   [SLSA v1 predicate](https://slsa.dev/spec/v1.2/provenance) and compare its
   resolved source repository, full commit and ref with the manifest. Require
   matching attestation subject/image digest and locally pulled digest. Reject
   tags, unlisted image repositories, missing claims and mismatches.
3. Require explicit production authorization from the release workflow above,
   covering that exact commit and all digests. Enforce a strictly increasing
   promotion sequence persisted on the host; a previously seen manifest is a
   no-op, never an instruction to reactivate it. Reject sequence reuse with
   different content. Owner bootstrap pins an initial authorized manifest hash
   and sequence floor; lost state requires owner recovery, not a reset to zero.
4. Permit rollback only to a locally retained, previously authorized manifest
   whose hash is explicitly authorized by the failed/current promotion, or by a
   fresh signed rollback authorization from the same release authority. Bind
   rollback authorization to both current and target manifest hashes; recheck
   signatures and policy. Persist consumed rollback authorization and keep the
   highest promotion sequence, preventing replay after rollback.

Under one host lock, journal activation/rollback and retain previous authorized
manifests and digests; update Quadlets, run `systemctl --user daemon-reload` and
restart. Failure alerts and restores the authorized previous bundle when
permitted; otherwise stop for the owner. Recover from the journal without
repeatedly retrying a failed promotion. Fetch/verification failure leaves the
current deployment running. Corpus activation and recipe changes require the
maintenance window below and the same image authorization policy. Application
containers cannot write trust policy/state. Compromised release authority or
host root defeats this boundary. Before rollout, prove rejection of development
builds, wrong commits/digests and replayed rollback.

## Secrets

Use regional OCI Vault secrets and a standard vault with a software-protected
key. The host's
[instance principal](https://docs.oracle.com/en-us/iaas/Content/Identity/Tasks/callingservicesfrominstances.htm)
fetches
[secret bundles](https://docs.oracle.com/en-us/iaas/Content/secret-management/Tasks/get-secrets-contents.htm)
at deploy into Podman secrets: Google client secret, database passwords, the
corpus manifest public key and backup credentials. Scope the dynamic group to
this instance and `read secret-bundles` to named secrets; no secret management
or tenancy-wide grants. Decode through a pipe or private runtime file, never
logs, command arguments, images or OpenTofu state. Recreate affected containers
after secret replacement. Instance identity is a host-wide privilege: block
application-container access to instance metadata; a compromised host can still
obtain its allowed secrets. Keep an owner recovery copy outside the host. The
server holds no token signing key: access and refresh tokens are opaque database
rows, and the consent form is bound to the session's CSRF secret. In the first
production phase the owner enters the secrets as Podman secrets over SSH
([host runbook](../runbooks/host.md)).

## Object Storage

Use private Standard-tier buckets in the host region for releases, backups and
pipeline checkpoints/intent logs, with separate access policies. The
[IPv4-only S3 endpoint](https://docs.oracle.com/en-us/iaas/Content/Object/Concepts/use-ipv6-urls.htm)
is `https://<namespace>.compat.objectstorage.<region>.oraclecloud.com`, using
the region in ADR 0006. Use IPv4 connectivity, HTTPS, Signature Version 4 and
path-style addressing. Authenticate with an OCI IAM user's
[customer secret key](https://docs.oracle.com/en-us/iaas/Content/Identity/access/working-with-customer-secret-keys.htm)
(access-key/secret-key pair), not an OCI API signing key or instance principal.
Use separate service users for backup, corpus release upload and download; their
bucket policies, not the key itself, constrain access. Keep the MacBook uploader
key in the owner's credential store; host keys come from Vault. Rotate by
installing a replacement, proving access, then revoking the old key.

[pgBackRest configuration](https://pgbackrest.org/configuration.html) uses
`repo1-type=s3`, `repo1-s3-endpoint` set to the endpoint hostname without
`https://`, `repo1-s3-region` set to the host region, `repo1-s3-uri-style=path`,
`repo1-s3-key-type=shared`, `repo1-s3-bucket` and `repo1-path`. Render
`repo1-s3-key` and `repo1-s3-key-secret` values into a private runtime
configuration from Podman secret mounts; keep TLS verification enabled. These
documented generic S3 options are a compatibility proposal, not a completed OCI
restore test.

Upload `manifest.json`, `manifest.sig` and dumps with an S3 client configured
with that endpoint, region, path style and a customer secret key; Oracle lists
[supported operations and clients](https://docs.oracle.com/en-us/iaas/Content/Object/Tasks/s3compatibleapi_topic-Amazon_S3_Compatibility_API_Support.htm).
The uploader writes only its release bucket; the host reads releases and has no
release write/delete grant. Backup credentials access only their bucket,
including deletion needed for pgBackRest expiry. A separate pipeline identity
writes checkpoints and intent logs; exclude the never-deleted intent log from
expiry policies. Never put credentials in manifests. Prove multipart upload,
download/hash verification, WAL archive and restore with the pinned clients
before first release.

## Serving bundle and release activation

A serving bundle is the server image, embedder image and recipe, and the active
corpus release. `release activate <rel>` runs as the `activator` role in one of
two modes, recorded in the journal: `new` restores a database from Object
Storage; `retained` reactivates a database still listed in `retired_release`,
skipping steps 1 to 3 and verifying instead that the database exists and its
`release` row matches `<rel>`. Steps:

0. Take the `activation` advisory lock, shared with `release prune`,
   `release resume` and `release abort`; append `release_journal` (`started`,
   target database name). Every later step appends its own row, so any crash
   leaves a known last step.
1. Download `manifest.json`, `manifest.sig`, dump. Verify the signature with the
   pinned public key, then the dump `sha256` and size against the manifest.
2. Refuse when `schema_version` is not among the versions the running server
   reads, or when `dense_recipe` or `lexical_recipe` (the runtime algorithm
   identifier, not the per-release statistics) differ from what the running
   server and embedder report, or when free memory or disk is below the
   thresholds in [host budgets](budgets-host.md#host-memory-estimates).
3. `pg_restore --no-owner` as `restorer` into `corpus_<cc>_<rel>` with a
   dedicated `maintenance_work_mem`; the TOC is listed first and refused if it
   contains functions, extensions other than pgvector, or event triggers. Dumps
   are zstd-compressed (`-Z zstd:1`), so the host `pg_restore` must be built
   with zstd.
4. Prewarm the new database's chunk table and its indexes with `pg_prewarm`,
   then run smoke queries: counts, one search, one read.
5. In one transaction: compare-and-swap `auth.corpus_release.active_db` against
   the expected current value, insert the old active database into
   `retired_release` with `retain_until` seven days ahead, and append the
   journal row `activated`. 5a. In `retained` mode the same transaction also
   deletes the target from `retired_release`.
6. The server resolves the release once per request and holds that connection
   for the request. `release prune`, under the same lock, drops a database only
   when it is not `active_db`, is not the target of an unfinished journal, its
   `retain_until` has passed, and it has no connections; the journal records the
   drop. After a crash, `release resume` rereads the journal and applies the
   mode's guard: in `new` mode the target must still be neither active nor
   retired; in `retained` mode it must still be retired and not active. It then
   continues from the last recorded step. `release abort` drops the target only
   in `new` mode and only when the guard holds; in `retained` mode it only
   closes the journal.

The server serves only the active release; a retained database is a rollback
target that the server never opens. `lexical_recipe` identifies the algorithm
(tokenizer version, normalization, BM25 parameters) and must match the runtime;
`lexical_stats` (vocabulary, document frequencies, length normalization) belongs
to each release, changes with every corpus refresh, and is loaded for the
release being searched.

A recipe change needs a maintenance window: stop traffic at Caddy, deploy the
new server and embedder, activate the matching release, resume. Rollback of a
bundle is the same window in reverse: an authorized previous image manifest
under the host trust policy, then `release activate --retained` of the previous
release. The runbook owns the exact steps.

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
Reuse the measurement instance only after inventory and OpenTofu import; do not
destroy it to recreate capacity. If AWS data exists, stop writes for a final
backup, verify the copied objects and restore in OCI, then cut over. Keep old
data and decryption keys until OCI restore proof and the retention period
finish. Do not run two auth writers or roll back to a stale auth database after
accepting new writes; reconcile those writes and apply the existing auth restore
epoch rule first.
