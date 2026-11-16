# Public Document Summaries Plan (Plan 19)

> **For agentic workers:** Routes (`instructions/workflow.md`): every implementation stage is hard work, implemented by Codex `implementer` with `--model gpt-6.1-sol` and reviewed by `reviewer` with `--model gpt-6-astra`, because it moves the account boundary. The primary runs `make pre-push`, `make test`, `make test-integration`, `make web-test`, `make web-build` and `make e2e` as each stage needs, and runs the capacity acceptance itself.

**Goal:** visitors without an account read a document's identity, status, outline and approved chapter and article titles at `/vn/documents/{key}/summary`; reading the full text still needs an account ([ADR 0005](../adr/0005-google-login-and-embedded-oauth.md), History 2026-11-16).

**Spec:** [public document summaries](../design/web-public-documents.md) and [serving public summaries](../design/web-public-documents-serving.md). The spec wins over this plan. Each stage builds the part it names and leaves the rest as it is today.

## Stages

| Stage | Contents | Depends on | State |
|-|-|-|-|
| D3a corpus | `document_summary` rows in the build with their caps, eligibility and candidate headings under rules 1 to 4; `sources/vn-heading-exclusions.yaml` with its closed schema, applied and hashed into the build input hash; `corpus heading-scan <rel>` and its hashed output; `sources/vn-heading-reviews.yaml` with its closed schema; the manifest's `heading_review`; the next schema version; build tests for caps, eligibility, every heading fixture and the unapproved-release scan fixture | design merged | planned |
| D3a server | `GET /api/v1/public/documents/{key}` and `GET /api/v1/public/documents` in OpenAPI and the generated client; the admission gate, metadata-only resolver, row cache, non-waiting pool and its retirement on activation; `SERVER_PUBLIC_SUMMARIES` (off by default) and the release capability; `features.public_summaries` on status; `headings_approved` at activation; the error precedence; Go contract tests including closed-schema validation of actual responses and credential equivalence | D3a corpus | planned |
| D3b web | the independent summary route, layout and session-independent header; the public transport with `credentials: 'omit'`; topic and relation links by availability; the render-slot share and the 32 MiB summary cache; head tags and JSON-LD; the sitemap index and documents child; the isolation, decoded-payload, availability, freshness and escape tests | D3a server | planned |
| D3c evidence | a release built with D3a; the scan, the leader's review and exclusions, rebuilt until approved; the capacity acceptance on the local stack and on A1, with results in `docs/design/budgets.md` | D3b | planned |

Turning `SERVER_PUBLIC_SUMMARIES` on in production is the leader's deploy change after D3c passes.

The discovery design's indexing, sitemap and test sections gain the summary rules in PR #338, which carries `docs/design/discovery.md`.
