# ADR 0006: One OCI Ampere A1 host, rootless Podman Quadlet

Status: accepted 2026-11-08; host revised 2026-11-09; deploy channel and backup
encryption settled by the owner 2026-11-10.

## Context

Budget is about USD 60 per month. The owner already operates aboutme.vn with
Podman Quadlet, pgBackRest and Telegram alerts. The OCI A1 measurement host
offers more memory within this budget; see [costs](../design/budgets-host.md)
and [measurements](../research/host-oci-a1-2026-11.md).

## Decision

One OCI `VM.Standard.A1.Flex`, 4 OCPU (Neoverse-N1) and 24 GB, in
`ap-singapore-1`, on the owner's pay-as-you-go account upgraded from free tier.
Use rootless Podman and systemd Quadlet for caddy, web, server, embedder and
postgres; [deployment](../design/deployment.md#host) owns the OS and packages.
OpenTofu owns infrastructure. GitHub Actions builds arm64 images, pinned by
digest in GHCR. pgBackRest archives WAL to OCI Object Storage. Cloudflare
remains DNS only. Local development runs natively, without containers; no
Ansible. Thread limits live in [budgets](../design/budgets.md#indexing);
precision remains owned by [ADR 0009](0009-dense-embedding-recipe.md).

[Deployment](../design/deployment.md) owns the mechanism and source details:
Balanced boot/block volumes replace gp3; Vault secrets use instance-principal
access; Object Storage uses its S3-compatible API and customer secret keys; OCI
Monitoring and Notifications replace CloudWatch. Numeric sizes and costs live in
[host budgets](../design/budgets-host.md).

The host pulls an authorized, signed image release manifest on a systemd timer.
It verifies the [host trust policy](../design/deployment.md#images-and-deploy)
before serialized activation, retains previous digests, alerts on failure and
rolls back only with authorization to a previously authorized manifest. There is
no inbound deploy listener and no OCI or host credential in GitHub.

Backups use default OCI Object Storage encryption with Oracle-managed keys. No
customer-managed bucket key or pgBackRest client-side encryption is used. Anyone
with bucket read access can read backups; tightly scoped bucket access is the
security boundary. [Deployment](../design/deployment.md#backups) owns the access
and restore rules.

## Consequences

Low recurring cost and familiar container operations; provider-specific
infrastructure and IAM must be rewritten. A single host still has no failover;
A1 capacity and replacement time are not guaranteed by the free allowance. RTO
remains about one hour manual, RPO minutes through WAL, subject to restore
proof. Account email and backups remain in Singapore; the privacy notice names
OCI as the hosting provider. A regional outage can exceed the RTO because
cross-region backup replication is disabled. Pull deployment adds polling delay
and a host updater; compromised release authority can still publish malicious
code. The design chooses public image distribution to avoid host registry
credentials; published image layers and release metadata are public.

## Alternatives rejected

- AWS EC2: replaced by the owner's measured A1 choice and lower cost.
- Restricted SSH: familiar and immediate, but a reusable host credential and
  public listener remain. A
  [forced command](https://man.openbsd.org/sshd.8#AUTHORIZED_KEYS_FILE_FORMAT)
  limits credential misuse; it does not remove the credential or attack surface.
- [OCI federation](https://docs.oracle.com/en-us/iaas/Content/Identity/api-getstarted/token_exchange_grant_type_workload_id-federation.htm)
  plus
  [Run Command](https://docs.oracle.com/en-us/iaas/Content/Compute/Tasks/runningcommands.htm):
  short-lived OCI authorization without an inbound deploy port, but the
  documented exchange still needs an OAuth client credential or an existing OCI
  principal. It adds identity-domain, IAM and agent configuration.
- Run Command with an OCI API key in GitHub violates the no-cloud-credentials
  requirement. A production Actions runner exposes the host to workflow jobs.
- Ubuntu arm64: viable, but switching adds image validation when Oracle Linux
  already has packaged Quadlet support and the host measurement.
- VNGCloud (approved for banhmi, never built), a Vietnamese VPS, Docker Compose
  on the host, Keycloak or any second identity service.

## History

- 2026-11-08: accepted in the brainstorming conversation.
- 2026-11-09: public domain `trachuan.vn`; DNS moves from Route 53 to a
  Cloudflare zone on the Free plan used for DNS only (`proxied = false`),
  managed by OpenTofu as for aboutme.vn and danny.vn
  ([deployment](../design/deployment.md#domain-and-dns)).
- 2026-11-09: owner replaced EC2 with OCI A1 in Singapore after host
  measurement; retained the container, infrastructure and backup architecture.
  Restricted SSH and additional backup encryption were proposed for review.
  Query precision remains a separate study.
- 2026-11-10: after independent review, the owner chose pull deployment and
  default Oracle-managed backup encryption. Added host-side authorization and
  rollback rules; kept host, OS and thread facts in their respective homes.
- 2026-11-10: local development runs natively (PostgreSQL 18 and the Temporal
  development server on the MacBook); `deploy/compose.yml` is removed.
  Production keeps rootless Podman Quadlet on the host.
