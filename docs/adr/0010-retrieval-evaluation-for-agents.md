# ADR 0010: Retrieval evaluation for agent consumers

Status: accepted 2026-11-09. The owner delegated these choices with the rule "we
build for agents, what compares better".

## Context

Search is hybrid: dense, BM25 and fusion. ADR 0009 chose the dense model on
dense-only numbers. The [hybrid study](../research/retrieval-hybrid-2026-11.md)
proposes how to judge the whole response and which recipes to test; an
independent review corrected it before these decisions.

The main consumers are AI agents over MCP. An agent reads every hit it receives,
pays for each hit in context tokens, writes Vietnamese with diacritics, and
cites what it is given. banhmi's 92 golden cases already guided the model
choice, and three of them send unaccented queries that ADR 0002 rejects.

## Decision

1. Four cohorts. **Legacy parity**: banhmi's 92 cases with their expectations
   and matcher unchanged, compared only with banhmi's accepted numbers; no floor
   applies. **Old product**: the same 92 cases under product scoring; the three
   unaccented cases become rejection checks that must all return the ADR 0002
   diacritics message and sit outside the recall, MRR and abstention
   denominators; the other 89 keep their expectations. **Development** and
   **sealed**: the new cases below. The floors in
   [testing](../../instructions/testing.md) apply to the old product and sealed
   cohorts, each on its own.
2. Product scoring matches exact document and provision identities, with
   alternates adjudicated from official text; no substring credit. Product
   current-law precision counts every returned primary hit.
3. New cases: about 40 development and 40 sealed, 30 positive and 10 negative in
   each, grouped by connected document family and intent as the study defines.
   Each scenario is written from a legal subject, not from provision text; an
   agent that has not seen the provision writes the queries it would send to
   `search`; a second pass adjudicates labels against official text; the owner
   spot-checks 10 cases per cohort. A separate agent session writes the sealed
   cases straight into `work/eval-sealed/` (mode 700, never committed until
   release). Until the baseline and one finalist are frozen, no brief, scratch
   file, prompt or tuning context may contain sealed queries or labels; the
   scoring script reads them and prints aggregates only.
4. Promotion order: every gate passes on the old product and sealed cohorts;
   then sealed product micro-recall@8 decides, and its paired 95% interval for
   finalist minus baseline must lie above zero; recall@8 must not fall on the
   old product cohort, and MRR@8 and recall@20 must not fall on either cohort.
   Otherwise the baseline stays. Current-law precision and abstention are hard
   gates.
5. Every trial reports returned evidence characters per response (p50, p95).
   Size breaks ties only on the development cohort, between configurations of
   the same model with identical matched-citation counts and abstention
   decisions: lower p95 first, then lower p50. A size-only promotion of a frozen
   finalist needs identical matched citations and gate results on every cohort,
   and never changes the model.
6. Abstention uses the best-member cosine of the first fused provision,
   including lexical-only hits; the threshold is calibrated on development cases
   per dense recipe.
7. Models are compared exact search against exact search; ANN loss and cost are
   measured separately per model. Vietnamese_Embedding_v2 stays unless a
   different model wins under item 4.
8. The release contract gains versioned `chunk_recipe` and `retrieval_recipe`
   fields; [data](../design/data.md) changes with the index plan.

## Consequences

About 80 new labeled cases before any superiority claim, with low statistical
power stated in every report. The three unaccented legacy cases count as
rejection checks in the old product cohort and as written in legacy parity. The
index plan takes its chunk, lexical and retrieval recipes from the experiments
this protocol scores. Floors do not change. These metrics measure retrieval and
response size, not the quality of an agent's final answer or its real token
spend.

## Alternatives rejected

- Tune on the 92 legacy cases: they already chose the model, so gains would be
  overfit.
- 120 new cases with five folds (first draft of the study): more labeling with
  no power justification.
- MRR@8 or nDCG as the primary endpoint: rank-weighted metrics suit a person
  reading the first hit; an agent reads the list, so coverage within it matters
  more.
- Questions written from provision text: shared wording favors the lexical arm.

## History

- 2026-11-09: accepted after the study's review findings S-01 to S-06 were
  fixed; cohorts, tie-break order, the sealed-case location and old-cohort
  recall@8 non-regression clarified after findings S-07 to S-10.
- 2026-11-09: the primary moved baseline implementation before tuning: recipe 1
  reproduces the study's baseline algorithms with ADR 0009 dense embedding and
  product settings on our corpus. Build the index and old-cohort harness first;
  later trials use cached arm rankings with our document/provision identities.
  This replaces the earlier experiments-before-index order. The baseline
  threshold is calibrated on old product only as a provisional exception to item
  6, labeled until development cases exist; no superiority claim follows. New
  cohorts, tuning, taxonomy, signing, upload and activation remain separate.
