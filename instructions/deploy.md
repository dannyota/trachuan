# Deploy

Read for `deploy/`, `.github/workflows/`, containers, the host and releases.

- Target: one EC2 Graviton host in ap-southeast-1 on Danny's personal AWS account, Amazon Linux 2023, rootless Podman with systemd Quadlet units. Compose is for local development only.
- Infrastructure as code in OpenTofu under `deploy/aws/`, adapted from aboutme. Host bundle under `deploy/host/`: cloud-init, `install.sh`, Quadlet templates, pgBackRest, alert scripts.
- Images: linux/arm64, built on GitHub Actions, pushed to GHCR, pinned by digest. Deploy through GitHub Actions OIDC role and `aws ssm send-command`; no inbound SSH.
- Secrets live in SSM Parameter Store and become Podman secrets at deploy. Never read secret values into a conversation or a brief.
- Corpus release activation (`release activate <rel>`) is separate from code deploys and refuses a manifest whose embedding recipe differs from the running embedder.
- Backups: pgBackRest to S3 with WAL archiving; monthly automated restore drill. A change to backup, restore or release scripts needs adversarial review.
- Caddy does TLS and proxying only. Authentication and limits for `/api/v1` and `/mcp` live in the Go server; admission for public pages lives in Nitro middleware in `apps/web`, with the values in `docs/design/budgets.md`.
