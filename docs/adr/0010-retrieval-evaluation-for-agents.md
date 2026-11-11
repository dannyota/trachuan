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

1. Three cohorts. **Legacy parity**: banhmi's 92 cases with their expectations
   and matcher unchanged, compared only with banhmi's accepted numbers; no floor
   applies. **Old product**: the same 92 cases under product scoring; the three
   unaccented cases become rejection checks that must all return the ADR 0002
   diacritics message and sit outside the recall, MRR and abstention
   denominators; the other 89 keep their expectations. **Development**: the new
   cases below. The targets and gates in
   [testing](../../instructions/testing.md) apply to the old product and
   development cohorts, each on its own.
2. Product scoring matches exact document and provision identities, with
   alternates adjudicated from official text; no substring credit. Product
   current-law precision counts every returned primary hit.
3. New cases: about 40 development cases, 30 positive and 10 negative, committed
   and readable by everyone who works on search. Each scenario is written from a
   legal subject, not from provision text; an agent that has not seen the
   provision writes the query it would send to `search`; a second pass
   adjudicates labels against official text; the owner spot-checks 10 cases. The
   same cases score later recipe, corpus and model changes. Cases are added from
   agent error reports and owner findings and are never removed to raise a
   score.
4. Change rule: a recipe or model change targets a named failure class seen in
   case results, such as a per-document cap that drops expected provisions,
   never a parameter grid. It ships when every gate passes on the old product
   and development cohorts, the net count of matched expected citations within
   20 hits rises on at least one cohort (same fixtures and scoring modes before
   and after), and recall@20 and MRR@8 fall on neither cohort; otherwise the
   current recipe stays. Recall@20 counts because 20 compact hits are what an
   agent receives by default
   ([ADR 0013](0013-compact-search-and-batch-read.md)). Current-law precision,
   abstention and the rejection checks are hard gates. Recall and MRR have
   targets that every report states, but a target is not a gate: a release ships
   when the gates pass and recall@20 and MRR@8 do not fall against the previous
   release. Reports state the small sample size; no interval or superiority
   claim is required.
5. Every evaluation reports returned evidence characters per response (p50,
   p95). A change that only reduces size ships when matched citations and gate
   results are identical on every cohort.
6. Abstention uses the best-member cosine of the first fused provision,
   including lexical-only hits; the threshold is calibrated on the old product
   and development cohorts together, per dense recipe.
7. Models are compared exact search against exact search on the same cases; ANN
   loss and cost are measured separately per model. Vietnamese_Embedding_v2
   stays unless a different model passes item 4.
8. The release contract gains versioned `chunk_recipe` and `retrieval_recipe`
   fields; [data](../design/data.md) changes with the index plan.

## Consequences

About 40 new labeled cases, visible to everyone, and about 130 scored product
cases in all, with low statistical power stated in every report. No result
proves a recipe better on unseen questions; the guards are that changes target
named failure classes and must not fall on the old product cohort, which the
model choice already used. Case authoring stays small enough to repeat as the
corpus grows. The three unaccented legacy cases count as rejection checks in the
old product cohort and as written in legacy parity. The index plan takes its
chunk, lexical and retrieval recipes from the experiments this protocol scores.
Gates and targets change only with a History line. These metrics measure
retrieval and response size, not the quality of an agent's final answer or its
real token spend.

## Alternatives rejected

- Tune on the 92 legacy cases: they already chose the model, so gains would be
  overfit.
- 120 new cases with five folds (first draft of the study): more labeling with
  no power justification.
- MRR@8 or nDCG as the primary endpoint: rank-weighted metrics suit a person
  reading the first hit; an agent reads the list, so coverage within it matters
  more.
- Questions written from provision text: shared wording favors the lexical arm.
- A sealed cohort of about 40 cases with isolated authoring and one sealed run
  per finalist (this ADR's 2026-11-09 text): it proves superiority on unseen
  questions, which matters when training or benchmarking models. trachuan
  applies a fixed model, and the isolation cost bought no decision the visible
  cases cannot make.

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
- 2026-11-11: baseline scope correction, not a tuning trial. Recipe 1 admits
  `relation_context` documents only by number reference
  (`context_scope:"number-routed/1"`). Corpus scope added one-hop context
  documents after the 2026-11-09 baseline; searching them as primary evidence
  took 82 of 736 legacy hit slots on run `vn/20261110_4`, matched no expected
  citation and raised the provisional abstention threshold through three
  out-of-scope negatives. The correction restores the candidate set the baseline
  measured (documents selected as scope). Promotion still needs development and
  sealed cohorts under items 4 to 6, and the threshold stays labeled
  provisional.
- 2026-11-11: the owner set recall and MRR as targets, not gates ("dont try to
  be 0.9, that is number, we do our best") and left the cut-off to the primary,
  who chose recall@20, the compact default agents receive. Gates are current-law
  precision, abstention, rejections and no regression of recall@20 or MRR@8
  against the previous release; recall@8 stays a reported diagnostic.
- 2026-11-11: the owner removed the sealed cohort: "we dont try to train ai
  model and benchmarks, we use and apply AI, improve for it, and when we change
  embedding models, we can reuse them too". Items 1 and 3 to 7 now use one
  visible development cohort of about 40 cases, a change rule of named failure
  classes with non-regression on both cohorts instead of a sealed paired
  interval, and calibration on old product and development together.
  `work/eval-sealed/` is not created.
- 2026-11-11: the development cohort landed with 65 cases (50 positive, 15
  negative, 110 expected citations) instead of the 40 planned, because the
  authoring produced more usable scenarios than needed and more cases make the
  measurement steadier; scenarios and queries were written by Claude subagents
  without provision text, labels adjudicated against official text of run
  vn/20261111_2, rechecked by a second model, 10 cases spot-checked by the
  owner. The spot check turned one negative into a positive.
