# Open Sign-up Plan

> **For agentic workers:** Routes (`instructions/workflow.md`): backup, restore, release and trust-policy scripts go to Codex `implementer` with `--model gpt-6.1-sol`, reviewed by `reviewer` with `--model gpt-6-astra`; other host and infrastructure files go to `gpt-6.1-terra`, reviewed by `gpt-6.1-sol`. Account and TOTP code goes to `gpt-6.1-sol`, reviewed by `gpt-6-astra`. The primary applies infrastructure and deploys. Executor and coordinator: trachuan-leader; accounts work coordinated by trachuan-dev2.

**Goal:** anyone with a verified email can sign up on `https://trachuan.vn` with email and password, optionally protected by TOTP, after the host meets the gate in [ADR 0006](../adr/0006-single-host-oci-podman.md) History (2026-11-13).

**Decisions:** [ADR 0015](../adr/0015-email-password-accounts-and-totp.md) (owner, 2026-11-14: open sign-up, OCI Email Delivery). Release v0.1.0 (cc4cdc1d, corpus vn/20261114_3) is live for the owner.

**Spec:** [deployment](../design/deployment.md) (Host, Secrets, Object Storage, Backups, Observability, Host trust policy), [host budgets](../design/budgets-host.md), [auth](../design/auth.md). The spec wins over this plan.

## Part A: hardening gate (primary)

### A1: Data volume
- [ ] Attach a 60 GB Balanced block volume to `trachuan-host`; mount it for the PostgreSQL volume and pgBackRest spool with SELinux labels; move the PostgreSQL volume in a maintenance window; the boot volume keeps OS, images and model (host budgets disk table).
- [ ] `deploy/host/install.sh` and `docs/runbooks/host.md` describe the mount; proof: restart, readyz, prewarm, disk under 70% on each filesystem.

### A2: Object Storage and identities
- [ ] Private Standard buckets in the host region: backups, releases, checkpoints; public and pre-authenticated access prohibited.
- [ ] IAM service users with customer secret keys: backup (backup bucket read, write, delete), release upload (MacBook, release bucket write), release download (host, release bucket read); policies by bucket, nothing tenancy-wide.

### A3: Vault secrets
- [ ] Vault `trachuan-vault`, key `trachuan-secrets`; secrets named as in deployment.md "Secrets" (TOTP key ring after Part B defines it).
- [ ] Dynamic group `trachuan-host` policy: `read secret-bundles` on those names, `read objects` in `trachuan-releases`, `use log-content`.
- [ ] Root `trachuan-secrets` service and timer fetch bundles into Podman secrets through a pipe and recreate affected containers; `secrets_failed` metric and alarm.
- [ ] Owner keeps a recovery copy outside the host; the runbook names secrets only.

### A4: Backups
- [ ] pgBackRest in its own container or unit against the PostgreSQL volume: `repo1-type=s3` with the OCI endpoint, path style, `repo1-cipher-type=none`; continuous WAL, weekly full, daily differential, weekly verify, 30-day retention.
- [ ] Restore proof into a scratch database on the host with smoke queries; record time against the one-hour RTO.
- [ ] Monthly automated restore timer; alarm when the newest backup is older than 26 hours.
- [ ] Auth restore runbook applies the epoch rule in auth.md before traffic resumes.

### A5: Alarms
- [ ] Compute Instance Monitoring agent; host timer publishes filesystem used percent and free bytes for both mounts as custom metrics.
- [ ] OCI alarms per host budgets "Host alarms" through Notifications email to the owner; instance accessibility and infrastructure status; missing heartbeat.
- [ ] Host `watch` timer with Telegram alerts and an external uptime check (owner supplies the Telegram bot and chat; names only in Git).

### A6: Signed pull deployment
- [ ] Repository settings (pull-deployment.md "Repository settings") applied from `deploy/github/settings.json`.
- [ ] `images.yml` attests to the registry and signs the image release manifest for a protected `v*` tag with green CI on its commit; `rollback.yml`.
- [ ] Root host updater and timer per pull-deployment.md: verification, promotion sequence, host bundle install, activation, automatic rollback, `deploy_failed` metric and alarm, log file shipped to OCI Logging.
- [ ] Proof: rejection of a development build, a wrong commit or digest, an older sequence and a replayed rollback, on the host in mode `verify`.

### A6b: Corpus release without SSH
- [ ] Bucket `trachuan-releases`, IAM users `trachuan-release-upload` (MacBook, write) and `trachuan-release-verify` (GitHub environment `corpus-release`, read).
- [ ] `scripts/corpus-upload.sh`, `corpus release-row`, `corpus-release.yml`, auth migrations for `release_journal` and `retired_release`, `server release-check`, host activator and timer per corpus-release.md; `release_failed` metric and alarm.
- [ ] Proof: the next evaluated corpus release activates through the timer; a crash and resume test on a 2099 release.

### A6c: Cutover
- [ ] Bootstrap the updater in mode `verify` (last planned SSH deploy), run the host proofs, switch to `activate`.
- [ ] Remove the deploy SSH key from `opc` and `trachuan`; disable the Run Command plugin; the owner keeps his key and the serial console.

### A7: OpenTofu import
- [ ] `deploy/oci/` OpenTofu root importing instance, volumes, reserved IP, security list, buckets, IAM users and policies, vault and alarms; plan shows no changes after import; state holds no secret values.

## Part B: accounts (trachuan-dev2)

- [ ] B1 Design: `docs/design/accounts.md` with the API, schema, mail outbox, TOTP and key ring contracts and a budgets section (Argon2id parameters, token lifetimes, rate limits, cool-downs), written from ADR 0015 by the primary and reviewed by `gpt-6.1-sol` before code.
- [ ] B2 Mail: OCI Email Delivery approved sender for `trachuan.vn`, SPF and DKIM records through the `cf` CLI, SMTP credential as a secret; proof mail to the owner lands in the inbox, not spam.
- [ ] B3 Server: migrations and endpoints for registration, verification, password sign-in, reset, add password to a Google account; TOTP enrollment, pending authentication, recovery codes; epoch fencing on every issuer; outbox sender.
- [ ] B4 Web: sign-up, verify, sign-in with password, reset, account security (TOTP with in-browser QR, recovery codes), OAuth consent works for password and TOTP accounts.
- [ ] B5 Acceptance on the host with throwaway accounts under `SERVER_SIGNUP_EMAILS`: register, verify, sign in, reset, enroll and use TOTP, recovery code, MCP connect with a password account.

## Part C: open sign-up

- [ ] When Part A is complete and B5 passes, clear `SERVER_SIGNUP_EMAILS` on the host, record the date in ADR 0015 History, and delete this plan.
