# Open Sign-up Plan

> **For agentic workers:** Routes (`instructions/workflow.md`): backup, restore, release and trust-policy scripts go to Codex `implementer` with `--model gpt-6.1-sol`, reviewed by `reviewer` with `--model gpt-6-astra`; other host and infrastructure files go to `gpt-6.1-terra`, reviewed by `gpt-6.1-sol`. Account and TOTP code goes to `gpt-6.1-sol`, reviewed by `gpt-6-astra`. The primary applies infrastructure and deploys. Executor and coordinator: trachuan-leader; accounts work coordinated by trachuan-dev2.

**Goal:** anyone with a verified email can sign up on `https://trachuan.vn` with email and password, optionally protected by TOTP, after the host meets the gate in [ADR 0006](../adr/0006-single-host-oci-podman.md) History (2026-11-13).

**Decisions:** [ADR 0015](../adr/0015-email-password-accounts-and-totp.md) (owner, 2026-11-14: open sign-up, OCI Email Delivery). Release v0.1.0 (cc4cdc1d, corpus vn/20261114_3) is live for the owner.

**Spec:** [deployment](../design/deployment.md) (Host, Secrets, Object Storage, Backups, Observability, Host trust policy), [host budgets](../design/budgets-host.md), [auth](../design/auth.md). The spec wins over this plan.

## Part A: hardening gate (primary)

### A1: Data volume
- [x] Attach a 60 GB Balanced block volume to `trachuan-host`; mount it for the PostgreSQL volume and pgBackRest spool with SELinux labels; move the PostgreSQL volume in a maintenance window; the boot volume keeps OS, images and model (host budgets disk table). (2026-11-14)
- [x] `deploy/host/install.sh` and `docs/runbooks/host.md` describe the mount; proof: restart, readyz, prewarm, disk under 70% on each filesystem.

### A2: Object Storage and identities
- [x] Private Standard buckets in the host region: backups, releases, checkpoints; public and pre-authenticated access prohibited. (backups and releases; checkpoints not created, the MacBook keeps them)
- [x] IAM service users with customer secret keys: backup (backup bucket read, write, delete), release upload (MacBook, release bucket write), release download (host, release bucket read); policies by bucket, nothing tenancy-wide. (the host reads releases through its instance principal instead of a download user)

### A3: Vault secrets
- [x] Vault `trachuan-vault`, key `trachuan-secrets`; secrets named as in deployment.md "Secrets" (TOTP key ring after Part B defines it).
- [x] Dynamic group `trachuan-host` policy: `read secret-bundles` on those names, `read objects` in `trachuan-releases`, `use log-content`.
- [x] Root `trachuan-secrets` service and timer fetch bundles into Podman secrets through a pipe and recreate affected containers; `secrets_failed` metric and alarm. (reconciled inside the updater, v0.1.5)
- [x] Owner keeps a recovery copy outside the host; the runbook names secrets only. (Vault holds the off-host copy)

### A4: Backups
- [x] pgBackRest in its own container or unit against the PostgreSQL volume: `repo1-type=s3` with the OCI endpoint, path style, `repo1-cipher-type=none`; continuous WAL, weekly full, daily differential, weekly verify, 30-day retention.
- [ ] Restore proof into a scratch database on the host with smoke queries; record time against the one-hour RTO. (auth restore check passes in 53 s; the whole-cluster RTO rehearsal is still open)
- [x] Monthly automated restore timer; alarm when the newest backup is older than 26 hours.
- [x] Auth restore runbook applies the epoch rule in auth.md before traffic resumes.

### A5: Alarms
- [x] Compute Instance Monitoring agent; host timer publishes filesystem used percent and free bytes for both mounts as custom metrics.
- [x] OCI alarms per host budgets "Host alarms" through Notifications email to the owner; instance accessibility and infrastructure status; missing heartbeat. (17 alarms on topic trachuan-alarms)
- [ ] Host `watch` timer with Telegram alerts and an external uptime check (owner supplies the Telegram bot and chat; names only in Git). (deferred until the owner supplies a Telegram bot)

