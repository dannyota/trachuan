# Corpus Scope Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Route in this repository: Task 1 is a primary spike with recorded fixtures; Tasks 2 to 12 each go to one Codex `implementer` (`gpt-6.1-terra` high) with standing answers, resumed by session id; Task 13 is the primary's live run; the Claude `reviewer` reviews the whole branch after Task 12. Depends on Plan 2 (index and evaluation) being merged first; this branch is rebased onto it.

**Goal:** A new Vietnam run reaches the documents agents need: local scope selection over the full central VBPL listing with an audited loss comparison, named numbers and one-hop relation targets from VBPL, retry of source gaps, and central documents VBPL lacks from `vanban.chinhphu.vn` (starting with AI), all with explicit provenance.

**Architecture:** `discover` lists the full central VBPL catalog once per run (page size 1000, two stable passes) and selects scope locally by agency ({62, 908}) and approved title phrases, plus named number lookups; it replaces the State Bank sweep query and the server-side keyword searches (owner decision 2026-11-09); a new `expand` stage between `discover` and `fetch` adds one hop of explicit outward VBPL relation targets as class `relation_context`. A new `apps/corpus/internal/chinhphu` client and a secondary discovery path admit central documents only after an exact VBPL number lookup finds none; they are fetched as metadata plus signed PDFs (`no_tree` until the text extraction plan). Number normalization folds Cyrillic look-alikes in one shared function.

**Tech Stack:** Go 1.27.3, Temporal Go SDK v1.50.0, pgx/v5, sqlc 1.31.1, golang.org/x/net/html for page parsing, recorded fixtures.

**Spec:** `docs/design/sources.md`; `docs/design/source-execution.md` (lookup results, membership, fetch-close finalization, source pause, retries); `docs/design/pipeline.md` (run lifecycle, items, VBPL ingest rules, expansion and retries); `docs/design/data.md`; `docs/design/budgets.md` "Pipeline"; ADR 0012; `docs/research/vn-sources-2026-11.md`. The spec wins over this plan.

## Global Constraints

- VBPL is the only primary source; `chinhphu` admits a central document only after a complete exact VBPL number lookup returns no eligible or unresolved match (an equal number with a different verified issue date is a different document); a later VBPL match replaces it only under the hand-over decision table in sources.md (matching number, issuer and issue date), recording `source_replaced`; conflicting or missing evidence keeps both with `source_identity_ambiguous`.
- Expansion is one hop, explicit outward VBPL ids only, never recursive, never inbound; context never seeds expansion; the expected set closes after `expand`.
- Named lookups: `optionDoc: docNum`, `matchMode: exact`, `agencyLevel: TRUNG_UONG`, all pages, local equality on normalized numbers plus VBPL-only named eligibility in data.md (date qualifier `number@YYYY-MM-DD`, required for yearless numbers; different dates neither seed nor block); zero is `named_document_missing`, several or unresolved is `named_document_ambiguous`; never pick one.
- Local selection: NFC, lowercase, whitespace-collapsed contiguous phrase over the title, no diacritic folding; State Bank membership by `agencyIds` ∩ {62, 908}; every match is a typed support tuple `(rule_kind, rule_value, evidence_reference)` per data.md; a selection loss is `scope_removed`, only catalog absence in two complete stable listings is `withdrawn`. The owner-approved comparison is recorded in `docs/research/vn-scope-comparison-2026-11.md` (artifact `comparison-v2.json`, SHA-256 `a75a1ff9…`).
- Number normalization: NFC, uppercase, `Đ` to `D`, Cyrillic look-alikes folded to Latin, whitespace around `/` and `-` removed; one function shared by discovery, build and retrieval.
- `chinhphu` safety: HTTPS only; pages on `vanban.chinhphu.vn`, files on `datafiles.chinhphu.vn`; no credentials, nonstandard ports or nonpublic addresses; redirects revalidated; cookies and postback tokens only in memory and never in artifacts or fixtures; named user agent; throttling and limits from budgets; a challenge or 403 pauses the source.
- Recipes change for discover, expand, fetch and build; a frozen run never changes; the changes take effect in a new run.
- chinhphu search requests the 500 cap; complete only when the reported total is below the cap, every page was read and distinct docids equal it; the empty marker is a complete zero; a missing detail page (HTTP 200, `Không tìm thấy văn bản này`) is not found.
- Stages that call a narrow service (the chinhphu source at 1 in flight) use Plan 2's per-stage in-flight limit, not the worker-wide batch concurrency; after pulling new code, `corpus migrate` runs before the worker restarts (runbook).
- Tests never call the network; fixtures are recorded and redacted by the primary. Files at most 400 lines; doc comments on exported symbols; errors wrapped with `%w`.

