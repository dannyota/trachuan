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

- [x] Labeled set from seeds: the 300 masked articles of taxonomy.md "Evaluation gate", drawn by `corpus taxonomy labels` from vn/20261115_3 (seed 1, 197 documents), frozen as `taxonomy-labels.json`.
- [x] Candidate selection: embedding and lexical candidates measured on the masked set fell far short of 0.95 (ADR 0004 History); taxonomy.md now limits each article to the subtrees of its document request's level-2 families and gates family recall at 0.95.
- [ ] Model-adjudicated sets (no Anthropic calls; Codex gpt-6-astra labels blind): inheritance (100 inherited consolidated articles) now; multi-family (100 unseeded articles, half from documents with a multi-topic document-level result) after the 8.4 screening pass, frozen before any multi-family scoring; post-2023 (100 articles). Multi-family and post-2023 frozen from vn/20261115_3 as taxonomy-multi-family.json and taxonomy-post-2023.json (gpt-6-astra, blind, under the labeling rule; the coordinator read 30 and then 20 items); inheritance unavailable (32 eligible). The coordinator reads 10 of multi-family and 20 of post-2023 before either gates.

### 8.3 Classify stage and batch submission

- [ ] Classify stage: request builder, Batch API submission per batch-submissions.md (intent log first, custom ids, polling, idempotent application, cost recorded per batch), result validation, thresholds calibrated per level on the labeled set.

### 8.4 Pilot (paid, small)

- [x] Confirm current Batch and model prices on Anthropic's pricing page (2026-11-16, cited in the classify PRs).
- [x] Screening pass inside the pilot budget (400 documents, 0.36 USD, 374 with two or more families): document requests only, for at most 400 documents drawn at random from those holding unseeded articles; it feeds the multi-family draw and measures nothing.
- [x] Labeled set only, both `claude-haiku-5-5` and `claude-sonnet-5-5`; family recall 0.970 and 0.973; deepest recall stayed below 0.70 for both models and for a two-step trial, so the LLM layer's scope is level 2 (ADR 0004 History 2026-11-17). Pilot ledger 5.28 USD.
- [x] Level-2 run of Haiku v2 and Sonnet v2 on the 200 model-adjudicated articles (2026-11-17, 3.93 USD, ledger 9.21; no fragments in either set). Level 2, multi-family / post-2023: Haiku recall 0.809 / 0.861, top-1 0.760 / 0.867, pair precision 0.592 / 0.641, family recall 0.852 / 0.889; Sonnet recall 0.809 / 0.852, top-1 0.780 / 0.876, pair precision 0.547 / 0.626, family recall 0.826 / 0.880. Additional-node precision over both sets: Haiku 25/104 (0.240), Sonnet 22/120 (0.183), floor 0.80. Seeded slice, all 300 / non-fragment 260 (40 fragments), level 2: Haiku v2 recall 0.873 / 0.881, top-1 0.786 / 0.791; Sonnet v2 0.920 / 0.935, 0.810 / 0.813. No row passes: family recall caps recall (given the family, level-2 recall is 0.95 to 0.98), and additional nodes are mostly wrong; Sonnet is no better than Haiku at level 2.
- [x] Post-2023 and multi-family references: Codex `gpt-6-astra` labels them blind under the labeling rule, the coordinator reads samples, and they are frozen (`corpus taxonomy freeze`).
- [x] Prompt-4 trial, Haiku (2026-11-17, 0.33 USD): each article request chose from the full level-2 menu, returning one primary and at most one additional node. Raw level 2, seeded non-fragment / multi-family / post-2023: recall 0.815 / 0.765 / 0.815, top-1 0.687 / 0.730 / 0.770, all below v2; additional 26/106. Top-1 0.90 needs a primary threshold of 0.96, which leaves 99 to 100% of articles unassigned; no additional threshold reaches 0.80. The split-half check holds as a fail (recall 0.00 to 0.07 in both directions). Not merged.
- [x] Pilot ledger closed at 9.54 of 15 USD. No further paid prompt trials: the LLM layer serves nothing, and level 2 comes from seeds and overrides only.
- [ ] Free offline nearest-neighbour experiment over the release's embeddings: seeded neighbours, leave-one-document-out on the seeded 260 (no neighbour from the same document or a related one), both adjudicated sets, the split-half check, coverage beside precision and recall; k and the agreement rule chosen on one half. A pass becomes a design proposal before any further code.

### 8.5 Full batch (paid, after the coordinator confirms the estimate)

- [ ] The cheapest model that passed the floors; unassigned gaps below threshold; evaluated corpus run; release.
