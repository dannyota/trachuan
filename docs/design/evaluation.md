# Retrieval evaluation harness

Proposed implementation of
[ADR 0010](../adr/0010-retrieval-evaluation-for-agents.md). This work measures
the legacy and old product cohorts only. Development cases, sealed cases, tuning
experiments and promotion decisions come later. Never create or read
`work/eval-sealed/` during this work. A low baseline score is a result, not
grounds to remove cases or lower a floor.

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
would change the test to historical eligibility. Cache/report fields distinguish
`evaluation_date` from nullable `request_as_of`. Use the evaluation deadline
profile for every arm and serialize case execution within the item.

CLI: `corpus eval <cc> <rel> --out <directory> [--previous <report.json>]`. It
verifies/resolves the build artifact, opens that release, runs exact and ANN
plus arm controls, and writes the same report/cache as the stage. Exit 0 means
the diagnostic run completed, including measured floor failures. Exit 1 means
invalid inputs, infrastructure failure or incomplete scoring. Optional
`--require-floors` exits 2 for failed or unavailable old-product floors, for
future automation; it cannot turn an incomplete run into a passing report.

Stage key is `<cc>/<rel>`, one item after a done build. Input hash combines
verified build artifact hash, fixture and mapping hashes, previous report hash
or empty string, frozen date, and mode list in fixed order. Recipe hash covers
scorer version `agent-eval/1`, legacy matcher version, product matcher version,
metric definitions, rejection IDs/message, cache schema/depths, quantile rule,
and floors. It reads but never changes the dump or singleton release row.
Artifact files: `report.json`, `cases.jsonl`, `arms.jsonl`, `queries.f32`,
`query-index.json`, `MANIFEST.sha256`. `Rows` is a no-op; `Resolve` exposes the
report status and regenerates any evaluation gaps. No production queries enter
these artifacts.

## Fixtures and identity adjudication

Commit the authored banhmi `deploy/eval/golden.json` data unchanged to
`apps/corpus/internal/evaluation/testdata/vn/legacy.json`. Retain all 92 IDs,
questions, expectations, alternate numbers, notes and flags. The fixture is
public authored evaluation data, not captured user questions. Copy no matcher
implementation. `fixtures.json` records its source path, source commit if
available, byte hash, authoring origin and case count. `old-product.json`
references the legacy fixture hash and the three rejection IDs, rather than
duplicating questions or rewriting expectations.

Legacy case format is the existing JSON array:
`{id,question, expected_citations:[{doc_number,article?,clause?,point?,relation_ok?, alt_doc_numbers?}],expect_abstain?,expect_fail?,notes?}`.
Missing booleans mean false. `expect_fail` never removes a case from scoring.
Validate IDs, explicit expectations and contradictory abstention labels before
any model call.

`identity-map.json` is
`{version:1,legacy_sha256,identity_inventory_hash, entries:[...]}`. Each entry
contains `case_id`, zero-based `citation_index`, `state` (`resolved` or
`absent`), `expected_number`, `reason`, `reviewed_at`,
`evidence:[{source_url,fetch_sha256,node_id,quoted_span}]`, and `alternatives`.
Each alternative is `{country,document_key,anchor:null|string,kind}` where kind
is `document`, `provision`, or `relation`; a relation alternative additionally
names `source_key,target_key,relation_kind,evidence_sha256`. Null anchor is
required for document and document-level relation alternatives; provision
alternatives require an exact anchor. Use official legal text and stored source
evidence, not retrieval rank, to adjudicate alternatives.

