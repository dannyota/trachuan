# Retrieval Cases and Fixes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Routes (`instructions/workflow.md`): Task 1 goes to `implementer` with `--model gpt-6.1-sol`, reviewed by `reviewer` with `--model gpt-6-astra`; Task 3's override goes to `gpt-6.1-terra`, reviewed by `gpt-6.1-sol`; Task 2 cases are written by Claude subagents (no API key) and rechecked by a Codex worker on another model; Tasks 4 to 6 are the executing session's. Executor and coordinator: the trachuan session.

**Goal:** About 40 visible development cases scored next to the old product cohort, and fixes for the failure classes the run 6 misses already name (per-document cap, expired documents), each kept only under ADR 0010 item 4.

**Architecture:** Scenarios are written from the run's document list without provision text; queries by an agent that sees only the scenario; labels adjudicated against official text and rechecked by another model. The suite loads `development.json` like the old product cases and scores it as its own cohort; calibration fits on both cohorts. Each fix is one measured before/after change, not a grid.

**Tech Stack:** Go evaluation package `apps/corpus/internal/evaluation`; corpus database of the newest built run; JSON fixtures; Claude subagents and Codex workers.

**Spec:** [ADR 0010](../adr/0010-retrieval-evaluation-for-agents.md) (as amended 2026-11-11: three cohorts, item 4 change rule), [ADR 0013](../adr/0013-compact-search-and-batch-read.md), [evaluation](../design/evaluation.md), [calibration](../design/calibration.md), [retrieval recipe](../design/retrieval-recipe.md), [testing](../../instructions/testing.md). The spec wins over this plan.

## Scope

|In|Out|
|-|-|
|Development cohort in the suite; about 40 cases (30 positive, 10 negative) with identity entries; owner spot check; calibration on old product and development; per-document cap fix; history-question policy; causes of the remaining misses; release-1 recipe recorded|Sealed cases (removed 2026-11-11); parameter grids; model changes (ADR 0009 keeps Vietnamese_Embedding_v2 unless a model passes item 4); taxonomy-filtered cases (Plan 6); OCR text (Plan 7b) beyond what the run under test contains|

## Global Constraints

- Cases are written from a legal subject, not from provision text; the query writer sees only the scenario; queries are Vietnamese with diacritics or a document number (ADR 0002).
- The development cohort targets 30 positive and 10 negative cases; cases are never removed to raise a score; a wrong label is corrected from official text.
- Product scoring and the 2026-11-11 gates and targets apply unchanged to old product and development, each on its own.
- A change ships only under ADR 0010 item 4: all gates pass on both cohorts, at least one expected citation gained within 20 hits, and recall@20 and MRR@8 fall on neither cohort.
- Corpus scope for scenarios: the documents of the newest built run (banking and finance centred: State Bank 2,059, Ministry of Finance 274, Government 116, Prime Minister 82 primary documents in run 6), plus negatives outside it.
- Authoring uses Claude subagents and Codex workers, not the Anthropic API key (owner, 2026-11-11).

## Review Focus

- Development numbers mixed into old product totals or the reverse: Task 1 `TestDevelopment_ScoredSeparately`.
- An empty development file passing or failing `quality_pass`: Task 1 `TestDevelopment_NotRunWhenEmpty`.
- Calibration fitting on one cohort only, or on rejection checks: Task 1 `TestCalibrate_UsesOldProductAndDevelopment`.
- The cap override reaching build or serving recipes: Task 3 `TestCapOverride_EvalOnly` (recipe hash and serving search unchanged).
- A wrong label credited or penalised: Task 2 recheck by a second model, then the owner spot check.

---

### Task 1: Development cohort in the suite

**Files:** Modify `apps/corpus/internal/evaluation/{cases.go,fixtures.go,run.go,report.go,metrics_report.go,calibrate.go}` and tests; create `apps/corpus/internal/evaluation/testdata/vn/development.json` (empty array, filled in Task 2).

**Interfaces:** Development cases use the legacy case format plus `tags` (from `identifier`, `clause`, `multi_document`, `abbreviation`, `missing_date`, `historical`, `out_of_domain`, `negative`); their identity entries live in `identity-map.json` like old product entries. The report gains `cohorts.development` with the same records and floors as old product; development is `not_run` while the file is empty; the report has no sealed entry. Calibration fits on old product and development together and labels itself `provisional-old-product` or `old-product+development`.

- [ ] Tests: `TestDevelopment_ScoredSeparately`; `TestDevelopment_NotRunWhenEmpty`; `TestDevelopment_FloorsApply`; `TestCalibrate_UsesOldProductAndDevelopment`; `TestReport_NoSealedEntry`.
- [ ] Implement, run unit and integration tests, commit `feat(eval): development cohort`.

### Task 2: Cases

- [ ] Scenarios: a subagent with only the run's document list (number, title, type, issuer, dates; no provision text) and the tag list writes scenarios `{id, subject, situation, intent, tags, negative}` in Vietnamese, about 50 positive and 15 negative so that drops still leave 30 and 10. A Codex worker on another model removes duplicates and scenarios that name an article or quote text, and records why.
- [ ] Queries: a fresh subagent that sees only the scenarios writes the one query an agent would send to `search` for each.
- [ ] Labels: a subagent with read access to the run's corpus database and official sources (VBPL, vanban.chinhphu.vn) labels each positive with expected citations `{doc_number, article?, clause?, point?, alt_doc_numbers?}` and evidence (`fetch_sha256`, node id, quoted span), or reclassifies it negative with a reason; current law at the evaluation date unless tagged `historical`. A Codex worker on another model rechecks every label against its evidence; the executing session settles disagreements from official text.
- [ ] Show Danny 10 cases and apply his corrections; commit `development.json` and the identity entries; `make eval` on the newest run; record development and old product results.

### Task 3: Per-document cap

- [ ] A research-only per-document cap override in ranking (`internal/retrieval`, bounded like `Depths`), set only by `corpus eval --cap N|none`; it never changes the build recipe, calibration or serving search, and the report records it. Test `TestCapOverride_EvalOnly`.
- [ ] Run the current cap and 4, 5 and none on both cohorts; keep a value only under ADR 0010 item 4 and compact p95 within the ADR 0013 size gate; record the numbers and change `docs/design/budgets.md` and the recipe if one wins.

### Task 4: History-question policy

- [ ] On cases tagged `historical` (development and old product), compare (a) default search returns up to 3 non-current hits after the current ones, clearly marked, and (b) agents pass `in_force_only:false` themselves as `guide` instructs; measure recall@20, current-law precision of primary hits and compact size; write the decision into `docs/design/retrieval.md` with the numbers, or keep (b) if (a) costs precision.

### Task 5: Remaining misses

- [ ] Read the per-case rows of every miss within 20 on both cohorts after Tasks 3 and 4; group them by cause (missing text, validity, lexical, dense, label); add one backlog line per cause with its case count. A fix for a cause is its own change under item 4.

### Task 6: Record

- [ ] Record each kept change with its numbers in ADR 0010 History and the release-1 recipe in `docs/design/retrieval-recipe.md`; if nothing passes item 4, recipe 1 stays.