## Review Focus

- A gateway or source failure read as "no document", so a missing lookup admits a duplicate secondary document or drops context (Task 5 `TestNamed_LookupFailureIsGapNotAbsence`, Task 9 `TestAdmit_LookupFailureDefers`).
- A number collision between distinct documents merged into one identity (Task 3 `TestNormalize_CyrillicLookalikes`, Task 9 `TestAdmit_CollisionIsIdentityAmbiguous`).
- Expansion that recurses or grows after the set closes (Task 6 `TestExpand_ContextNeverSeeds`, `TestFinalize_NewTargetIsScopeChanged`).
- Session tokens or cookies written into artifacts or fixtures (Task 8 `TestClient_NoTokenInArtifacts`).
- Retry on start that resets fetch but leaves stale downstream rows (Task 7 `TestStart_RetryRebuildsDependents`).

---

### Task 1: Spike, fixtures and the keyword loss comparison (primary)

**Files:** Create `apps/corpus/internal/vbpl/testdata/` additions (number lookups: unique, colliding, empty; exact title; relation direction; non-JSON 200), `apps/corpus/internal/chinhphu/testdata/` (search first page, second page, empty, multiple, detail, attachment headers), `docs/research/vn-scope-comparison-2026-11.md`.

- [x] Record redacted fixtures with `record.sh`-style scripts and write the observed contracts into the research note: VBPL number lookups (unique, colliding, empty, paged) with totals for completeness; `vanban.chinhphu.vn` search first and later pages, empty and multiple results, completeness evidence (totals and last page), hidden postback token rotation, redirects, a challenge or 403 response, detail field selectors with central issuer evidence, and the first bytes of a real signed PDF for signature checks. Reconcile the AI document list (all result rows, with docids) without inventing entries.
- [x] Define the offline test seams later tasks use: injectable clock, HTTP transport, DNS resolver and reduced limits for both clients; record them in the research note.
- [x] Run the keyword comparison (then, by owner decision, the full central listing with local selection, artifact v2): complete stable `all_words` and `exact` listings for every proposed term plus the full old and new seed sets; save the immutable comparison artifact (hash recorded in the note); account for every old-only id; owner approves exclusions.
- [x] Measure the expansion set for the proposed seed set (old seeds plus named numbers and the exact-mode keyword seeds), not only the old run: unique targets, requests, bytes, elapsed time, against budgets.
- [ ] Commit `test(corpus): VBPL and chinhphu fixtures and the scope comparison`.

### Task 2: Shared execution contracts

**Files:** Create migration `apps/corpus/migrations/00004_membership.sql`, `apps/corpus/sql/membership.sql`, `apps/corpus/internal/stage/membership/` (readers and writers), modify `apps/corpus/internal/stage/types.go` (structured gap causes), `stage/batch.go` and `workflow/build.go` only where `source-execution.md` "Fetch-close finalization" needs a fenced close step; regenerate the store.

**Interfaces:** Produces the membership storage (class, supporting seeds, evidence hashes), source-neutral document and file row writers, structured retry causes on gaps, the evidence-bearing `LookupResult`/`SearchResult` types (`Query`, `Pages`, `Total`, `Complete`, `ResponseHashes`, items), the fetch-close finalization hook, and the source pause state (`source_paused` storage and checks); exact names come from `source-execution.md`.

- [x] Tests first: `TestMembership_TypedSupportTuples` (agency, term, named, secondary_probe, relation; snapshot and hash include them), `TestMembership_LeavesOnlyWithoutSupport`, `TestMembership_ClassPrecedenceAndSeeds`, `TestLookupResult_EmptyIsAbsenceOnlyWhenComplete`, `TestFinalize_FencedAndIdempotent`, `TestSourcePause_PersistsAcrossWorkers`, `TestGapCause_RoundTrip`.
- [x] Implement; `TestReplay_Histories` must still pass; commit `feat(corpus): shared membership, lookup evidence and source pause contracts`.

