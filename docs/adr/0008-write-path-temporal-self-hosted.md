# ADR 0008: Write path on self-hosted Temporal, worker on the MacBook

Status: accepted 2026-11-08.

## Context

The owner wants the corpus pipeline to be easy to control, observe, retry and
resume. The first design proposed a Go CLI with JSON files per step. An
independent design review (astra, 2026-11-08) found it lacked durable run
records, completeness accounting, idempotent commits, cancellation and recovery
for external batch jobs, and recommended Temporal as coordinator while keeping
those contracts in application code. banhmi's Temporal redesign grew from one
workflow to seven and never shipped; the review attributed that to scope growth,
not to Temporal itself. Temporal Cloud has no permanent free tier; the server
and UI are MIT licensed. The owner's MacBook is powerful and already runs the
pipeline, and may be off between runs.

## Decision

Run the Temporal server, its UI and PostgreSQL persistence on the MacBook
through `deploy/compose.yml`, started for a run and stopped afterwards. One
workflow `BuildRelease(country, rel)` with one activity per stage, fixed batches
sized in design/budgets.md, continue-as-new after each stage, payloads as
references only. Completeness, idempotency, submission reconciliation and the
signed release artifact are application contracts in `apps/corpus`, recorded in
[pipeline](../design/pipeline.md). No second workflow or scheduler without a
measured need.

## Consequences

Visible state, retries and resumption with no service fee. The server host runs
no Temporal. A run pauses while the MacBook is off and resumes when the worker
returns; activities interrupted by sleep retry against idempotent commits.
Workflow code changes need replay-compatible versioning.

## Alternatives rejected

Temporal Cloud pay-as-you-go (about 6 USD per month at the expected volume, but
external dependency and no need once self-hosting is acceptable). CLI with a
state file (every control feature hand-written). River on PostgreSQL (jobs, not
workflows; sequencing stays in application code). DBOS (less tooling to inspect
runs). Temporal dev server as the authoritative coordinator (documented as not
for production).

## History

- 2026-11-08: accepted after the independent design review.
