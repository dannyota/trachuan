# Classification Plan (Plan 8)

> **For agentic workers:** Routes (`instructions/workflow.md`): every task that submits or prepares paid batches goes to Codex `implementer` with `--model gpt-6.1-sol`, reviewed by `reviewer` with `--model gpt-6-astra`; the design delta is written by the primary and reviewed by `gpt-6-astra`. Executor: trachuan-dev. Paid calls use `ANTHROPIC_API_KEY_DANNY` only, passed per command, never exported, never switched while a batch is in flight.

**Goal:** every primary article in the release carries subject nodes: seeds from the 2023 codification package, then inheritance through consolidations, then an LLM layer in batches for the rest, gated by the floors in [taxonomy](../design/taxonomy.md#evaluation-gate).

**Budget:** owner cap 150 USD on the key, pilot included. Pilot budget 15 USD. The full batch is pre-approved when the pilot passes every applicable floor and the projection, pilot included, stays under 100 USD; otherwise, and before any Opus run, report to the coordinator. Admission: before each submission or retry, settled cost plus the reserved cost of every unsettled batch plus this batch's reserve (counted input tokens at the batch input price, plus every request's `max_tokens` at the batch output price) must fit the applicable budget (pilot 15 USD, full batch 100 USD, cap 150 USD); otherwise it is not submitted. A batch's reserve is released only when its cost is settled from its results.

**Spec:** [taxonomy](../design/taxonomy.md) ("Classification layers", "Serving", "Evaluation gate"), [batch submissions](../design/batch-submissions.md), [evaluation](../design/evaluation.md) filter gate. The spec wins over this plan.

## Measured size (corpus_vn_20261114_5, 2026-11-15)

| Item | Count |
| --- | --- |
| primary documents | 2,725 (445 consolidated) |
| primary articles | 23,688 |
| seeded by package ItemID and article number | 5,020 (21.2%) |
| unseeded in consolidated documents | 2,458 |
| unseeded elsewhere (LLM set before inheritance) | 16,210 in 1,663 documents |
| article text, embedder tokens | mean 649, p50 242, p95 2,263 |
| articles per chapter | 5.6 |

## Tasks

### 8.0 Design delta (primary, astra review)

- [x] taxonomy.md and ADR 0004: consolidation inheritance through corresponding articles (same anchor and heading), method `inherited` below seeds and above `llm` (data.md, corpus-schema.md), confidence 0.9, its own precision rows at levels 2, 3 and deepest; each layer gated by its own rows.
- [x] taxonomy.md: LLM request shape (chapters; groups of 12 without chapters), model, effort and prompt in the recipe, request size bounds, output keyed by article and validated against that article's candidates, `too_long` and `invalid_response` gaps; post-2023 slice model-adjudicated by Codex `gpt-6-astra`, blind, 20 read by the coordinator, then frozen with a hash; custom-node slice not applicable while `custom.yaml` is empty.

### 8.1 Seeds and inheritance in build (no paid calls)

- [ ] Migration adds `inherited` to the assignment method check. Build computes seed and inheritance assignments, with `seed_article_missing` gaps and the count of consolidated articles without a corresponding source; the inheritance rows and the filter gate run on them; seeds serve without floors, inherited assignments only when their rows pass.

### 8.2 Candidates and labeled set (no paid calls)

- [ ] Labeled set from seeds: the 300 masked articles of taxonomy.md "Evaluation gate", drawn by `corpus taxonomy labels`, frozen with a hash.
- [x] Candidate selection: embedding and lexical candidates measured on the masked set fell far short of 0.95 (ADR 0004 History); taxonomy.md now limits each article to the subtrees of its document request's level-2 families and gates family recall at 0.95.
- [ ] Model-adjudicated sets (no Anthropic calls; Codex gpt-6-astra labels blind): inheritance (100 inherited consolidated articles) now; multi-family (100 unseeded articles, half from documents with a multi-topic document-level result) after the 8.4 screening pass, frozen before any multi-family scoring; post-2023 (100 articles). The coordinator reads 10 of multi-family and 20 of post-2023 before either gates.

### 8.3 Classify stage and batch submission

- [ ] Classify stage: request builder, Batch API submission per batch-submissions.md (intent log first, custom ids, polling, idempotent application, cost recorded per batch), result validation, thresholds calibrated per level on the labeled set.

### 8.4 Pilot (paid, small)

- [ ] Confirm current Batch and model prices on Anthropic's pricing page.
- [ ] Screening pass inside the pilot budget: document requests only, for at most 400 documents drawn at random from those holding unseeded articles; it feeds the multi-family draw and measures nothing.
- [ ] Labeled set only, both `claude-haiku-5-5` and `claude-sonnet-5-5`; measure family recall of the document request first (start with `F` = 5), then iterate prompts until every applicable floor passes; report cost per 1,000 articles per model.
- [ ] Post-2023 reference: Codex `gpt-6-astra` labels 100 post-2023 articles blind; the coordinator reads 20 before it gates.

### 8.5 Full batch (paid, after the coordinator confirms the estimate)

- [ ] The cheapest model that passed the floors; unassigned gaps below threshold; evaluated corpus run; release.
