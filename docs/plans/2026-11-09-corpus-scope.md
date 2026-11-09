# Corpus Scope Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Route in this repository: Task 1 is a primary spike with recorded fixtures; Tasks 2 to 10 each go to one Codex `implementer` (`gpt-6.1-terra` high) with standing answers, resumed by session id; Task 11 is the primary's live run; the Claude `reviewer` reviews the whole branch after Task 10.

**Goal:** A new Vietnam run reaches the documents agents need: named numbers and one-hop relation targets from VBPL, exact-mode keyword searches with an audited loss comparison, retry of source gaps, and central documents VBPL lacks from `vanban.chinhphu.vn` (starting with AI), all with explicit provenance.

**Architecture:** Discovery stays in `discover`; a new `expand` stage between `discover` and `fetch` adds one hop of explicit outward VBPL relation targets as class `relation_context`. A new `apps/corpus/internal/chinhphu` client and a secondary discovery path admit central documents only after an exact VBPL number lookup finds none; they are fetched as metadata plus signed PDFs (`no_tree` until the text extraction plan). Number normalization folds Cyrillic look-alikes in one shared function.

**Tech Stack:** Go 1.27.3, Temporal Go SDK v1.50.0, pgx/v5, sqlc 1.31.1, golang.org/x/net/html for page parsing, recorded fixtures.

**Spec:** `docs/design/sources.md`; `docs/design/pipeline.md` (run lifecycle, items, VBPL ingest rules, expansion and retries); `docs/design/data.md`; `docs/design/budgets.md` "Pipeline"; ADR 0012; `docs/research/vn-sources-2026-11.md`. The spec wins over this plan.

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

- A gateway or source failure read as "no document", so a missing lookup admits a duplicate secondary document or drops context (Task 4 `TestNamed_LookupFailureIsGapNotAbsence`, Task 8 `TestAdmit_LookupFailureDefers`).
- A number collision between distinct documents merged into one identity (Task 2 `TestNormalize_CyrillicLookalikes`, Task 8 `TestAdmit_CollisionIsIdentityAmbiguous`).
- Expansion that recurses or grows after the set closes (Task 5 `TestExpand_ContextNeverSeeds`, `TestResolve_NewTargetIsScopeChanged`).
- Session tokens or cookies written into artifacts or fixtures (Task 7 `TestClient_NoTokenInArtifacts`).
- Retry on start that resets fetch but leaves stale downstream rows (Task 6 `TestStart_RetryRebuildsDependents`).

---

### Task 1: Spike, fixtures and the keyword loss comparison (primary)

**Files:** Create `apps/corpus/internal/vbpl/testdata/` additions (number lookups: unique, colliding, empty; exact title; relation direction; non-JSON 200), `apps/corpus/internal/chinhphu/testdata/` (search first page, second page, empty, multiple, detail, attachment headers), `docs/research/vn-scope-comparison-2026-11.md`.

- [ ] Record redacted fixtures with `record.sh`-style scripts; reconcile the AI document list from `vanban.chinhphu.vn` (all result rows, with docids) without inventing entries.
- [ ] Run the keyword comparison: complete stable `all_words` and `exact` listings for every proposed term plus the full old and new seed sets; save the immutable comparison artifact (hash recorded in the note); account for every old-only id; owner approves exclusions.
- [ ] Measure the expansion set for run vn/20261109's seeds (unique targets, requests, bytes) against budgets.
- [ ] Commit `test(corpus): VBPL and chinhphu fixtures and the scope comparison`.

### Task 2: Source list and number normalization

**Files:** Modify `apps/corpus/internal/source/list.go` and test, `sources/vn.yaml`, `internal/retrieval/numbers.go` and test (or move the function to a shared root package named in the brief).

**Interfaces:** Produces `source.List.VBPL.NamedNumbers []string`, `VBPL.MatchMode string`, `Chinhphu{Keywords, NamedNumbers []string}`, query identity hashing over source, type, option, mode, level, agencies and keyword or number; `retrieval.NormalizeNumber` folding Cyrillic look-alikes.

- [ ] Tests first: `TestParse_NamedNumbersAndChinhphu`, `TestQueryIdentity_IncludesModeAndLevel`, `TestNormalize_CyrillicLookalikes` (`280/2026/NĐ-CР` with Cyrillic `Р` equals `280/2026/NĐ-CP`), `TestNormalize_ExistingCasesUnchanged`.
- [ ] Implement; `sources/vn.yaml` gets the reviewed lists from sources.md and Task 1.
- [ ] Commit `feat(corpus): source list for named numbers and the chinhphu source`.

### Task 3: VBPL client lookups

**Files:** Modify `apps/corpus/internal/vbpl/client.go`, `types.go` and tests.

**Interfaces:** Produces `(*Client) LookupNumber(ctx, number string) ([]ListItem, error)` (all pages) and `ListQuery.Option`, `ListQuery.Mode`; a non-JSON 200 body is a retryable error.

- [ ] Tests first against Task 1 fixtures: `TestLookupNumber_UniqueCollisionEmpty`, `TestList_ExactMode`, `TestAPI_NonJSON200Retryable`.
- [ ] Implement; commit `feat(corpus): exact number and title lookups in the VBPL client`.

### Task 4: Discover named numbers and exact keywords

**Files:** Modify `apps/corpus/internal/stage/discover/` and tests, `apps/corpus/sql/discover_expected.sql` as needed (regenerate the store).

