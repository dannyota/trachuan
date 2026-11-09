# Corpus Scope Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Route in this repository: Task 1 is a primary spike with recorded fixtures; Tasks 2 to 12 each go to one Codex `implementer` (`gpt-6.1-terra` high) with standing answers, resumed by session id; Task 11 is the primary's live run; the Claude `reviewer` reviews the whole branch after Task 12. Depends on Plan 2 (index and evaluation) being merged first; this branch is rebased onto it.

**Goal:** A new Vietnam run reaches the documents agents need: named numbers and one-hop relation targets from VBPL, exact-mode keyword searches with an audited loss comparison, retry of source gaps, and central documents VBPL lacks from `vanban.chinhphu.vn` (starting with AI), all with explicit provenance.

**Architecture:** Discovery stays in `discover`; a new `expand` stage between `discover` and `fetch` adds one hop of explicit outward VBPL relation targets as class `relation_context`. A new `apps/corpus/internal/chinhphu` client and a secondary discovery path admit central documents only after an exact VBPL number lookup finds none; they are fetched as metadata plus signed PDFs (`no_tree` until the text extraction plan). Number normalization folds Cyrillic look-alikes in one shared function.

**Tech Stack:** Go 1.27.3, Temporal Go SDK v1.50.0, pgx/v5, sqlc 1.31.1, golang.org/x/net/html for page parsing, recorded fixtures.

**Spec:** `docs/design/sources.md`; `docs/design/source-execution.md` (lookup results, membership, fetch-close finalization, source pause, retries); `docs/design/pipeline.md` (run lifecycle, items, VBPL ingest rules, expansion and retries); `docs/design/data.md`; `docs/design/budgets.md` "Pipeline"; ADR 0012; `docs/research/vn-sources-2026-11.md`. The spec wins over this plan.

## Global Constraints

- VBPL is the only primary source; `chinhphu` admits a central document only after a complete exact VBPL number lookup returns zero matches and no active VBPL member holds its normalized number; VBPL always wins and a hand-over records `source_replaced`.
- Expansion is one hop, explicit outward VBPL ids only, never recursive, never inbound; context never seeds expansion; the expected set closes after `expand`.
- Named lookups: `optionDoc: docNum`, `matchMode: exact`, `agencyLevel: TRUNG_UONG`, all pages, local equality on normalized numbers; zero is `named_document_missing`, several is `named_document_ambiguous`; never pick one.
- Keyword queries use `optionDoc: title`, `matchMode: exact`; a run with the new list is frozen only after the comparison artifact accounts for every old-only id.
- Number normalization: NFC, uppercase, `Đ` to `D`, Cyrillic look-alikes folded to Latin, whitespace around `/` and `-` removed; one function shared by discovery, build and retrieval.
- `chinhphu` safety: HTTPS only; pages on `vanban.chinhphu.vn`, files on `datafiles.chinhphu.vn`; no credentials, nonstandard ports or nonpublic addresses; redirects revalidated; cookies and postback tokens only in memory and never in artifacts or fixtures; named user agent; throttling and limits from budgets; a challenge or 403 pauses the source.
- Recipes change for discover, expand, fetch and build; a frozen run never changes; the changes take effect in a new run.
- Tests never call the network; fixtures are recorded and redacted by the primary. Files at most 400 lines; doc comments on exported symbols; errors wrapped with `%w`.

## Review Focus

- A gateway or source failure read as "no document", so a missing lookup admits a duplicate secondary document or drops context (Task 5 `TestNamed_LookupFailureIsGapNotAbsence`, Task 9 `TestAdmit_LookupFailureDefers`).
- A number collision between distinct documents merged into one identity (Task 3 `TestNormalize_CyrillicLookalikes`, Task 9 `TestAdmit_CollisionIsIdentityAmbiguous`).
- Expansion that recurses or grows after the set closes (Task 6 `TestExpand_ContextNeverSeeds`, `TestResolve_NewTargetIsScopeChanged`).
- Session tokens or cookies written into artifacts or fixtures (Task 8 `TestClient_NoTokenInArtifacts`).
- Retry on start that resets fetch but leaves stale downstream rows (Task 7 `TestStart_RetryRebuildsDependents`).

---

### Task 1: Spike, fixtures and the keyword loss comparison (primary)

**Files:** Create `apps/corpus/internal/vbpl/testdata/` additions (number lookups: unique, colliding, empty; exact title; relation direction; non-JSON 200), `apps/corpus/internal/chinhphu/testdata/` (search first page, second page, empty, multiple, detail, attachment headers), `docs/research/vn-scope-comparison-2026-11.md`.

