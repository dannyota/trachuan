# Retrieval evaluation harness

Implementation of [ADR 0010](../adr/0010-retrieval-evaluation-for-agents.md). It
measures the legacy, old product and development cohorts; development is
`not_run` while its case file is empty. A low score is a result, not grounds to
remove cases or lower a floor.

## Package, command and stage

`apps/corpus/internal/evaluation/` owns `cases.go`, `mapping.go`, `legacy.go`,
`product.go`, `metrics.go`, `calibrate.go`, `cache.go`, `report.go`, `run.go`.
`apps/corpus/internal/stage/eval/` adapts the stage interface; the CLI adapter
lives in `apps/corpus/internal/cli/eval.go`. Both call
`Run(ctx context.Context, engine *retrieval.Engine, suite Suite, options Options) (Report,Cache,error)`.
`Options` contains frozen civil date, modes, recipe hashes, previous report path
and fixture hashes, never arbitrary SQL. Separate
`Calibrate(ctx,engine,suite) (Calibration,error)` is called by build before
publishing its final recipe.

Freeze the engine clock to the build's evaluation date, but leave request `AsOf`
nil and `InForceOnly` true for both old cohorts and calibration. Setting `AsOf`
would change the test to historical eligibility. Research-only overrides on
`corpus eval` (the per-document cap, and validity policy overrides for cases
tagged `historical`, a non-current tail and not-yet documents) may change a
trial's request or shaping; each is recorded on the report and on every record
that carries `limit`, never reaches serving or the stage, and a mismatch makes
`previous_diff` incompatible. Cache/report fields distinguish `evaluation_date`
from nullable `request_as_of`. Use the evaluation deadline profile for every arm
and serialize case execution within the item.

CLI: `corpus eval <cc> <rel> --out <directory> [--previous <report.json>]`. It
verifies/resolves the build artifact, opens that release, runs exact and ANN
plus arm controls, and writes the same report/cache as the stage. Exit 0 means
the diagnostic run completed, including measured floor failures. Exit 1 means
invalid inputs, infrastructure failure or incomplete scoring. Optional
`--require-floors` exits 2 for failed or unavailable old-product or development
floors or a failed rejection check (development floors that are `not_run`
because the file is empty do not count), for future automation; it cannot turn
an incomplete run into a passing report.

Stage key is `<cc>/<rel>`, one item after a done build. Input hash combines
verified build artifact hash, fixture and mapping hashes, previous report hash
or empty string, frozen date, and mode list in fixed order. Recipe hash covers
scorer version `agent-eval/1`, legacy matcher version, product matcher version,
metric definitions, rejection IDs/message, cache schema/depths, quantile rule,
and floors. It reads but never changes the dump or singleton release row.
Artifact files: `report.json`, `cases.jsonl`, `arms.jsonl`, `queries.f32`,
`query-index.json`, `MANIFEST.sha256`. `Rows` is a no-op; `Resolve` exposes the
report status and regenerates evaluation gaps: one `eval_floor_failed` per
failed or unavailable old-product floor (detail `<mode>:<metric>`) or
development floor (detail `development:<mode>:<metric>`) and one
`eval_rejection_failed` per failed rejection check (detail the case ID). The
owner may accept them; release stays blocked by `release_eligible`. No
production queries enter these artifacts.

## Fixtures and identity adjudication

Case fixtures, `identity-map.json` and identity validation are specified in
[evaluation fixtures](evaluation-fixtures.md).

## Scoring contracts

Legacy has 87 positive cases, 96 expected citations and 5 negatives. Reimplement
the Vietnam branch of banhmi's matcher: trimmed case-insensitive number
equality, otherwise uppercase and remove non-ASCII alphanumerics, then equality
or containment in either direction. Apply the same to alternate numbers.
Citation matching splits on commas/Unicode whitespace; case-insensitive tokens
`điều`, `khoản`, `điểm` must be followed by the expected value after removing
edge parentheses and periods. Document-only `relation_ok` also matches a
returned confirmed relation's other document number. No relation credit when
article/clause/point is specified. Report direct-only scores alongside legacy
relation credit. Guard fixtures against empty expected numbers.

Legacy evaluation bypasses only the product diacritics rejection through
`Engine.PrepareLegacy(ctx,request)` followed by `Engine.Collect`; it still
enforces size/token and SQL safety limits, and does not restore accents or add
Qwen instructions. The method is a trusted evaluation API, never exposed by the
server. All other candidate and shaping choices are recipe 1. Call the result
`legacy_matcher_on_product_recipe`, not a reproduction of banhmi's engine.
Compare descriptively with the accepted numbers and the
[local exact baseline](../research/retrieval-hybrid-results-2026-11.md),
labeling corpus, model and policy differences; no legacy floor or parity claim.

