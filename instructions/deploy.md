# Deploy

Read for `deploy/`, `.github/workflows/`, containers, the host and releases.

- Target and container architecture: [ADR 0006](../docs/adr/0006-single-host-oci-podman.md). OS and package requirements: [deployment](../docs/design/deployment.md#host). Runtime thread limits: [budgets](../docs/design/budgets.md#indexing).
- Infrastructure as code in OpenTofu under `deploy/oci/`; port the provider-specific modules from aboutme. Host bundle under `deploy/host/`: cloud-init, `install.sh`, Quadlet templates, pgBackRest, alert scripts.
- Images, pull deployment and host verification: [deployment trust policy](../docs/design/deployment.md#images-and-deploy). Apply that policy to activation and rollback; never bypass it for a manual deploy.
- Secret storage and deploy-time injection: [secrets](../docs/design/deployment.md#secrets). Never read secret values into a conversation or a brief.
- [Signed corpus activation](../docs/runbooks/release-restore.md#release-a-corpus) is separate from code deploys and refuses a manifest whose embedding recipe differs from the running embedder.
- For a GitHub repository or registry owner change, follow the staged [repository move](../docs/design/repository-move.md).
- Backup storage, encryption, access and restore rules: [backups](../docs/design/deployment.md#backups) and [Object Storage](../docs/design/deployment.md#object-storage). A change to backup, restore or release scripts needs adversarial review.
- Caddy does TLS and proxying only. Authentication and limits for `/api/v1` and `/mcp` live in the Go server; admission for public pages lives in Nitro middleware in `apps/web`, with the values in `docs/design/budgets.md`.