### Task 3: Source list and number normalization

**Files:** Modify `apps/corpus/internal/source/list.go` and test, create `apps/corpus/internal/source/select.go` and test, modify `sources/vn.yaml`, `internal/retrieval/numbers.go` and test (or move the function to a shared root package named in the brief).

**Interfaces:** Produces `source.List.VBPL.NamedNumbers` (entries `number` or `number@YYYY-MM-DD`, parsed into number and optional date; yearless numbers without a date are rejected), `VBPL.SelectionVersion`, `Chinhphu{Keywords, NamedNumbers}`; `SweepAgencies` becomes the local selection agencies and `Keywords` the local terms, while `KeywordAgencies` stays parseable for older snapshots and leaves `sources/vn.yaml`; a selection function over listing items returning typed supports; query identity hashing for the central listing (source, type, level, no agencies, no keyword, page size) and lookups (option, mode, number); `retrieval.NormalizeNumber` folding Cyrillic look-alikes.

- [x] Tests first: `TestParse_NamedNumbersAndChinhphu`, `TestParse_YearlessNamedNeedsDate`, `TestSelect_AgencyAndTermSupports`, `TestSelect_ContiguousNFCPhraseNoFolding` (52/2024 title "không dung tiền mặt" does not match), `TestQueryIdentity_CentralListingAndLookup`, `TestNormalize_CyrillicLookalikes` (`280/2026/NĐ-CР` with Cyrillic `Р` equals `280/2026/NĐ-CP`), `TestNormalize_ExistingCasesUnchanged`.
- [x] Implement; `sources/vn.yaml` gets the 32 approved terms, 12 named numbers and the chinhphu lists from sources.md.
- [x] Commit `feat(corpus): source list for named numbers and the chinhphu source`.

### Task 4: VBPL client lookups

**Files:** Modify `apps/corpus/internal/vbpl/client.go`, `types.go` and tests.

**Interfaces:** Produces `(*Client) LookupNumber(ctx, number string) (LookupResult, error)` (all pages, evidence-bearing per source-execution.md), `ListQuery.Size`, `ListQuery.Option`, `ListQuery.Mode` and an empty agency list for the central listing; a non-JSON 200 body is a retryable error (synthetic test body, labeled synthetic; no live source produced one).

- [x] Tests first against Task 1 fixtures: `TestLookupNumber_UniqueCollisionEmpty`, `TestLookupNumber_PagedTotalsAndEmptyLastPage`, `TestList_CentralNoAgencyNoKeyword`, `TestAPI_NonJSON200Retryable`.
- [x] Implement; commit `feat(corpus): exact number and title lookups in the VBPL client`.

### Task 5: Discover the central listing, local selection and named numbers

**Files:** Modify `apps/corpus/internal/stage/discover/` and tests, `apps/corpus/sql/discover_expected.sql` as needed (regenerate the store).

**Interfaces:** Discover plans one central listing item (two stable passes at page size 1000, at most three) and one item per named number; local selection writes typed supports; classes `primary` in the expected set; gaps `named_document_missing`, `named_document_ambiguous`; `scope_removed` for selection losses, `withdrawn` only for catalog absence.

- [ ] Tests first: `TestDiscover_CentralListingStablePasses`, `TestDiscover_PartialListingProvesNothing`, `TestDiscover_LocalSelectionSupports`, `TestNamed_UniqueJoinsSeeds`, `TestNamed_DifferentDateNeitherSeedsNorBlocks`, `TestNamed_ZeroAndManyAreGaps`, `TestNamed_LookupFailureIsGapNotAbsence`, `TestScopeRemoved_SelectionLossVsCatalogAbsence`.
- [ ] Implement; commit `feat(corpus): discover the central listing with local scope selection`.

### Task 6: Expand stage

**Files:** Create `apps/corpus/internal/stage/expand/` with tests; modify expected-set loading (`discover/expected.go`); drift enforcement in the fetch-close finalization hook from Task 2 (qualified fetch targets compared with the frozen active membership); membership storage comes from Task 2.

**Interfaces:** `expand.New(client *vbpl.Client) *expand.Stage` (name `expand`), registered between `discover` and `fetch` in Task 12; `relation_context` membership with supporting seeds, codes and evidence hashes.

