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
   and matcher rules unchanged, applied to each hit and to each complete
   fragment it carries, compared only with banhmi's accepted numbers; no floor
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
   ([ADR 0013](0013-compact-search-and-batch-read.md)). Current-law precision
   and the rejection checks are hard gates; abstention is a target (balanced
   accuracy of the low-confidence flag, both rates reported per cohort), because
   the flag keeps the hits and an agent reads it as a signal. Recall and MRR
   have targets that every report states, but a target is not a gate: a release
   ships when the gates pass and recall@20 and MRR@8 do not fall against the
   previous release. A change that alters only the abstention threshold or its
   calibration policy ships when matched citations are identical on every cohort
   and abstention balanced accuracy does not fall on either cohort. Reports
   state the small sample size; no interval or superiority claim is required.
5. Every evaluation reports returned evidence characters per response (p50,
   p95). A change that only reduces size ships when matched citations and gate
   results are identical on every cohort.
6. Abstention uses the best-member cosine of the first fused provision,
   including lexical-only hits; the threshold is calibrated on the old product
   and development cohorts together, per dense recipe, to the best balanced
   accuracy ([calibration](../design/calibration.md)).
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

Abstention as a target, not a gate, is an accepted trade-off: an agent may
refuse a flagged response that held the answer, or trust unflagged weak
evidence, and a release where every response is flagged, or none is, passes the
retrieval gates. Every report states both rates per cohort so that state is
visible, `guide` tells agents the flag is advisory and the hits remain, and the
non-regression rule in item 4 stops a calibration change that lowers balanced
accuracy on either cohort.

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
- 2026-11-12: abstention moved from gate to target and the calibration objective
  changed to balanced accuracy. Run vn/20261111_3, the first fit on both cohorts
  (20 negatives), chose threshold 0.726 under the "abstain on every negative"
  rule because one development negative scores 0.726 (a document that exists but
  lacks the annex the question needs); 79 of 84 old product and 44 of 50
  development positives then carried the low-confidence flag, and the 0.95
  abstention gate failed while retrieval was unchanged (old product recall@20
  80/93). The recipe keeps the hits on a low-confidence response, so the flag is
  a signal, not a refusal; a gate on it blocked releases for no evidence lost.
  Balanced accuracy on the same scores reaches about 0.86 at threshold 0.53
  (positives answered 111 of 134, negatives flagged 18 of 20).
- 2026-11-12: recipe 2 under item 4, measured on run vn/20261111_4 with the
  research overrides of Plan 5: per-document cap 5 and a default non-current
  tail of 2. Old product recall@20 80 to 85 of 93, recall@8 73 to 77, MRR@8
  0.692 to 0.705; development recall@20 68 to 83 of 113, recall@8 64 to 76,
  MRR@8 0.653 to 0.667; current-law precision 1.0 on both; compact p95 at 20
  hits 32.8k to 32.3k and 34.6k to 35.0k characters. Rejected: not-yet documents
  as primary hits (precision 0.952 and 0.935) and the agent-side
  `in_force_only:false` for historical questions (no change, the complement only
  gets leftover slots).
- 2026-11-12: the filter gate joins `quality_pass` for releases that serve
  taxonomy nodes ([evaluation](../design/evaluation.md)): per old product case
  and mode, an inside or partial cohort and an outside cohort; filtered recall@8
  is not lower than unfiltered recall@8 for the seeded citations, and every
  expected citation is within the first 100 paged hits at `limit` 8. Its status
  uses the floor vocabulary: `pass`, `fail`, `unavailable` with no inside case,
  `out_of_scope` without served nodes, and `not_run` under a research override.
  Subject filters only reorder, so the gate guards that a filter never costs an
  expected citation.
- 2026-11-12: recipe 2 is the release-1 recipe. Its first build, run
  vn/20261112_2, also carries relations from text and the extraction fixes made
  after vn/20261111_4, so the change against release vn/20261111_4 is not the
  recipe's alone. Old product recall@20 80 to 86 of 93, recall@8 73 to 78, MRR@8
  0.692 to 0.718; development recall@20 68 to 83 of 113, recall@8 64 to 74,
  MRR@8 0.6530 to 0.6533; current-law precision 534 of 534 and 390 of 390;
  rejections 3 of 3; filter gate 83 of 83; exact and ANN equal. Calibration on
  both cohorts chose threshold 0.530 (balanced accuracy 0.864; old product
  0.911, development 0.853). Compact p95 at 20 hits 33.8k characters, full p95
  at 8 hits 84.3k; the size gate passes. Development recall@8 and MRR@8 stay
  below the trial's 76 and 0.667; the cause is not examined.
- 2026-11-13: recipe 3 adopts the
  [sibling clause merge](../design/retrieval-siblings.md). Research override on
  run vn/20261113_1 against release vn/20261112_2, reproduced after rebase: old
  product recall@20 86 of 93 unchanged, MRR@8 0.7182 to 0.7247, recall@8 78 to
  81; development recall@20 83 to 84 of 113, MRR@8 0.6533 to 0.6616, recall@8 74
  to 81; current-law precision 534 of 534 and 390 of 390; abstention 0.911 and
  0.853; compact p95 at 20 hits 35,519 characters, so the size gate passes. Item
  4 passes. Serving accepts recipes 2 and 3; the first recipe-3 build
  recalibrates.
- 2026-11-13: legacy parity reads complete fragments (scorer `agent-eval/4`).
  Recipe 3 folds an article's member clauses into one hit cited by the article,
  so banhmi's matcher, which read one clause per hit, lost rank-1 credit on run
  vn/20261113_3 for expectations such as Điều 11 khoản 1 while the product path
  credited the same complete fragment. The number and citation rules stay
  banhmi's and also apply to each complete fragment's own citation, in a
  recipe-2 parent hit as in a merged hit, as product matching already does; a
  truncated fragment earns nothing. Legacy parity stays comparable with
  banhmi's numbers, where those clauses were separate hits.