Old product uses 84 positive cases with 93 citations and 5 negatives. The
remaining three cases are rejection checks: `edge-no-diacritics-infosec`,
`edge-no-diacritics-payment`, `edge-no-diacritics-mangluoi`. All must return
`diacritics_required` and the exact
[message](retrieval-recipe.md#validation-and-routing), no hits and no embed
request. Exclude them from recall, MRR, precision and abstention denominators;
report 3/3 separately as a hard gate.

Product matching is exact country/key/anchor membership in the adjudicated
alternatives. A document-level expectation accepts any returned provision of
that exact document. A parent hit earns child credit only when a fragment names
the exact child identity with `Complete:true`; a truncated parent holding part
of a child earns nothing for that child. A child hit does not automatically
satisfy a parent expectation. A relation-backed alternative requires the
explicit mapped edge and evidence hash in the response. Count each expectation
at most once, at its first matching rank; two hits or alternates never increase
its denominator. No number substring, article-label-only or family membership
credit.

## Metrics

Score separate limit-8 and limit-20 responses, including their own hydration,
cap/backfill and abstention. Also score dense-only and lexical-only controls
under identical eligibility and response processing; only hybrid gates apply.

- Micro recall@k: sum matched expectations / sum expected expectations over
  positive cases, including absent mappings. Macro recall averages per-case
  fractions. MRR@8 averages `1/first_correct_rank`, or zero for a miss, over all
  positive cases. Empty/abstaining positives remain in denominators. Returned
  low-confidence evidence still counts as returned evidence for recall.
- Current-law precision (stored-status consistency): current hits / all hits of
  the primary response group, any class, in limit-8 responses. Count every hit,
  including hits on negative cases and abstaining responses; unknown validity is
  not current. Report unknown and contradictory status counts and non-current
  group precision separately. Legacy only trims a trailing non-current run,
  unless all hits are non-current, in which case all count. Zero denominator is
  null/N/A, never 1. Independently adjudicated legal validity is not claimed.
- Abstention accuracy: correct abstain flag / all non-rejection cases. Treat
  abstention as the positive class: TP negative case abstained; FN negative
  answered; FP positive case abstained; TN positive answered. Also report
  negative recall TP/(TP+FN), positive answer rate TN/(TN+FP) and balanced
  accuracy, the mean of those two rates, which is the abstention target. No hits
  means abstain; an infrastructure error means evaluation incomplete, not
  abstention. Low-confidence responses keep their hits, so the flag is a signal
  agents read, not a loss of evidence; recall and MRR score the hits regardless.
  Balanced accuracy is a target record (`targets`, like recall@20 and MRR@8,
  target 0.85, per cohort, with both rates and the confusion counts); it never
  enters `floors`, `quality_pass`, `--require-floors` exit codes or
  `eval_floor_failed` gaps, and `non_regression` compares it between releases.
- Evidence characters: sum Unicode code points actually returned in hit text and
  relation evidence, across both groups. Count repeated bytes again if sent
  twice; omit keys, scores, titles, context prefixes and structural JSON.
  Include empty responses as zero. Report p50/p95 separately for each limit,
  mode and cohort using nearest rank `sorted[ceil(p*n)-1]`; null for no
  responses.
- ANN candidate recall: raw eligible ANN top-depth overlap with exact top-depth
  divided by exact top-depth count, before fallback; zero exact candidates is
  N/A. Report fallback frequency and effective post-fallback overlap too. Report
  exact minus ANN hybrid recall@8, MRR@8 and recall@20 in percentage points plus
  citation/decision differences. No model comparison mixes modes.

Old-product floors are those in [testing](../../instructions/testing.md),
checked independently for exact and serving ANN hybrid. Report each as `pass`,
`fail`, or `unavailable` with numerator, denominator, value and floor. Any
unavailable required metric cannot pass. Development floors use the same records
and are `not_run` while the development case file is empty. Taxonomy
classification floors are `out_of_scope` until the LLM layer lands. Filtered
cases run when the release serves nodes, in three cohorts per golden case and
mode. Inside: every expected citation has effective nodes; the filter is their
deepest effective nodes, or their families when those exceed 20, and the case
passes when filtered recall@8 is at least unfiltered recall@8 and every expected
citation is within the first 100 paged hits (pages of `limit` 8,
[retrieval](retrieval.md#filter-contract)). Partial: some but not all expected
citations have effective nodes; the filter is the seeded citations' nodes chosen
the same way, and the case passes when every expected citation is within the
first 100 paged hits and the seeded citations' recall@8 is at least their
unfiltered recall@8. Outside: the filter is the served family with the lowest
code that holds none of the expected citations, and the case passes when every
expected citation is within the first 100 paged hits; with no such family the
case is skipped with reason `no_outside_family`. A case whose families still
exceed 20 is skipped with reason `too_many_families`; skipped cases are counted
by reason. The filter gate passes when every case in every cohort passes and the
inside cohort has at least one case; with no inside case it is `unavailable`,
and it is `out_of_scope` when the release serves no nodes
([retrieval](retrieval.md#filter-contract)). `quality_pass` means both hybrid
modes meet all old-product floors, and development floors once development cases
exist, the filter gate when the release serves nodes, and all three rejection
checks pass; legacy has no gate. `release_eligible` is always false in this
baseline, including when `quality_pass` is true. Measured failures leave the
stage done and the run evaluated, with `quality_pass:false` and
`release_eligible:false`. Per-case `mapping_incomplete` is a reported
diagnostic, not a run failure. Malformed fixture/map files, incomplete case
execution or infrastructure failure fail the stage. CLI and status must
distinguish completed evaluation from passed quality gates.

## Cache and report format

Cache exact dense and lexical arms to the study's maximum trial depths in
budgets. Recipe-1 responses use only their own shallower prefixes. Store ANN
recipe-depth raw and fallback lists separately. `arms.jsonl` rows have
`version,case_id,cohort,release_hash,recipe_hashes,evaluation_date, request_as_of,mode,depths,route, eligibility_hash,dense[],lexical[],raw_ann[],fallback,groups[],diagnostics`.
Entries store chunk id, owning identity, rank and raw cosine or BM25 score;
`groups[]` holds the recipe-1 groups of the shaped responses. Alternate-group
evidence, member cosines for other groupings and consolidation family roots are
deferred to the tuning plan, which hydrates them from the immutable dump and
these deep arm lists. Exceeding the cache budget fails explicitly. Changing
tokenizer, prefix or dictionary needs fresh arms. This work implements only
recipe-1 shaping, not the trial runner.

Reuse one query vector per accepted fixture input and recipe across limits, arm
controls and modes, through one `Engine.Prepare` (or `PrepareLegacy`) per input
whose vector the cache writes to `queries.f32`. Exact deep-cache prefixes
produce shallower exact arms; collect ANN separately at recipe-1 depth/settings.
Record the database instance kind (`built` or `restored`) because restored HNSW
pages may differ. Eval scores the resolved database it actually opens; the
primary also runs the CLI after a restore and retains both reports as recovery
evidence.

`queries.f32` uses the embedding artifact's binary format; `query-index.json`
maps case IDs/cohorts to rows, counts, query UTF-8 hashes and query recipe hash.
Rejected product inputs have `{case_id,cohort,reason}` with no vector row; their
legacy counterpart may still have a vector. Shared inputs point to the same row
only after exact input/recipe equality. Only authored fixture inputs are
eligible for caching. No questions in logs or report copies. Cache key includes
dump, fixtures, map, query vectors, all recipes, date, tie rules and software
versions; mismatches invalidate reuse.

`report.json` fields: `version:1`, `status`, `quality_pass`, `release_eligible`,
`country`, `rel`, `build_artifact_sha256`, `dump_sha256`, `fixture_hashes`,
`identity_map_sha256`, `recipes`, `evaluation_date`, `request_as_of`,
`database_instance_kind`, `software`, `calibration`,
`cohorts:{legacy,old_product,development}`, `floors`, `rejections`, `coverage`,
`ann_loss`, `timings_ms`, `evidence_characters`, `cache_manifest`,
`previous_diff`, `not_run`, `limitations`, and the counts in
[scope adoption](scope-adoption.md#readers-and-evaluation). Each metric stores
counts as well as the value. `coverage` lists each missing original/alternate
document number and affected case/citation, including the five known omissions
in the ingest research. Never remove them because the source sweep did not find
them.

`mapping_incomplete` lists each validation issue by cohort, case ID, citation
index and detail, sorted in that order; affected unavailable recall and MRR
metrics, targets and non-regression records carry `reason: mapping_incomplete`
and `mapping_incomplete_count` of distinct affected cases.

Only `status:"complete"` reports enter a successful stage artifact. Each cohort
has `case_count`, `positive_count`, `negative_count`, `rejection_count`,
`expected_citations` and `metrics[]`. A metric record is
`{mode,arms,limit,name,numerator,denominator,value}`; value is null for a zero
denominator. For macro recall and MRR, numerator is the sum of case fractions or
reciprocal ranks; otherwise it is an integer count. Confusion counts are
`{tp,fp,tn,fn}` alongside abstention metrics. Each floor record adds `floor` and
`status` from the vocabulary above. Size summaries hold `{n,p50,p95}`; timing
summaries hold `{n,p50,p95,p99}` in milliseconds, using the same quantile rule.

`cases.jsonl` records case ID, mapping status, expected/matched counts, ranks,
mapped identities, returned identities, arm/fusion/group/cap/abstention losses,
decision reason, validity gaps and character counts, without query text.
`previous_diff` is null with a reason when no previous report exists; otherwise
compare per-case gains/losses and reject incompatible scorer/fixture versions.
Corpus changes are labeled coverage changes, not an isolated model effect.
Report low sample size and old-cohort reuse; no superiority interval or claim is
needed here.
