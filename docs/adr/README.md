# Architecture decision records

One topic per record, numbered. A record has Status (proposed, accepted,
superseded), Context, Decision, Consequences, Alternatives rejected, and a
History list. To change a decision in force, update its record with the owner's
approval and add a History entry saying what changed and why. Open a new record
only for a new topic.

| ADR                                             | Topic                                       |
| ----------------------------------------------- | ------------------------------------------- |
| [0001](0001-reuse-banhmi-data-only.md)          | Reuse banhmi data, not code                 |
| [0002](0002-evidence-only-vietnamese-queries.md) | Evidence only; Vietnamese queries; no platform model |
| [0003](0003-postgres-per-release-corpus.md)     | PostgreSQL with one database per corpus release |
| [0004](0004-subject-taxonomy-from-phap-dien.md) | Subject taxonomy seeded from Bộ pháp điển   |
| [0005](0005-google-login-and-embedded-oauth.md) | Google-only login; OAuth server copied from aboutme |
| [0006](0006-single-host-aws-podman.md)          | One AWS Graviton host, rootless Podman Quadlet |
| [0007](0007-nuxt-frontend.md)                   | Nuxt 4 frontend with English routes         |
