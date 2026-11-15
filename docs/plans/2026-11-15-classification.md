# Classification Plan (Plan 8)

> **For agentic workers:** Routes (`instructions/workflow.md`): every task that submits or prepares paid batches goes to Codex `implementer` with `--model gpt-6.1-sol`, reviewed by `reviewer` with `--model gpt-6-astra`; the design delta is written by the primary and reviewed by `gpt-6-astra`. Executor: trachuan-dev. Paid calls use `ANTHROPIC_API_KEY_DANNY` only, passed per command, never exported, never switched while a batch is in flight.

**Goal:** every primary article in the release carries subject nodes: seeds from the 2023 codification package, then inheritance through consolidations, then an LLM layer in batches for the rest, gated by the floors in [taxonomy](../design/taxonomy.md#evaluation-gate).

**Budget:** owner cap 150 USD on the key, pilot included. Pilot budget 15 USD. The full batch is pre-approved when the pilot passes every applicable floor and the projection, pilot included, stays under 100 USD; otherwise, and before any Opus run, report to the coordinator.

**Spec:** [taxonomy](../design/taxonomy.md) ("Classification layers", "Serving", "Evaluation gate"), [batch submissions](../design/batch-submissions.md), [embedder](../design/embedder.md) for node vectors, [evaluation](../design/evaluation.md) filter gate. The spec wins over this plan.

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

- [x] taxonomy.md: consolidation inheritance layer (confidence 0.9, union of sources) with its own slice (precision 0.90 at level 2).
- [x] taxonomy.md: LLM request shape (chapters; groups of 12 without chapters), model, effort and prompt in the recipe, output codes outside the candidates dropped; post-2023 slice model-adjudicated by Codex `gpt-6-astra`, blind, 20 read by the coordinator; custom-node slice not applicable while `custom.yaml` is empty.

### 8.1 Seeds and inheritance in build (no paid calls)

- [ ] Build computes seed assignments and inheritance assignments, with `seed_article_missing` gaps; the filter gate in evaluation runs on them; release serves them without the LLM floors.

### 8.2 Candidates and labeled set (no paid calls)

- [ ] Node candidate text per taxonomy.md "Serving", embedded with the dense document recipe; candidate recall at 10 measured on the labeled set before any prompt work.
- [ ] Labeled set from seeds: 300 masked seeded articles and 100 with labels in two or more families, sampled by document, frozen with a hash.

### 8.3 Classify stage and batch submission

- [ ] Classify stage: request builder, Batch API submission per batch-submissions.md (intent log first, custom ids, polling, idempotent application, cost recorded per batch), result validation, thresholds calibrated per level on the labeled set.

### 8.4 Pilot (paid, small)

- [ ] Confirm current Batch and model prices on Anthropic's pricing page.
- [ ] Labeled set only, both `claude-haiku-5-5` and `claude-sonnet-5-5`; iterate prompts until every applicable floor passes; report cost per 1,000 articles per model.
- [ ] Post-2023 reference: Codex `gpt-6-astra` labels 100 post-2023 articles blind; the coordinator reads 20 before it gates.

### 8.5 Full batch (paid, after the coordinator confirms the estimate)

- [ ] The cheapest model that passed the floors; unassigned gaps below threshold; evaluated corpus run; release.