- [ ] Record redacted fixtures with `record.sh`-style scripts and write the observed contracts into the research note: VBPL number lookups (unique, colliding, empty, paged) with totals for completeness; `vanban.chinhphu.vn` search first and later pages, empty and multiple results, completeness evidence (totals and last page), hidden postback token rotation, redirects, a challenge or 403 response, detail field selectors with central issuer evidence, and the first bytes of a real signed PDF for signature checks. Reconcile the AI document list (all result rows, with docids) without inventing entries.
- [ ] Define the offline test seams later tasks use: injectable clock, HTTP transport, DNS resolver and reduced limits for both clients; record them in the research note.
- [ ] Run the keyword comparison: complete stable `all_words` and `exact` listings for every proposed term plus the full old and new seed sets; save the immutable comparison artifact (hash recorded in the note); account for every old-only id; owner approves exclusions.
- [ ] Measure the expansion set for the proposed seed set (old seeds plus named numbers and the exact-mode keyword seeds), not only the old run: unique targets, requests, bytes, elapsed time, against budgets.
- [ ] Commit `test(corpus): VBPL and chinhphu fixtures and the scope comparison`.

### Task 2: Shared execution contracts

**Files:** Create migration `apps/corpus/migrations/00004_membership.sql`, `apps/corpus/sql/membership.sql`, `apps/corpus/internal/stage/membership/` (readers and writers), modify `apps/corpus/internal/stage/types.go` (structured gap causes), `stage/batch.go` and `workflow/build.go` only where `source-execution.md` "Fetch-close finalization" needs a fenced close step; regenerate the store.

**Interfaces:** Produces the membership storage (class, supporting seeds, evidence hashes), source-neutral document and file row writers, structured retry causes on gaps, the evidence-bearing `LookupResult`/`SearchResult` types (`Query`, `Pages`, `Total`, `Complete`, `ResponseHashes`, items), the fetch-close finalization hook, and the source pause state (`source_paused` storage and checks); exact names come from `source-execution.md`.

- [ ] Tests first: `TestMembership_ClassPrecedenceAndSeeds`, `TestLookupResult_EmptyIsAbsenceOnlyWhenComplete`, `TestFinalize_FencedAndIdempotent`, `TestSourcePause_PersistsAcrossWorkers`, `TestGapCause_RoundTrip`.
- [ ] Implement; `TestReplay_Histories` must still pass; commit `feat(corpus): shared membership, lookup evidence and source pause contracts`.

### Task 3: Source list and number normalization

**Files:** Modify `apps/corpus/internal/source/list.go` and test, `sources/vn.yaml`, `internal/retrieval/numbers.go` and test (or move the function to a shared root package named in the brief).

**Interfaces:** Produces `source.List.VBPL.NamedNumbers []string`, `VBPL.MatchMode string`, `Chinhphu{Keywords, NamedNumbers []string}`, query identity hashing over source, type, option, mode, level, agencies and keyword or number; `retrieval.NormalizeNumber` folding Cyrillic look-alikes.

- [ ] Tests first: `TestParse_NamedNumbersAndChinhphu`, `TestQueryIdentity_IncludesModeAndLevel`, `TestNormalize_CyrillicLookalikes` (`280/2026/NĐ-CР` with Cyrillic `Р` equals `280/2026/NĐ-CP`), `TestNormalize_ExistingCasesUnchanged`.
- [ ] Implement; `sources/vn.yaml` gets the reviewed lists from sources.md and Task 1.
- [ ] Commit `feat(corpus): source list for named numbers and the chinhphu source`.

### Task 4: VBPL client lookups

**Files:** Modify `apps/corpus/internal/vbpl/client.go`, `types.go` and tests.

**Interfaces:** Produces `(*Client) LookupNumber(ctx, number string) (LookupResult, error)` (all pages, evidence-bearing per source-execution.md) and `ListQuery.Option`, `ListQuery.Mode`; a non-JSON 200 body is a retryable error.

- [ ] Tests first against Task 1 fixtures: `TestLookupNumber_UniqueCollisionEmpty`, `TestList_ExactMode`, `TestAPI_NonJSON200Retryable`.
- [ ] Implement; commit `feat(corpus): exact number and title lookups in the VBPL client`.

### Task 5: Discover named numbers and exact keywords

**Files:** Modify `apps/corpus/internal/stage/discover/` and tests, `apps/corpus/sql/discover_expected.sql` as needed (regenerate the store).

**Interfaces:** Discover plans one item per named number besides query items; classes `primary` in the expected set; gaps `named_document_missing`, `named_document_ambiguous`.

- [ ] Tests first: `TestNamed_UniqueJoinsSeeds`, `TestNamed_ZeroAndManyAreGaps`, `TestNamed_LookupFailureIsGapNotAbsence`, `TestKeyword_ExactModeQueryIdentity`, `TestScopeRemoved_OldModeQueries`.
- [ ] Implement; commit `feat(corpus): discover named numbers and exact keyword queries`.

### Task 6: Expand stage

**Files:** Create `apps/corpus/internal/stage/expand/` with tests; modify expected-set loading (`discover/expected.go`) and `relate` resolution for `scope_changed`; migration for membership class and supporting seeds if data.md requires columns.

**Interfaces:** `expand.New(client *vbpl.Client) *expand.Stage` (name `expand`), registered between `discover` and `fetch` in Task 12; `relation_context` membership with supporting seeds, codes and evidence hashes.

