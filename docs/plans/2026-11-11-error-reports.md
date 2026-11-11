# Agent Error Reports Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Routes (`instructions/workflow.md`): Tasks 1, 4, 5 and 6 go to Codex `implementer` on `gpt-6.1-terra`, reviewed by `gpt-6.1-sol`; Tasks 2 and 3 (validation and the MCP tool) go to `implementer` with `--model gpt-6.1-sol`, reviewed by `reviewer` with `--model gpt-6-astra` with an adversarial pass; the final whole-branch review is `gpt-6-astra` adversarial. Task 7 is the executing session's. Executor: trachuan-dev2; coordinator: the trachuan session, which merges. Starts after this plan merges.

**Goal:** Users' agents and the website report corpus defects as structured data (handle, kind, typed suggestion), the operator triages them on the host, and accepted reports export as reviewable drafts for corrections, extraction review and identity-map review.

**Architecture:** New package `apps/server/internal/reports` (validation, dedupe and limits, storage through sqlc) used by an MCP tool in `mcpapi`, web handlers in `accountapi`, and admin CLI commands in `app`. One new auth table `issue_report`. Retention runs in the existing hourly purge loop.

**Tech Stack:** Go 1.27.3, pgx v5, sqlc 1.31.1, the MCP go-sdk already in use; no new modules.

**Spec:** [error reports](../design/error-reports.md) (all), [data](../design/data.md) `issue_report`, [budgets](../design/budgets.md) "Error reports", [web API](../design/web-api.md) report routes and account route rules, [search results](../design/mcp-results.md) (handles, errors, guide), ADR 0002. The spec wins over this plan.

## Scope

|In|Out|
|-|-|
|migration 00004 and queries; validation of every field; dedupe and limits; MCP tool `report_issue` and the `guide` paragraph; `POST /api/v1/reports`, `GET` and `DELETE /api/v1/me/reports` with OpenAPI and generated client; admin CLI `reports list|show|accept|reject|export`; retention in the purge loop; runbook section on triage|a web triage page (none); applying corrections (never automatic); the Nuxt form (Plan 10)|

## Global Constraints

- No free-text field and no URL anywhere: unknown members are `invalid_request`; every string is a handle, a closed-list value, an ISO date or a normalized document number checked against the release as the design says.
- Kinds `wrong_text`, `wrong_validity`, `broken_citation`, `missing_document`; suggestion members exactly as the design's table; `eff_from` not after `eff_to`.
- Limits from budgets: 20 reports per account per day, 200 open and 1,000 retained per account, 100,000 retained in total; non-open reports deleted 90 days after their last state change. Admission locks the account row and takes the reports advisory lock before counting. Calls share the `read` per-account bucket.
- One open report per (account, group key) with group key (document_key, anchor, kind, target); a repeat returns the existing report with `deduplicated: true`. `report_issue` is annotated `readOnlyHint: false`, `idempotentHint: true`.
- Triage never shows email or Google subject; account ids only with `--accounts`. Exports carry no account data and never write into `sources/` or the identity map.
- Errors: MCP and web error shapes as now; auth database errors 503. Never log report contents beyond kind and report id. Go files at most 400 lines; Markdown at most 300 lines.

## Review Focus

- Free text smuggled through a typed field (a document number such as `1/CAN-I-DISMISS-MY-EMPLOYEE`, a handle anchor, dates outside range): Task 2 `TestValidate_NoFreeTextChannel`.
- A report against a document or anchor that is not in the release, or against a retired release: Task 2 `TestValidate_HandleResolves`.
- Concurrent duplicate or distinct reports exceeding a cap, and submit-withdraw cycles across accounts exceeding the retained caps: Task 1 `TestStore_DedupeAndCapsUnderRace`.
- An account reading or withdrawing another account's report: Task 4 `TestReports_OwnOnly`.
- Triage or export leaking account identity: Task 5 `TestTriage_NoIdentityWithoutFlag` and `TestExport_NoAccountData`.

---

### Task 1: Report schema

**Files:** Create `apps/server/migrations/00004_reports.sql`, `apps/server/sql/issue_report.sql`, regenerated store, integration tests.

**Interfaces:** Table per data.md with a partial unique index on (account, document_key, anchor, kind, target) where `state = 'open'`, `ON DELETE CASCADE` from account, indexes for retention and triage grouping. Queries: `CreateReport` (used inside the admission transaction after the account row lock and the reports advisory lock: inserts unless an open duplicate exists and the daily, open and retained caps allow; returns the row and a deduplicated flag or a cap marker), `ListOwnReports` (cursor pages of 100), `WithdrawOwnReport`, `TriageGroups`, `TriageReports`, `DecideReports`, `CreateExportBatch` (one transaction assigning a batch id to accepted reports without one and copying their snapshot rows into `issue_report_export`), `BatchSnapshot`, retention of snapshots, `DeleteClosedReports` (batch limit and retention parameters).