There is one mapping entry per original expectation, including absent documents.
Resolved entries need at least one unique adjudicated identity; absent entries
have no alternatives and are always misses. An absent entry names every
original/alternate number searched and why no corpus identity is available.
Ambiguous mappings or missing entries block scoring with `mapping_incomplete`;
they are not absences to invent. If an absent document appears in a newer
inventory, require re-adjudication, never auto-credit it. The primary reviews
identities against official evidence before accepting the map. Do not invent an
absent document key or a provision anchor from a label. Bind the map to the
pre-index [identity inventory](index.md#build-stage), so it can be frozen before
chunk/embed and does not depend on its own build hash. If any cited source
identity or expectation changes, require re-adjudication.

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
- Current-law precision: current primary hits / all returned primary hits in
  limit-8 responses, micro-averaged. Count every hit, including hits on negative
  cases and abstaining responses; unknown validity is not current. Report
  unknown and contradictory status counts and non-current group precision
  separately. Legacy only trims a trailing non-current run, unless all hits are
  non-current, in which case all count. Zero denominator is null/N/A, never 1.
  Independently adjudicated legal validity is not claimed.
- Abstention accuracy: correct abstain flag / all non-rejection cases. Treat
  abstention as the positive class: TP negative case abstained; FN negative
  answered; FP positive case abstained; TN positive answered. Also report
  negative recall TP/(TP+FN) and positive answer rate TN/(TN+FP). No hits means
  abstain; an infrastructure error means evaluation incomplete, not abstention.
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
unavailable required metric cannot pass. Future sealed gates are `not_run`;
taxonomy and filtered cases are `out_of_scope`, never passing. `quality_pass`
means both hybrid modes meet all old-product floors and all three rejection
checks pass; legacy has no gate. `release_eligible` is always false in this
baseline, including when `quality_pass` is true. Measured failures leave the
stage done and the run evaluated, with `quality_pass:false` and
`release_eligible:false`. Invalid fixtures, mappings, incomplete case execution
or infrastructure failure fail the stage. CLI and status must distinguish
completed evaluation from passed quality gates.

## Provisional calibration

Build calls the same engine on old-product exact hybrid limit-8 responses,
before finalizing the release row. No lexical, chunk or model experiment runs.
Cache first-group best-member cosines before applying a threshold; empty or
rejected cases remain fixed abstentions. Candidate thresholds are midpoints
between consecutive distinct observed cosines, plus always-answer/always-abstain
sentinels defined in budgets. Choose the threshold that abstains on every old
negative while maximizing positive answer rate; ties choose the lowest
threshold. If none answer any positive, keep the always-abstain threshold and
report that failure rather than weaken the condition.

Calibration calls `Prepare` and `Collect` on the draft, then `retrieval.Rank`,
which returns `FirstBestCosine` without a threshold; it never calls `Search`.
Hash only the pre-calibration retrieval policy in `calibration.json`, never the
finalized recipe that will contain that file's hash. Use finite float64 cosine
values clamped to [-1,1]; if a rounded midpoint equals the lower score, use the
upper score, since equality passes. Exclude the three rejection checks from
fitting.

Record threshold, exact float values, fixture/mapping/recipe hashes, date and
all candidate TP/FP/TN/FN counts in `calibration.json`. Hash that file into
`retrieval_recipe.abstention.calibration_sha256`. The chosen numeric threshold
is computed from real scores, not asserted by this design or fixed at the
historical Qwen seed. Build adopts the first published artifact on retries. Eval
never re-fits the threshold; ANN uses the same frozen value.

This is the primary's provisional exception to development-only calibration.
Label it `provisional-old-product` in every report until development cases
exist. Fitting on the old cohort biases its abstention result; it establishes
neither independent validation nor a model improvement. Later calibration
changes the retrieval recipe and requires a new build.

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
`cohorts:{legacy,old_product}`, `floors`, `rejections`, `coverage`, `ann_loss`,
`timings_ms`, `evidence_characters`, `cache_manifest`, `previous_diff`,
`not_run`, `limitations`. Each metric stores counts as well as the value.
`coverage` lists each missing original/alternate document number and affected
case/citation, including the five known omissions in the ingest research. Never
remove them because the source sweep did not find them.

Only `status:"complete"` reports enter a successful stage artifact. Each cohort
has `case_count`, `positive_count`, `negative_count`, `rejection_count`,
`expected_citations` and `metrics[]`. A metric record is
`{mode,arms,limit,name,numerator,denominator,value}`; value is null for a zero
denominator. For macro recall and MRR, numerator is the sum of case fractions or
reciprocal ranks; otherwise it is an integer count. Confusion counts are
`{tp,fp,tn,fn}` alongside abstention metrics. Each floor record adds `floor` and
`status` from the vocabulary above. Size summaries hold `{n,p50,p95}`; timing
summaries hold `{n,p50,p95,p99}` in milliseconds, using the same quantile rule.

`cases.jsonl` records case ID, expected/matched counts, ranks, mapped
identities, returned identities, arm/fusion/group/cap/abstention losses,
decision reason, validity gaps and character counts, without query text.
`previous_diff` is null with a reason when no previous report exists; otherwise
compare per-case gains/losses and reject incompatible scorer/fixture versions.
Corpus changes are labeled coverage changes, not an isolated model effect.
Report low sample size and old-cohort reuse; no superiority interval or claim is
needed here.
