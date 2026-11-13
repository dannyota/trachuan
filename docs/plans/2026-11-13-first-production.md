# First Production Host Implementation Plan

> **For agentic workers:** Routes (`instructions/workflow.md`): Tasks 1 and 2 go to Codex `implementer` with `--model gpt-6.1-sol`, reviewed by `reviewer` with `--model gpt-6-astra` (release and security paths). Tasks 3 and 4 are the primary's; Task 5 is the owner's with trachuan-dev2. Executor and coordinator: trachuan-leader.

**Goal:** `https://trachuan.vn` served from `trachuan-host` so the owner can sign in with Google and connect MCP clients on the real domain (Plans 9b and 9c live acceptance).

**Architecture:** One OCI A1 host (ADR 0006), rootless Podman Quadlet: Caddy (TLS, proxy), web (Nuxt), server (Go API, MCP, OAuth), queryembed, PostgreSQL 18 with pgvector. Images built for linux/arm64 by GitHub Actions and pulled by digest. No test environment: the owner decided on 2026-11-13 that the first host is production.

**Spec:** [deployment](../design/deployment.md), [ADR 0006](../adr/0006-single-host-oci-podman.md), [host budgets](../design/budgets-host.md), [budgets](../design/budgets.md), [server runbook](../runbooks/server-local.md). The spec wins over this plan, except for the phasing below, which ADR 0006 History records.

## Done on 2026-11-13 (primary)

- Reserved public IP `129.221.172.197` (`trachuan-prod`) replaced the measurement instance's ephemeral IP.
- Security list `trachuan-sl`: TCP 80 and 443 and UDP 443 from anywhere; SSH only from the owner's addresses.
- Cloudflare records by the official `cf` CLI, all DNS only: `A trachuan.vn` and `A www.trachuan.vn` to the reserved IP; `CAA 0 issue "letsencrypt.org"` and `CAA 0 issuewild ";"`. Universal SSL is disabled, so Cloudflare adds no CAA records of its own.

## Phasing

Phase 1 (this plan) serves the site to the owner only. Phase 2 must land before any other account signs in: signed pull deployment and host trust policy, OCI Vault secrets through the instance principal, pgBackRest to Object Storage with a restore proof, off-host alarms, a separate data volume, OpenTofu import of the OCI resources. Until then no user data exists that a backup must protect, and an image change is deployed by the primary pulling a new digest over SSH.

## Task 1: Images

- [ ] `deploy/images/{server,web,queryembed}.Containerfile`, ported from aboutme `deploy/server.Dockerfile` and `deploy/web.Dockerfile`; queryembed with the pinned ONNX Runtime assets and model of its `ort` CI job. Non-root user, no shell where the base allows it; only production commands (`apps/server/cmd/fakegoogle`, `fakeembed` and the fixture loader are never built in).
- [ ] `.github/workflows/images.yml`: manual dispatch and protected `v*` tags; `ubuntu-24.04-arm`; push `ghcr.io/dannyota/trachuan-{server,web,queryembed}` by digest; GitHub build provenance attestation; a step that lists the binaries in each image and fails on any other; job summary with the three digests.
- [ ] PostgreSQL (`pgvector/pgvector` PG 18) and Caddy official images are pinned by digest in the host bundle, not rebuilt.

## Task 2: Host bundle

- [ ] `deploy/host/install.sh` (idempotent, run as `opc` with sudo): Podman and container-selinux from the OL9 repositories, user `trachuan` with lingering and subordinate IDs, `net.ipv4.ip_unprivileged_port_start=80`, firewalld HTTP, HTTPS and HTTP/3, data directories with SELinux labels. Verifies the Podman version and cgroup v2 the design requires.
- [ ] Quadlet units under `deploy/host/quadlet/`: host networking with every service on 127.0.0.1 except Caddy, PostgreSQL with a named volume, queryembed, server, web, Caddy; images by digest from one env file; secrets by Podman secret name only (`google_client_secret`, `postgres_password`, `auth_db_password`, `corpus_reader_password`, no token signing key (the server uses none)).
- [ ] `deploy/host/Caddyfile`: apex only, `www` redirects to the apex; `/api/v1/*`, `/mcp`, `/mcp/*`, `/oauth/*` and `/.well-known/oauth-*` to the server, the rest to web; `/healthz` and `/readyz` are checked on the host, not at the edge; Let's Encrypt as the only issuer; `SERVER_TRUSTED_PROXIES` and `NUXT_TRUSTED_PROXIES` set to `127.0.0.1/32`, since rootless bridge port forwarding would hide client addresses.
- [ ] `docs/runbooks/host.md`: install, owner secret creation (commands that read the value from the owner's terminal, never from arguments or files in the repository), first start, pulling a new digest, corpus release restore and activation, health checks.

## Task 3: Install and start (primary)

- [ ] Run `install.sh` over SSH; owner creates the Podman secrets; pull images by digest; apply auth migrations; start; health checks on 127.0.0.1 on the host; outside smoke on `https://trachuan.vn` (home page, public status, OAuth metadata, `/mcp` 401); TLS certificate from Let's Encrypt.

## Task 4: Corpus release (primary)

- [ ] Restore the dump of `corpus_vn_20261112_2` (evaluated, `quality_pass`) on the host with the PostgreSQL 18 client, grant the corpus reader, select it as the active release; `/readyz` opens the engine and its query recipe check passes against the host's queryembed.

## Task 5: Acceptance (owner with trachuan-dev2)

- [ ] Owner creates a Google OAuth web client for `https://trachuan.vn` (redirect `https://trachuan.vn/api/v1/auth/google/callback`) in project `trachuan-vn` and enters its secret on the host himself.
- [ ] Plans 9b and 9c live acceptance on `https://trachuan.vn` with dev2's checklist.