- [ ] **Step 1: Failing tests** (integration): `TestMigrate_00004RoundTrip`; `TestStore_DedupeAndCapsUnderRace` (concurrent identical reports on separate connections; caps at their boundaries); `TestStore_OwnRowsOnly`; `TestStore_ExportBatchSnapshotSurvivesDeletion`; `TestStore_RetentionDeletesClosed`.
- [ ] **Step 2: Implement**; run `make generate` and `make sqlc-check`; commit `feat(server): error report schema`.

### Task 2: Validation, dedupe and limits

**Files:** Create `apps/server/internal/reports/{report.go,validate.go,number.go,service.go}` with tests; the release's number segment set comes from one query over `document.number_norm`, cached with the engine.

**Interfaces:** `reports.Input{Country, Kind, Handle, Release string; Suggestion map[string]json.RawMessage}`; `reports.Service{Store; Resolver (release lease, engine lookup of a document, anchor and document number, and the set of number segments of the release); Clock}` with `Submit(ctx, accountID, Input) (Result, error)` returning `{ReportID int64; State string; Deduplicated bool}`; typed errors mapping to `invalid_request`, `rate_limited`, `release_expired`, `unavailable`.

- [ ] **Step 1: Failing tests:** `TestValidate_KindSuggestionMatrix`; `TestValidate_NoFreeTextChannel` (unknown members, `source_url`, numbers whose segments are not in the release, numbers outside the grammar or year range, long values); `TestValidate_Dates` (future effective dates up to ten years, issue dates not in the future); `TestValidate_LegacyNumberCitation` (`196-HĐBT`); `TestValidate_HandleResolves` (unknown document, unknown anchor, retired release); `TestSubmit_DedupeAndCaps`; `TestSubmit_BlockedAccount`.
- [ ] **Step 2: Implement**; document numbers use the corpus normalizer; `broken_citation` requires membership in the release's `number_norm` set, `missing_document` the structural form and release segments; commit `feat(server): error report validation`.

### Task 3: MCP tool

**Files:** Modify `apps/server/internal/mcpapi` (register `report_issue`, schema with enums and per-kind suggestion members, `read` bucket), the `guide` text; tests.

- [ ] **Step 1: Failing tests:** `TestReportIssue_SchemaHasNoFreeText`; `TestReportIssue_Annotations` (through `tools/list`); `TestReportIssue_ResultShapeAndDedupe`; `TestReportIssue_ErrorsAndLimits`; `TestGuide_MentionsReporting`.
- [ ] **Step 2: Implement**; commit `feat(server): report_issue MCP tool`.

### Task 4: Web routes and contract

**Files:** Modify `apps/server/internal/accountapi` (or a new `reportsapi` package if accountapi would pass 400 lines per file), `packages/schema/openapi.yaml`, regenerated client, contract tests.

- [ ] **Step 1: Failing tests:** `TestReports_SubmitEqualsMCP`; `TestReports_OwnOnly`; `TestReports_WithdrawOpenOnly`; `TestReports_OwnListPagination` (reach and withdraw a report beyond the first page); contract tests for the three routes.
- [ ] **Step 2: Implement**; `make schema-gen`; commit `feat(server): error report web routes`.

### Task 5: Triage CLI and export

**Files:** Modify `apps/server/internal/app/admin.go` (or a new `admin_reports.go`), tests.

**Interfaces:** `server admin reports list [--kind K] [--state S]`, `show <group id> [--accounts]`, `accept|reject (<group id>|--ids ...) --by NAME`, `export --out DIR [--batch ID]`.

- [ ] **Step 1: Failing tests:** `TestTriage_GroupsAndCounts` (two targets cited from one provision are two groups); `TestTriage_NoIdentityWithoutFlag`; `TestDecide_GroupAndIDs`; `TestExport_DraftFormats` (corrections draft parses as the corrections YAML shape with TODO evidence, only document-level groups with one agreed non-empty set; others in validity-review.jsonl); `TestExport_NoAccountData`; `TestExport_BatchRecovery` (a crash after the transaction or during writing is repaired by `--batch` into a fresh directory; an existing batch directory is refused; an expired snapshot fails with a fixed message; membership unchanged after account deletion); `TestExport_OnlyUnbatchedReports`.
- [ ] **Step 2: Implement**; commit `feat(server): error report triage and export`.

### Task 6: Wiring, retention and runbook

**Files:** Modify `apps/server/internal/app` (routes, purge loop step), `docs/runbooks/server-local.md` (triage and export), `docs/architecture.md`.

- [ ] **Step 1: Failing tests:** `TestRoutes_ReportsMounted` through the real constructor; `TestPurge_DeletesClosedReports`.
- [ ] **Step 2: Implement**; commit `feat(server): error report wiring and retention`.

### Task 7: Proof (executing session)

- [ ] **Step 1:** Local server with a personal token: report each kind through `report_issue`, repeat one (deduplicated), hit a cap with a lowered test limit; triage, accept, export; check the export files and that no account data or free text appears anywhere; record in the PR.
- [ ] **Step 2:** Open the PR; the coordinator merges after the final adversarial review and green CI.
