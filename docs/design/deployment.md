# Deployment and operations

Decision: [ADR 0006](../adr/0006-single-host-aws-podman.md). Reused from
aboutme: OpenTofu modules, host bundle, Quadlet templates, pgBackRest config,
alert scripts, image build with signed provenance, Playwright harness with the
fake Google provider.

## Host

One EC2 Graviton instance (initial `t4g.large`, 2 vCPU, 8 GB; final size from
measurement), gp3 volume, Amazon Linux 2023, rootless Podman, sysctl
`net.ipv4.ip_unprivileged_port_start=80`, one Elastic IP, Route 53 by OpenTofu.
No CloudFront or ALB at launch.

## Images and deploy

linux/arm64 images from GitHub Actions to GHCR, pinned by digest, with a signed
build record. Deploy: GitHub Actions assumes an IAM role through OIDC and runs
`aws ssm send-command`; the host script updates Quadlet image references,
`systemctl daemon-reload`, restarts. Rollback reverts the tag. No inbound SSH.

## Secrets

SSM Parameter Store, fetched at deploy into Podman secrets: Google client
secret, token signing keys, database passwords, the release manifest public
key. Signing keys rotate; the server verifies with all current keys and signs
with the newest.

## Serving bundle and release activation

A serving bundle is the server image, embedder image and recipe, and the active
corpus release. `release activate <rel>` runs as the `activator` role:

1. Download `manifest.json`, `manifest.sig`, dump. Verify the signature with
   the pinned public key, then the dump `sha256` and size against the manifest.
2. Refuse when `schema_version`, `dense_recipe` or `lexical_recipe` differ from
   what the running server and embedder report, or when free memory or disk is
   below the thresholds in [budgets](budgets.md).
3. `pg_restore --no-owner` as `restorer` into `corpus_<cc>_<rel>` with a
   dedicated `maintenance_work_mem`; the TOC is listed first and refused if it
   contains functions, extensions other than pgvector, or event triggers.
4. Smoke queries: counts, one search, one document read.
5. Take the `activation` advisory lock (shared with `release prune`), append a
   `release_journal` row for each step, then compare-and-swap
   `auth.corpus_release.active_db` against the expected current value. The old
   active database is appended to `retired_release` with `retain_until` seven
   days ahead.
6. The server resolves the release once per request and holds that connection
   for the request. `release prune`, under the same lock, drops a database only
   when it is not active, its `retain_until` has passed, and it has no
   connections; the journal records the drop. A failed activation leaves the
   journal at the failed step for `release resume` or `release abort`, which
   drops the half-restored database.

Retained releases stay readable through `document`; `search` against a
retained release whose dense or lexical recipe differs from the running
bundle returns `release_incompatible`. Each release carries its own BM25
statistics (vocabulary, document frequencies, length normalization) in
`lexical_recipe`, and query sparse vectors are built from the release being
searched.

A recipe change needs a maintenance window: stop traffic at Caddy, deploy the
new server and embedder, activate the matching release, resume. Rollback of a
bundle is the same window in reverse: previous image tags, then activation of
the previous release, which is still retained. The runbook owns the exact
steps.

## Backups

pgBackRest to a private S3 bucket: continuous WAL, weekly full, daily
differential, weekly verify, 30-day retention, SSE-KMS with a key the host role
cannot decrypt from backups alone. Alarm when the newest backup is older than
26 hours. Monthly automated restore into a scratch database with smoke queries.
Auth restore follows the epoch rule in [auth](auth.md). Targets: RPO minutes,
RTO about one hour manual.

## Observability

journald 30 days; health and readiness endpoints; Prometheus metrics scraped
locally; a `watch` systemd timer and Telegram alerts; external uptime check;
CloudWatch alarms on disk, memory and instance status. Public SSR pages are
cached so Node does not compete with the embedder.
