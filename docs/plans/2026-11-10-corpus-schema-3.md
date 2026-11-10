# Corpus Schema 3 Implementation Plan

> Route: each task goes to one Codex `implementer` (`gpt-6.1-terra` high), resumed by session id; the primary writes SQL and runs `sqlc generate`, runs integration tests and proofs, and owns Git; Codex `reviewer` (`gpt-6.1-sol` high) reviews the whole branch after Task 8. Task 9 is the primary's live run. This plan replaces Task 11 of `2026-11-09-corpus-scope.md`.

**Goal:** A release built from a scope run loads only present documents with their class and source, corrects wrong or missing source validity with text rules and a reviewed manual list, and reports what it corrected, so agents stop receiving replaced law as current law.

**Spec:** `docs/design/scope-adoption.md` (schema version 3, inventory versions, presence, provenance snapshot, loading, readers), `docs/design/validity-status.md` (layers, text evidence, rules A to C, manual corrections, review queue, decision history), `docs/design/corpus-schema.md`, `docs/design/index.md`, `docs/design/evaluation.md`, `docs/design/retrieval-recipe.md`. The spec wins over this plan.

## Global Constraints

- Legacy runs (selection version 0) keep inventory version 1 bytes and hashes; golden fixtures prove it. A frozen run whose build recipe names schema version 2 is refused, never rebuilt.
- Every automatic validity rule fails closed: an unparsed or ambiguous shape changes nothing and becomes a review entry. No network or model at build time.
- Tests first; failure observed where the worker can run it. Integration tests use 2099 releases and `internal/testpg`. Non-test Go files at most 400 lines; design files at most 300.
- Never hand-edit `apps/corpus/internal/store`; new queries are written by the primary.

### Task 1: Schema version 3 and readers

**Files:** `internal/corpusdb/schema.sql`, `schema.go` and tests; `internal/retrieval/engine.go`, `release.go` and tests; `apps/corpus/internal/stage/build/` only where rows must supply the new NOT NULL columns (class `primary`, source from the key, `validity_conflict` false) so the suite stays green.

- [x] Tests first: `TestSchema_Version3Constraints` (class, source, status_basis values, validity_conflict, validity_decision keys, ordinal range, quote length, layer-required fields, release `schema_version = 3`), `TestOpen_RejectsSchemaVersion2` (error names both versions), `TestBuild_FrozenSchema2RecipeRefused`.
- [x] Implement; commit `feat: corpus schema version 3 with class, source and validity decisions`.

### Task 2: Inventory version 2, provenance snapshot and planning reconciliation

**Files:** `apps/corpus/sql/build.sql` (primary: one consistent read of items, gaps with structured fields, final snapshot bytes and hash, file projection), regenerated store; `apps/corpus/internal/stage/build/{inventory,build,record}.go` and tests.

- [x] Tests first: `TestInventory_V1GoldenBytesUnchanged`, `TestInventory_V2GoldenBytesAndHashes`, `TestInventory_RepublishedFinalChangesBuildInput`, `TestBuild_RequiresFinalMembership`, `TestBuild_MembershipUnreconciled`, `TestProvenance_SnapshotStableAndBound`.
- [x] Implement; commit `feat(corpus): inventory version 2 bound to final membership, with a provenance snapshot`.

### Task 3: Presence, source-aware reader and loading

**Files:** `apps/corpus/internal/stage/build/{documents,load,relations}.go` and new files by concern (`presence.go`, `reader.go`); `apps/corpus/internal/cli/inventory.go`; `apps/corpus/internal/evaluation/mapping.go`; tests.

- [x] Tests first: `TestLoad_ClassAndSourceFromSnapshot`, `TestLoad_EachExcludedDispositionNotLoaded`, `TestLoad_IgnoresArtifactsOfAbsentKeys`, `TestLoad_SecondaryMetadataOnly`, `TestRelations_ReplacedSecondaryNeverResolvesNumber`, `TestIdentity_ReplacedSecondaryNeedsReadjudication`, `TestLoad_LegacyRunClassPrimary`.
- [x] Implement; commit `feat(corpus): load present documents with class, source and secondary metadata`.

### Task 4: Effect article parser