- [ ] Tests first: `TestExpand_QualifyingKindsOutwardOnly`, `TestExpand_ContextNeverSeeds`, `TestExpand_ExcludesRenditionsAndSelfLinks`, `TestExpand_SourceFailureKeepsKnownTargets`, `TestExpand_TwoObservationWithdrawal`, `TestResolve_NewTargetIsScopeChanged`.
- [ ] Implement per sources.md "One-hop relation context"; commit `feat(corpus): expand stage for one-hop relation context`.

### Task 7: Retry source gaps on start

**Files:** Modify `apps/corpus/internal/run/start.go` and tests, SQL in `apps/corpus/sql/` (regenerate).

- [ ] Tests first: `TestStart_ResetsOnlySourceGapItems`, `TestStart_DeletesThoseGapRows`, `TestStart_RetryRebuildsDependents`, `TestStart_FrozenRecipesUnchanged`.
- [ ] Implement per pipeline.md; commit `feat(corpus): retry source gaps when a run starts again`.

### Task 8: chinhphu client

**Files:** Create `apps/corpus/internal/chinhphu/{client,search,detail,download}.go` with tests.

**Interfaces:** `chinhphu.New(opts ...Option) *Client`; `(*Client) Search(ctx, keyword string) (SearchResult, error)` (all pages, evidence-bearing); `(*Client) LookupNumber(ctx, number string) (SearchResult, error)`; `(*Client) Detail(ctx, docid int) (Detail, error)`; `(*Client) Download(ctx, url string, dst io.Writer, max int64) (FileInfo, error)`; `Candidate{DocID int; Number, Title, Issuer, IssueDate string}`.

- [ ] Tests first on Task 1 fixtures: `TestSearch_PaginatesAndDedupes`, `TestSearch_EmptyAndMultiple`, `TestDetail_ParsesCentralIssuerAndAttachments`, `TestDownload_HostAllowlistAndPDFSignature`, `TestClient_NoTokenInArtifacts`, `TestClient_ChallengePausesSource` (through the shared pause state, so concurrent batches and a restarted worker stop too).
- [ ] Implement; commit `feat(corpus): vanban.chinhphu.vn client`.

### Task 9: Secondary discovery and hand-over

**Files:** Modify `apps/corpus/internal/stage/discover/` (secondary items), expected-set rules; tests.

- [ ] Tests first: `TestAdmit_ZeroVBPLMatchAdmits`, `TestAdmit_UniqueVBPLMatchSeedsPrimary`, `TestAdmit_LookupFailureDefers`, `TestAdmit_CollisionIsIdentityAmbiguous`, `TestAdmit_ProvincialOutOfScope`, `TestHandOver_SourceReplaced`.
- [ ] Implement per sources.md "Secondary discovery and hand-over"; commit `feat(corpus): secondary discovery from vanban.chinhphu.vn`.

### Task 10: Secondary fetch and provenance

**Files:** Modify `apps/corpus/internal/stage/fetch/` (secondary bundles through the shared source-neutral writers), `extract` (`no_tree`), `relate` (empty evidenced set); tests. Build loading belongs to Task 11.

- [ ] Tests first: `TestFetch_ChinhphuBundleHashesFiles`, `TestExtract_ChinhphuNoTree`, `TestRelate_ChinhphuEmptySet`.
- [ ] Implement; commit `feat(corpus): fetch secondary documents`.

### Task 11: Build and evaluation integration (Plan 2 code)

**Files:** Modify `apps/corpus/internal/stage/build/` (inventory version 2 with expand evidence and membership in the identity hash, class and provenance loading, exclusions), `internal/corpusdb/schema.sql` (schema version 2, `document.class`), `internal/retrieval` only if class reaches responses, `apps/corpus/internal/evaluation/mapping.go` (reject identity maps bound to inventory version 1).

- [ ] Tests first: `TestInventory_V2IncludesExpandEvidence`, `TestInventory_V1MapRejected`, `TestLoad_ClassColumnAndProvenance`, `TestSchema_Version2ClassCheck`, `TestBuild_SecondaryMetadataOnly`.
- [ ] Implement per index.md, corpus-schema.md and evaluation.md as revised; commit `feat(corpus): build and evaluation for membership classes and secondary provenance`.

### Task 12: Wiring, config and proofs

**Files:** Modify `workflow/registry.go` (stage order `discover, expand, fetch, ...`), config (`CORPUS_CHINHPHU_*` settings), worker, CLI status, runbook; proof `apps/corpus/test/proof/scope_test.go`.

- [ ] Tests first: `TestRegistry_ExpandAfterDiscover`, `TestReplay_Histories` still passes, `TestProof_ScopeRunWithExpansionAndSecondary` (fake VBPL and chinhphu servers, under 60 seconds).
- [ ] Implement; commit `feat(corpus): wire expand and the secondary source`.

### Task 13: Live run and coverage (primary)

- [ ] Start a new run with the reviewed source list; record expansion size, requests, time, gaps by kind; confirm the four VBPL golden documents and the AI documents are present.
- [ ] Re-adjudicate the vn identity map for the new inventory; write `docs/research/vn-scope-run-2026-11.md`; mark this plan landed.