- [x] Tests first: `TestExpand_QualifyingKindsOutwardOnly`, `TestExpand_ContextNeverSeeds`, `TestExpand_ExcludesRenditionsAndSelfLinks`, `TestExpand_SourceFailureKeepsKnownTargets`, `TestExpand_RemovalNeedsCompleteCurrentSupport`, `TestExpand_DepartedSeedDropsSupport`, `TestExpand_FailedObservationRetainsTargets`, `TestFinalize_NewTargetIsScopeChanged`, `TestFinalize_RetainedRemovalKeyIsDrift`, `TestDrift_GapCannotBeAccepted`.
- [x] Implement per sources.md "One-hop relation context"; commit `feat(corpus): expand stage for one-hop relation context`.

### Task 7: Retry source gaps on start

**Files:** Modify `apps/corpus/internal/run/start.go` and tests, SQL in `apps/corpus/sql/` (regenerate).

- [x] Tests first: `TestStart_ResetsOnlySourceGapItems`, `TestStart_DeletesThoseGapRows`, `TestStart_RetryRebuildsDependents`, `TestStart_FrozenRecipesUnchanged`.
- [x] Implement per pipeline.md; commit `feat(corpus): retry source gaps when a run starts again`.

### Task 7a: Membership predecessor and removal lifetime

**Files:** Migration 00004 (three nullable `run_scope` predecessor columns, primary), SQL (primary); modify `stage/discover` (predecessor selection and record, legacy tuple exception, removal carry), `stage/expand` and `stage/fetch` finalizers (read and verify the recorded predecessor), `run/retry.go` (clear it with seeds); tests.

**Interfaces:** one predecessor per run per `docs/design/membership.md`: the latest earlier run with a frozen `final` phase, else the latest selection-version-0 run through the legacy reader, else none; recorded at discovery close with run id, phase and hash; a removed-only member is dropped only after its removal gap was accepted.

- [x] Tests first: `TestPredecessor_RecordedAtDiscoveryClose`, `TestPredecessor_SkipsRunsWithoutFinal`, `TestPredecessor_HashMismatchFails`, `TestPredecessor_ExpandAndFetchUseRecord`, `TestRemoval_UnacceptedCarries`, `TestRemoval_AcceptedIsDropped`, `TestRetry_ClearsPredecessorWithSeeds`.
- [x] Implement per membership.md; commit `feat(corpus): frozen membership predecessor and accepted removal lifetime`.

### Task 8: chinhphu client

**Files:** Create `apps/corpus/internal/chinhphu/{client,search,detail,download}.go` with tests.

**Interfaces:** `chinhphu.New(opts ...Option) *Client`; `(*Client) Search(ctx, keyword string) (SearchResult, error)` (all pages, evidence-bearing); `(*Client) LookupNumber(ctx, number string) (SearchResult, error)`; `(*Client) Detail(ctx, docid int) (Detail, error)`; `(*Client) Download(ctx, url string, dst io.Writer, max int64) (FileInfo, error)`; `Candidate{DocID int; Number, Title, Issuer, IssueDate string}`.

- [x] Tests first on Task 1 fixtures: `TestSearch_PaginatesAndDedupes`, `TestSearch_EmptyAndMultiple`, `TestDetail_ParsesCentralIssuerAndAttachments`, `TestDownload_HostAllowlistAndPDFSignature`, `TestSearch_CapEqualIsIncomplete`, `TestSearch_EmptyMarkerIsCompleteZero`, `TestDetail_MissingIsNotFound`, `TestClient_NoTokenInArtifacts`, `TestClient_ChallengePausesSource` (through the shared pause state, so concurrent batches and a restarted worker stop too).
- [x] Implement; commit `feat(corpus): vanban.chinhphu.vn client`.

### Task 9: Secondary discovery

**Files:** `sources/vn.yaml` and `apps/corpus/internal/source/` (`chinhphu.issuer_policy_version`, `central_issuer_forms`, validation); `apps/corpus/internal/chinhphu/detail.go` (lossless repeated fields); `apps/corpus/internal/stage/discover/` (compound `chinhphu:keyword:` and `chinhphu:named:` items, issuer classification, candidate VBPL probes, `secondary_probe` tuples `<item key>|<docid>`, run-wide candidate ceiling at close); `apps/corpus/internal/stage/finalize.go` (`secondary_candidate_ceiling` in the non-acceptable list); tests. Hand-over at fetch close moves to Task 10.