**Files:** new `apps/corpus/internal/validity/` (pure package: article selection, sentence and list splitting, commencement and replacement forms, usable-article rule) with tests and fixtures, including the complete Điều 37 of `52/2024/NĐ-CP` and Điều 34 of `134/2025/QH15` recorded from the corpus by the primary.

- [x] Tests first: `TestParse_Dieu37TwoWholeOnePartial`, `TestParse_SpelledAndNumericDates`, `TestParse_QuotedTextIgnored`, `TestParse_ClauseScopedDateMakesArticleUnusable`, `TestParse_TruExceptionUnusable`, `TestParse_UnparsedEffectSentenceUnresolved`, `TestParse_ListContinuationInheritsSubject`, `TestParse_PartialItemPrefixes`, `TestParse_SubjectTypeMustMatchDocument`.
- [x] Implement; commit `feat(corpus): parser for effect articles`.

### Task 5: Validity rules, manual corrections and review queue

**Files:** `apps/corpus/internal/source/` (corrections file `sources/vn-corrections.yaml` in the frozen source list, validation), `sources/vn.yaml` (`TNHL` remap with a recorded-source fixture), `apps/corpus/internal/stage/build/` new files (`validity_pass.go`, `validity_events.go`, `corrections.go`, `review.go`), recipe fields (`validity_rules_version` `validity/1`, `corrections_sha256`), tests.

- [x] Tests first: one per row of the validity note's rules, plus `TestValidity_ChainBothOrders`, `TestValidity_ExpiredActorStillEmitsEvent`, `TestValidity_FutureReplacementKeepsPredecessorCurrent`, `TestValidity_DateConflictMakesNoEvents`, `TestValidity_IssuerMustBeVerified`, `TestValidity_AdverseStatusNameNotCandidate`, `TestValidity_ClearedIntervalBlocksRuleA`, `TestRuleC_OnlyInForceTargetsWithoutPartialMention`, `TestCorrections_EveryRejection`, `TestCorrections_RedundantAndConflictReports`, `TestReview_ListsEverySkippedItem`, `TestDecisions_HistoryOrderAndCap`.
- [x] Implement; commit `feat(corpus): validity corrections from text rules and a reviewed manual list`.

### Task 6: Retrieval

**Files:** `internal/retrieval/` (hit `class`, `source`, decision history outside the relation cap, eligibility "not `validity_conflict`", validity policy version), tests.

- [x] Tests first: `TestCollect_ValidityConflictNotInPrimaryGroup`, `TestShape_ConflictShownAsUnresolvedNonCurrent`, `TestHit_CarriesClassSourceAndDecisions`, `TestValidity_BothModesRoundTripDecisions`, `TestRecipe_ValidityPolicyVersionBumped`.
- [x] Implement; commit `feat(retrieval): class, source and validity decisions on hits`.

### Task 7: Evaluation

**Files:** `apps/corpus/internal/evaluation/`, `apps/corpus/internal/stage/eval/`, `apps/corpus/internal/evaluation/testdata/vn/` (adjudicated validity cases for 52/2024, 101/2012, 80/2016, 134/2025, 16/2019), tests.

- [x] Tests first: `TestReport_CountsByClassSourceAndBasis`, `TestValidityCases_ScoredOverPrimaryGroup`, `TestMetrics_StatusConsistencyNameAndDenominator`.
- [x] Implement; commit `feat(corpus): evaluation counts by class and adjudicated validity cases`.

### Task 8: Proof and review

- [x] Extend the scope proof through `build` and `eval` (`TestProof_ScopeRunBuildsAndEvaluates`: replaced secondary not loaded, class and source set, one rule A correction, one rule B event, `validity-review.json` present, dump and restore keep decisions), under 60 seconds.
- [ ] Whole-branch `gpt-6.1-sol` review; fix findings; `make pre-push`, `make sqlc-check`; push; draft pull request; merge on green CI.

### Task 9: Live run and coverage (primary)

- [ ] New run with the reviewed source list and an initial `sources/vn-corrections.yaml`; record expansion size, requests, time and gaps by kind; confirm the four documents the baseline lacked are present.
- [ ] Review every rule B expiry, rule C conflict and the whole review queue; add manual entries with evidence; rebuild; re-adjudicate the identity map.
- [ ] Repeat the real-output review of the 30 affected cases; write `docs/research/vn-scope-run-2026-11.md`; mark this plan and the corpus scope plan landed.
