# Deployment and operations

Decision: [ADR 0006](../adr/0006-single-host-aws-podman.md). Reused from
aboutme: OpenTofu modules, host bundle, Quadlet templates, pgBackRest config,
alert scripts, image build with signed provenance, Playwright harness with the
fake Google provider.

## Host

One EC2 Graviton instance (initial `t4g.large`, 2 vCPU, 8 GB; final size from
measurement), gp3 volume, Amazon Linux 2023, rootless Podman, sysctl
`net.ipv4.ip_unprivileged_port_start=80`, one Elastic IP. No CloudFront or ALB
at launch.

## Domain and DNS

The public origin is `https://trachuan.vn`: the website, `/api/v1`, `/mcp`, the
OAuth endpoints and `/.well-known/*` all live on the apex; `www.trachuan.vn`
redirects to it in Caddy. DNS is a Cloudflare zone on the Free plan used for DNS
only, as for aboutme.vn and danny.vn: every record has `proxied = false`, so
browsers and MCP clients reach Caddy directly and Caddy obtains the TLS
certificates. OpenTofu owns the records with the `cloudflare` provider,
following aboutme's `deploy/vn/dns`: A records for the apex and `www` pointing
at the Elastic IP, and CAA records that allow only the certificate authority
Caddy uses. The zone and the registrar's name servers are set up by the owner
before the host plan.

## Images and deploy

linux/arm64 images from GitHub Actions to GHCR, pinned by digest, with a signed
build record. Deploy: GitHub Actions assumes an IAM role through OIDC and runs
`aws ssm send-command`; the host script updates Quadlet image references,
`systemctl daemon-reload`, restarts. Rollback reverts the tag. No inbound SSH.

## Secrets

SSM Parameter Store, fetched at deploy into Podman secrets: Google client
secret, token signing keys, database passwords, the release manifest public key.
Signing keys rotate; the server verifies with all current keys and signs with
the newest.

## Serving bundle and release activation

A serving bundle is the server image, embedder image and recipe, and the active
corpus release. `release activate <rel>` runs as the `activator` role in one of
two modes, recorded in the journal: `new` restores a database from S3;
`retained` reactivates a database still listed in `retired_release`, skipping
steps 1 to 3 and verifying instead that the database exists and its `release`
row matches `<rel>`. Steps:

0. Take the `activation` advisory lock, shared with `release prune`,
   `release resume` and `release abort`; append `release_journal` (`started`,
   target database name). Every later step appends its own row, so any crash
   leaves a known last step.
1. Download `manifest.json`, `manifest.sig`, dump. Verify the signature with the
   pinned public key, then the dump `sha256` and size against the manifest.
2. Refuse when `schema_version`, `dense_recipe` or `lexical_recipe` (the runtime
   algorithm identifier, not the per-release statistics) differ from what the
   running server and embedder report, or when free memory or disk is below the
   thresholds in [budgets](budgets.md).
3. `pg_restore --no-owner` as `restorer` into `corpus_<cc>_<rel>` with a
   dedicated `maintenance_work_mem`; the TOC is listed first and refused if it
   contains functions, extensions other than pgvector, or event triggers.
4. Smoke queries: counts, one search, one document read.
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

Retained releases stay readable through `document`; `search` against a retained
release whose `dense_recipe` or `lexical_recipe` differs from the running bundle
returns `release_incompatible`. `lexical_recipe` identifies the algorithm
(tokenizer version, normalization, BM25 parameters) and must match the runtime;
`lexical_stats` (vocabulary, document frequencies, length normalization) belongs
to each release, changes with every corpus refresh, and is loaded for the
release being searched.

A recipe change needs a maintenance window: stop traffic at Caddy, deploy the
new server and embedder, activate the matching release, resume. Rollback of a
bundle is the same window in reverse: previous image tags, then
`release activate --retained` of the previous release. The runbook owns the
exact steps.

## Backups

pgBackRest to a private S3 bucket: continuous WAL, weekly full, daily
differential, weekly verify, 30-day retention, SSE-KMS with a key the host role
cannot decrypt from backups alone. Alarm when the newest backup is older than 26
hours. Monthly automated restore into a scratch database with smoke queries.
Auth restore follows the epoch rule in [auth](auth.md). Targets: RPO minutes,
RTO about one hour manual.

## Observability

journald 30 days; health and readiness endpoints; Prometheus metrics scraped
locally; a `watch` systemd timer and Telegram alerts; external uptime check;
CloudWatch alarms on disk, memory and instance status. Public SSR pages are
cached so Node does not compete with the embedder.
