# Retrieval Cases and Tuning Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Routes (`instructions/workflow.md`): Task 1 (suite support, sealed isolation) goes to `implementer` with `--model gpt-6.1-sol`, reviewed by `reviewer` with `--model gpt-6-astra`; Tasks 2 to 4 are authored by fresh Codex workers that each see only their own inputs (named per task), checked by a second worker on another model; Task 7 (trial runner) goes to `gpt-6.1-terra`, reviewed by `gpt-6.1-sol`; Tasks 5, 6, 8 and 9 are the executing session's. Executor and coordinator: the trachuan session.

**Goal:** Development and sealed cohorts under ADR 0010, tuning trials on cached arms that choose one finalist recipe for release 1 against the baseline, and a measured policy for history questions, without ever exposing sealed cases to anyone who tunes.

**Architecture:** Scenarios and queries are written by workers that never see provision text; labels are adjudicated against official text by other workers; a deterministic script groups cases into connected components, reserves sealed components first and writes sealed cases straight to `work/eval-sealed/` (mode 700, never committed before release). The evaluation suite loads the committed development cohort and, only in a sealed run, the sealed files, reporting aggregates only for sealed. Trials re-rank cached arms (`Collect` once, `Rank`/`Shape` many) over a small grid on the development cohort.

**Tech Stack:** Go evaluation package `apps/corpus/internal/evaluation`; corpus database of the newest built run; JSON fixtures; Codex workers.

**Spec:** [ADR 0010](../adr/0010-retrieval-evaluation-for-agents.md) (items 1 to 5 and the 2026-11-11 History), [ADR 0013](../adr/0013-compact-search-and-batch-read.md), [evaluation](../design/evaluation.md), [calibration](../design/calibration.md), [retrieval recipe](../design/retrieval-recipe.md), [study](../research/retrieval-hybrid-2026-11.md) ("Protect the gate set" for components, reservation, intervals), [testing](../../instructions/testing.md). The spec wins over this plan.

## Scope

|In|Out|
|-|-|
|Suite support for development and sealed cohorts; about 40 development and 40 sealed cases (30 positive, 10 negative each); identity entries; owner spot checks; trial runner and log over cached arms; one finalist and the baseline frozen; one sealed run; history-question policy; release-1 recipe decision recorded in ADR 0010 History|Model changes (ADR 0009 keeps Vietnamese_Embedding_v2 unless a model wins under item 4); taxonomy-filtered cases (Plan 6); OCR text (Plan 7b) beyond what the run under test contains|

## Global Constraints

- Sealed isolation (ADR 0010 item 3): no brief, report, scratch file, prompt, commit, log or tuning context may contain a sealed query or label until the baseline and the finalist are frozen; sealed files live only in `work/eval-sealed/` (directory mode 700, files 600); the scoring path prints aggregates only and never writes sealed question text, vectors or per-case rows to artifacts or caches.
- Cases are written from a legal subject, not from provision text; the query writer sees only the scenario; queries are Vietnamese with diacritics or a document number (ADR 0002).
- Each cohort targets 30 positive and 10 negative cases; never split a connected component to hit a count; components that touch the old 92-case set cannot be sealed; seed `20261111` for reservation.
- Product scoring and the 2026-11-11 gates and targets apply unchanged; intervals are paired bootstrap with 10,000 resamples over components, seed `20261109`.
- Corpus scope for scenarios: the documents of the newest built run (banking and finance centred: State Bank 2,059, Ministry of Finance 274, Government 116, Prime Minister 82 primary documents in run 6), plus negatives outside it.
- Cost: every model call is logged with tokens; the whole case authoring is expected under 30 USD; tell the coordinator before exceeding it.

## Review Focus

- A sealed question or label leaking into a report, cache, `cases.jsonl`, `queries.f32`, a log line or a worker brief: Task 1 `TestSealed_NoTextOrVectorsInArtifacts`, `TestSealed_AggregatesOnly`.
- A scenario writer copying provision wording, which favours the lexical arm: Task 3's checker measures 8-gram overlap between each query and its expected provision text and returns cases above 30% overlap for rewriting.
- Components joined wrongly, so near-duplicates land in both cohorts: Task 5 `assign_test` with crafted families, amendments, consolidations and paraphrase links.
- Overfitting the grid to 40 development cases: Task 7 logs every trial; the finalist must beat baseline on development recall@20 by at least two citations and not lose on old product; sealed decides.
- History questions scored against current-law eligibility: Task 8 measures both policies on the cases tagged historical before choosing.

---

### Task 1: Suite support for development and sealed cohorts

**Files:** Modify `apps/corpus/internal/evaluation/{cases.go,run.go,report.go,metrics_report.go,cache.go}` and tests; create `apps/corpus/internal/evaluation/testdata/vn/development.json` (empty array, filled in Task 6); modify `apps/corpus/internal/stage/eval/eval.go` and `apps/corpus/internal/cli/eval.go` (`--sealed DIR`).

