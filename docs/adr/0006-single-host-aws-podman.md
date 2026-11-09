# ADR 0006: One AWS Graviton host, rootless Podman Quadlet

Status: accepted 2026-11-08.

## Context

Budget is about USD 60 per month. Danny already operates aboutme.vn on one host
with Podman Quadlet, pgBackRest and Telegram alerts, and keeps OpenTofu AWS
modules.

## Decision

One EC2 Graviton host in ap-southeast-1 on Danny's personal AWS account, Amazon
Linux 2023, rootless Podman with systemd Quadlet units for caddy, web, server,
embedder and postgres. OpenTofu for infrastructure. Images built for arm64 on
GitHub Actions, deployed through an OIDC role and SSM send-command. pgBackRest
with WAL archiving to S3. Compose only for local development. No Ansible.

## Consequences

Low cost and familiar operations. Single host: RTO about one hour manual, RPO
minutes through WAL. Account email is stored in Singapore and the privacy notice
says so.

## Alternatives rejected

VNGCloud (approved for banhmi, never built). A Vietnamese VPS. Docker Compose on
the host. Keycloak or any second identity service.

## History

- 2026-11-08: accepted in the brainstorming conversation.