- [x] Tests first: `TestAdmit_ZeroVBPLMatchAdmits`, `TestAdmit_DifferentDateNeitherSeedsNorBlocks`, `TestAdmit_UniqueVBPLMatchSeedsPrimary`, `TestAdmit_LookupFailureDefers`, `TestAdmit_CollisionIsIdentityAmbiguous`, `TestAdmit_ProvincialOutOfScope`, `TestIssuer_RepeatedConflictingFieldsUnknown`, `TestNamed_SubstringHitRejected`, `TestCeiling_RunWideOverflowFailsDiscover`, `TestSupport_PerQueryTuplesSurviveSupersession`.
- [x] Implement per secondary-discovery.md and sources.md; commit `feat(corpus): secondary discovery from vanban.chinhphu.vn`.

### Task 10: Secondary fetch and provenance

**Files:** Modify `apps/corpus/internal/stage/fetch/` (secondary bundles through the shared source-neutral writers, hand-over in the fetch finalizer), `extract` (`no_tree`), `relate` (empty evidenced set); tests. Build loading belongs to Task 11.

- [ ] Tests first: `TestFetch_ChinhphuBundleHashesFiles`, `TestExtract_ChinhphuNoTree`, `TestRelate_ChinhphuEmptySet`, `TestHandOver_SourceReplaced` (fetch close applies the hand-over table and records `source_replaced`), `TestHandOver_AmbiguousKeepsBoth`.
- [ ] Implement; commit `feat(corpus): fetch secondary documents`.

### Task 11: Build and evaluation integration (Plan 2 code)

**Files:** Modify `apps/corpus/internal/stage/build/` (inventory version 2 with expand evidence, membership and typed support tuples in the identity hash, class and provenance loading, exclusions, dump and restore validation of schema version 2), `internal/corpusdb/schema.sql` and `schema.go` (`SchemaVersion = 2`, `document.class`), `internal/retrieval` release reading (accept schema version 2, refuse 1 with a clear error), `internal/retrieval` only if class reaches responses, `apps/corpus/internal/evaluation/mapping.go` (reject identity maps bound to inventory version 1).

**Design first:** Plan 2's review (`906d03c`) took schema version 2 for `document.status_basis` and moved the identity map hash into the build input, so the scope adoption text was dropped from `corpus-schema.md`, `index.md` and `evaluation.md` during the rebase (dropped hunks saved in the primary's scratchpad `rebase/dropped.md`). Before this task, a designer pass re-adds scope adoption against those decisions (scope schema version 3, `document.class`, inventory version 2 with membership, map hash in build input) and a design review approves it; then revise this task's file list and test names.

- [ ] Tests first: `TestInventory_V2IncludesExpandEvidence`, `TestInventory_V1MapRejected`, `TestLoad_ClassColumnAndProvenance`, `TestSchema_Version2ClassCheck`, `TestBuild_SecondaryMetadataOnly`.
- [ ] Implement per index.md, corpus-schema.md and evaluation.md as revised; commit `feat(corpus): build and evaluation for membership classes and secondary provenance`.

### Task 12: Wiring, config and proofs

**Files:** Modify `workflow/registry.go` (stage order `discover, expand, fetch, ...`), config (`CORPUS_CHINHPHU_*` settings), worker, CLI status and a new `corpus source-resume <cc> <rel> <source>` command that clears a source pause, runbook; proof `apps/corpus/test/proof/scope_test.go`.

- [ ] Tests first: `TestRegistry_ExpandAfterDiscover`, `TestSourceResume_ClearsPauseExplicitly`, `TestSourceResume_PausedSourceStaysPausedAfterRestart`, `TestReplay_Histories` still passes, `TestProof_ScopeRunWithExpansionAndSecondary` (fake VBPL and chinhphu servers, under 60 seconds).
- [ ] Implement; commit `feat(corpus): wire expand and the secondary source`.

### Task 13: Live run and coverage (primary)

- [ ] Start a new run with the reviewed source list; record expansion size, requests, time, gaps by kind; confirm the four VBPL golden documents and the AI documents are present.
- [ ] Re-adjudicate the vn identity map for the new inventory; write `docs/research/vn-scope-run-2026-11.md`; mark this plan landed.