**Interfaces:** Development cases use the legacy case format plus `tags` (from `identifier`, `clause`, `multi_document`, `abbreviation`, `missing_date`, `historical`, `out_of_domain`, `negative`) and `component`; their identity entries live in `identity-map.json` like old product entries. A sealed run reads `DIR/cases.json` and `DIR/identity-map.json`, refuses a directory not owned by the user or not mode 700, and runs only with `--sealed`; without it, sealed is `not_run`.

- [ ] Tests: `TestDevelopment_ScoredSeparately`; `TestSealed_RefusesOpenDirectory`; `TestSealed_NoTextOrVectorsInArtifacts` (cache, cases rows, query vectors and report hold no sealed question text or vector); `TestSealed_AggregatesOnly` (report has cohort totals, intervals and gate results only); `TestSealed_NotRunWithoutFlag`.
- [ ] Implement, run unit and integration tests, commit `feat(eval): development and sealed cohorts`.

### Task 2: Scenario inventory

- [ ] A Codex worker with only the run's document list (number, title, type, issuer, dates; no provision text) and the tag list writes about 110 scenarios as JSON `{id, subject, situation, intent, tags}` in Vietnamese: about 85 positive across the corpus's subjects and every tag, about 25 negative (outside the corpus, questions needing advice rather than law, invented document numbers). Output to the scratchpad.
- [ ] A second worker on another model removes duplicates and scenarios that name an article or quote text; records why.

### Task 3: Queries

- [ ] For each scenario, a fresh worker that sees only that scenario writes the one query an agent would send to `search` (Vietnamese with diacritics, or a document number where the scenario says the user has it). Batch by 20 scenarios per worker, no worker sees labels or provision text.

### Task 4: Adjudication

- [ ] A worker with read access to the run's corpus database and official sources (VBPL, vanban.chinhphu.vn) labels each positive with expected citations `{doc_number, article?, clause?, point?, alt_doc_numbers?}` and evidence (`fetch_sha256`, node id, quoted span), or reclassifies it negative with a reason; current law at the evaluation date unless the scenario is historical.
- [ ] A second worker on another model rechecks every label against the quoted evidence; disagreements go to the executing session to settle from official text.
- [ ] The Task 3 overlap check runs here (8-gram overlap between query and expected provision text above 30% returns the case to Task 3 for a rewrite by a new worker).

### Task 5: Components and reservation

- [ ] A script (`work/` only, not committed) joins cases sharing a document family (amendments, consolidations, adjudicated alternates) or a narrow intent, joins negatives by intent, marks components touching the old set, then with seed `20261111` reserves sealed components first (none touching the old set) toward 40 cases, the rest to development; it writes the sealed cases and their identity entries straight into `work/eval-sealed/` with modes 700/600 and prints only counts, and writes development to the scratchpad. Test the joiner with crafted cases (`assign_test`).

### Task 6: Owner spot check and commit

- [ ] Show Danny 10 development cases (from the scratchpad) and tell him the path of 10 sealed cases to read himself; the executing session never opens sealed files. Apply his corrections (sealed corrections by Danny or by the assignment script, not by the executing session).
- [ ] Commit `development.json` and the development identity entries; `make eval` on the newest run; record development and old product results.

### Task 7: Trial runner and log

- [ ] A command (under `apps/corpus/cmd/` or `corpus eval --trial`) that collects arms once per case and re-ranks for each grid point: per-document cap {3, 4, 5, none}, dense and lexical depth {50, 100}, lexical weight {0.5, 0.75, 1.0}, RRF k {30, 60}; it writes one JSON line per trial with the parameters, development and old product recall@20, recall@8, MRR@8, abstention, current-law precision and compact response size p95, and never reads sealed files.
- [ ] Choose the finalist: best development recall@20 that beats baseline by at least two citations, loses nothing on old product recall@20 or MRR@8, keeps every gate and keeps compact p95 within the ADR 0013 size gate; freeze baseline and finalist recipes in the log.

### Task 8: History-question policy

- [ ] On cases tagged `historical` (development and old product), compare two policies: (a) default search returns up to 3 non-current hits after the current ones, clearly marked, and (b) agents pass `in_force_only:false` themselves as `guide` instructs; measure recall@20, current-law precision of primary hits and compact size; write the decision into `docs/design/retrieval.md` with the numbers, or keep (b) if (a) costs precision.

### Task 9: Sealed run and decision

- [ ] Run baseline and finalist once each with `--sealed`; apply ADR 0010 item 4 (sealed recall@20 paired interval above zero, guards, gates); record the outcome, intervals and counts in ADR 0010 History and the release-1 recipe in `docs/design/retrieval-recipe.md` if the finalist wins; otherwise the baseline stays and further tuning needs fresh sealed cases.