### A6: Signed pull deployment
- [x] Repository settings (pull-deployment.md "Repository settings") applied from `deploy/github/settings.json`.
- [x] `images.yml` attests to the registry and signs the image release manifest for a protected `v*` tag with green CI on its commit; `rollback.yml`.
- [x] Root host updater and timer per pull-deployment.md: verification, promotion sequence, host bundle install, activation, automatic rollback, `deploy_failed` metric and alarm, log file shipped to OCI Logging.
- [x] Proof: rejection of a development build, a wrong commit or digest, an older sequence and a replayed rollback, on the host in mode `verify`. (2026-11-14)

### A6b: Corpus release without SSH
- [x] Bucket `trachuan-releases`, IAM users `trachuan-release-upload` (MacBook, write) and `trachuan-release-verify` (GitHub environment `corpus-release`, read).
- [x] `scripts/corpus-upload.sh`, `corpus release-row`, `corpus-release.yml`, auth migrations for `release_journal` and `retired_release`, `server release-check`, host activator and timer per corpus-release.md; `release_failed` metric and alarm.
- [ ] Proof: the next evaluated corpus release activates through the timer; a crash and resume test on a 2099 release. (activation proven with vn/20261114_5 on 2026-11-14; the crash and resume test on the host is not run)

### A6c: Cutover
- [x] Bootstrap the updater in mode `verify` (last planned SSH deploy), run the host proofs, switch to `activate`. (2026-11-14)
- [x] Remove the deploy SSH key from `opc` and `trachuan`; disable the Run Command plugin; the owner keeps his key and the serial console. (Run Command disabled; the owner chose to keep the deploy key for occasional use, 2026-11-14)

### A7: OpenTofu import
- [x] `deploy/oci/` OpenTofu root importing instance, volumes, reserved IP, security list, buckets, IAM users and policies, vault and alarms; plan shows no changes after import; state holds no secret values. (59 resources, 2026-11-15)

## Part B: accounts (trachuan-dev2)

- [x] B1 Design: `docs/design/accounts.md` with the API, schema, mail outbox, TOTP and key ring contracts and a budgets section (Argon2id parameters, token lifetimes, rate limits, cool-downs), written from ADR 0015 by the primary and reviewed by `gpt-6.1-sol` before code.
- [x] B2 Mail: OCI Email Delivery approved sender for `trachuan.vn`, SPF and DKIM records through the `cf` CLI, SMTP credential as a secret; proof mail to the owner lands in the inbox, not spam.
- [x] B3 Server: migrations and endpoints for registration, verification, password sign-in, reset, add password to a Google account; TOTP enrollment, pending authentication, recovery codes; epoch fencing on every issuer; outbox sender.
- [x] B4 Web: sign-up, verify, sign-in with password, reset, account security (TOTP with in-browser QR, recovery codes), OAuth consent works for password and TOTP accounts.
- [x] B5 Acceptance on the host with throwaway accounts under `SERVER_SIGNUP_EMAILS`: register, verify, sign in, reset, enroll and use TOTP, recovery code, MCP connect with a password account.
  - Done 2026-11-14 on v0.1.5, rechecked 2026-11-15 on v0.1.8, with three throwaway plus-address accounts under the allowlist (deleted afterwards) and one address kept off the allowlist: sign-up, mail link, set password, sign-in and sign-out, change password, reset (every session and personal token revoked, no session issued), uniform 202 answers and comparable sign-in latency for allowlisted, non-allowlisted and registered addresses, account-exists mail, mail in Vietnamese then English with tokens only in the `/link#` fragment, per-email password limit, MCP connect with a password account (DCR, consent, PKCE, token, tools/list, revoke).
  - Fixed from the run (#251, live in v0.1.8): signed-out consent redirected to Google instead of `/login?next=`; the per-email limit counted successful attempts (now failed attempts only, #249); `/link` title, username field and reset heading.
  - TOTP enabled in production 2026-11-15 (v0.1.12, after the release floor) and accepted on the host with a throwaway account (deleted afterwards): setup with QR and key, confirm with recovery codes, sign-in with a TOTP code and with a recovery code (single use), removal, enrolled and removed notices without links. `/login/factor` title fixed from the run.
- [x] B5 owner step: add a password to the owner's Google account (2026-11-15).

## Part C: open sign-up

- [ ] When Part A is complete and B5 passes, clear `SERVER_SIGNUP_EMAILS` on the host, record the date in ADR 0015 History, and delete this plan.