**Interfaces:** Discover plans one item per named number besides query items; classes `primary` in the expected set; gaps `named_document_missing`, `named_document_ambiguous`.

- [ ] Tests first: `TestNamed_UniqueJoinsSeeds`, `TestNamed_ZeroAndManyAreGaps`, `TestNamed_LookupFailureIsGapNotAbsence`, `TestKeyword_ExactModeQueryIdentity`, `TestScopeRemoved_OldModeQueries`.
- [ ] Implement; commit `feat(corpus): discover named numbers and exact keyword queries`.

### Task 5: Expand stage

**Files:** Create `apps/corpus/internal/stage/expand/` with tests; modify expected-set loading (`discover/expected.go`) and `relate` resolution for `scope_changed`; migration for membership class and supporting seeds if data.md requires columns.

**Interfaces:** `expand.New(client *vbpl.Client) *expand.Stage` (name `expand`), registered between `discover` and `fetch` in Task 10; `relation_context` membership with supporting seeds, codes and evidence hashes.

- [ ] Tests first: `TestExpand_QualifyingKindsOutwardOnly`, `TestExpand_ContextNeverSeeds`, `TestExpand_ExcludesRenditionsAndSelfLinks`, `TestExpand_SourceFailureKeepsKnownTargets`, `TestExpand_TwoObservationWithdrawal`, `TestResolve_NewTargetIsScopeChanged`.
- [ ] Implement per sources.md "One-hop relation context"; commit `feat(corpus): expand stage for one-hop relation context`.

### Task 6: Retry source gaps on start

**Files:** Modify `apps/corpus/internal/run/start.go` and tests, SQL in `apps/corpus/sql/` (regenerate).

- [ ] Tests first: `TestStart_ResetsOnlySourceGapItems`, `TestStart_DeletesThoseGapRows`, `TestStart_RetryRebuildsDependents`, `TestStart_FrozenRecipesUnchanged`.
- [ ] Implement per pipeline.md; commit `feat(corpus): retry source gaps when a run starts again`.

### Task 7: chinhphu client

**Files:** Create `apps/corpus/internal/chinhphu/{client,search,detail,download}.go` with tests.

**Interfaces:** `chinhphu.New(opts ...Option) *Client`; `(*Client) Search(ctx, keyword string) ([]Candidate, error)` (all pages); `(*Client) LookupNumber(ctx, number string) ([]Candidate, error)`; `(*Client) Detail(ctx, docid int) (Detail, error)`; `(*Client) Download(ctx, url string, dst io.Writer, max int64) (FileInfo, error)`; `Candidate{DocID int; Number, Title, Issuer, IssueDate string}`.

- [ ] Tests first on Task 1 fixtures: `TestSearch_PaginatesAndDedupes`, `TestSearch_EmptyAndMultiple`, `TestDetail_ParsesCentralIssuerAndAttachments`, `TestDownload_HostAllowlistAndPDFSignature`, `TestClient_NoTokenInArtifacts`, `TestClient_ChallengePausesSource`.
- [ ] Implement; commit `feat(corpus): vanban.chinhphu.vn client`.

### Task 8: Secondary discovery and hand-over

**Files:** Modify `apps/corpus/internal/stage/discover/` (secondary items), expected-set rules; tests.

- [ ] Tests first: `TestAdmit_ZeroVBPLMatchAdmits`, `TestAdmit_UniqueVBPLMatchSeedsPrimary`, `TestAdmit_LookupFailureDefers`, `TestAdmit_CollisionIsIdentityAmbiguous`, `TestAdmit_ProvincialOutOfScope`, `TestHandOver_SourceReplaced`.
- [ ] Implement per sources.md "Secondary discovery and hand-over"; commit `feat(corpus): secondary discovery from vanban.chinhphu.vn`.

### Task 9: Secondary fetch and provenance

**Files:** Modify `apps/corpus/internal/stage/fetch/` (secondary bundles), `extract` (`no_tree`), `relate` (empty evidenced set), build loading of `source_file.source` and document class; tests.

- [ ] Tests first: `TestFetch_ChinhphuBundleHashesFiles`, `TestExtract_ChinhphuNoTree`, `TestRelate_ChinhphuEmptySet`, `TestLoad_SecondaryMetadataOnly`, `TestLoad_ClassAndProvenance`.
- [ ] Implement; commit `feat(corpus): fetch and load secondary documents`.

### Task 10: Wiring, config and proofs

**Files:** Modify `workflow/registry.go` (stage order `discover, expand, fetch, ...`), config (`CORPUS_CHINHPHU_*` settings), worker, CLI status, runbook; proof `apps/corpus/test/proof/scope_test.go`.

- [ ] Tests first: `TestRegistry_ExpandAfterDiscover`, `TestReplay_Histories` still passes, `TestProof_ScopeRunWithExpansionAndSecondary` (fake VBPL and chinhphu servers, under 60 seconds).
- [ ] Implement; commit `feat(corpus): wire expand and the secondary source`.

### Task 11: Live run and coverage (primary)

- [ ] Start a new run with the reviewed source list; record expansion size, requests, time, gaps by kind; confirm the four VBPL golden documents and the AI documents are present.
- [ ] Re-adjudicate the vn identity map for the new inventory; write `docs/research/vn-scope-run-2026-11.md`; mark this plan landed.
