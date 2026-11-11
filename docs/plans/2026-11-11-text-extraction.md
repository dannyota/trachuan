# Text Extraction Implementation Plan

> Route: each task goes to one Codex `implementer` (`gpt-6.1-terra` high), resumed by session id; the OCR cross-check client (Task 6) is hard and goes to `gpt-6.1-sol` with a `gpt-6-astra` review; the primary writes SQL, runs integration tests, proofs, the pilot and live runs, and owns Git; Codex `reviewer` (`gpt-6.1-sol` high) reviews each merged slice. Three pull requests: A (Tasks 1 to 4), B (Tasks 5 to 7), then the run (Task 8).

**Goal:** The 502 documents that have text only in an HTML transcription (188) or a PDF (314) become searchable with article-level citations, marked with their text source, so agents can find consolidated texts and the secondary documents.

**Spec:** `docs/design/text-extraction.md`, `docs/adr/0014-ocr-on-the-write-path.md`, `docs/research/vn-text-inventory-2026-11.md`, `docs/design/corpus-schema.md`, `docs/design/mcp-results.md`, `docs/design/budgets.md` "Pipeline". The spec wins over this plan.

## Global Constraints

- A source-tree document's fetch, extract, chunk and embed artifacts stay byte-identical; a compatibility test pins recorded hashes.
- Fail closed: a scope that fails the article sequence check asserts no article; an OCR text without a completed cross-check covering every page is never indexed; validity rules read effect articles only from source trees.
- The cross-check model never rewrites stored text. Only public legal text is sent to Google Cloud Vision or the model. Credentials are read from the environment variables the runbook names and never logged.
- Tests never call a network service; fixtures are recorded and committed by the primary. Non-test Go files at most 400 lines.

### Task 1: Fixtures and expected provisions (primary)

- [ ] Record bundles from run 6 per stratum: articles, numbered outline, plain, tables, images, a second instrument (regulation issued with a decision), quoted amendments with footnotes, an empty transcription, two banhmi OCR texts (one consolidated circular, 142/2026/NĐ-CP).
- [ ] Write the expected provision list for each by hand from the file (anchors, article boundaries, scope), and adverse fixtures that must fall back to neutral anchors; commit under `apps/corpus/internal/textparse/testdata/`.

### Task 2: Paragraphs and the provision parser

**Files:** new `apps/corpus/internal/textparse/` (HTML to paragraphs with locators; plain OCR text to paragraphs; grammar, scopes, strict sequence check, neutral provisions, node ids).

- [ ] Tests first against Task 1 fixtures: one per fixture plus `TestParse_SequenceGapFallsBackToNeutral`, `TestParse_SecondInstrumentScopedAnchors`, `TestParse_RestartWithoutTitleIsUncertain`, `TestParse_NodeIDBindsScopeAnchorText`, `TestParse_TableAndImageMarkers`, `TestParse_ShortTextIsNoText`, `TestParagraphs_LimitsBecomeGap`.
- [ ] Implement; commit `feat(corpus): paragraph and article parser for transcribed text`.

### Task 3: Extract stage, text source selection and inputs

**Files:** `apps/corpus/internal/stage/extract/` (source selection, `text.json`, input hash for documents without a source tree, gaps), `apps/corpus/internal/stage/chunk/` (read generated provisions), recipe strings.

- [ ] Tests first: `TestExtract_SourceTreeArtifactBytesUnchanged` (pinned hashes through chunk and embed inputs), `TestExtract_HTMLTranscriptionBecomesProvisions`, `TestExtract_IgnoresOriginAndBackupVariants`, `TestExtract_NoSourceGaps` (each gap kind), `TestExtract_InputHashFreezesSelection`.
- [ ] Implement with the OCR source wired to an interface that returns "absent" until Task 5; commit `feat(corpus): extract provisions from HTML transcriptions`.

### Task 4: Schema version 4, build, retrieval and validity gating

**Files:** `internal/corpusdb` (`text_source`, `text_complete`, version 4), `apps/corpus/internal/stage/build/` (load both, validity rules read effect articles only from `source_tree`), `internal/retrieval` (document entry fields, provision locators), evaluation report counts by text source.

- [ ] Tests first: `TestSchema_Version4Constraints`, `TestOpen_RejectsSchemaVersion3`, `TestLoad_TextSourceAndComplete`, `TestValidity_ParsedTextIsReviewOnly`, `TestHit_DocumentEntryCarriesTextSource`, proof `TestProof_TranscriptionDocumentIsSearchable`.
- [ ] Implement; whole-slice `gpt-6.1-sol` review; pull request A.

### Task 5: OCR store and banhmi import

**Files:** primary: migration and SQL for `ocr_text` (PDF SHA-256, engine id, text hash, text path, origin, pages) and `ocr_check` (entry, checker id, result hash, state, flagged items); new `apps/corpus/internal/ocrstore/`; CLI `corpus ocr import-banhmi <dir>` (reads `<dir>/ocr/<sha256>.txt`, verifies the PDF hash exists in the run's fetch artifacts, never overwrites); extract's OCR source reads the store.

- [ ] Tests first: `TestStore_EntriesImmutable`, `TestImport_OnlyMatchingHashes`, `TestExtract_OCRNeedsCompletedCheck`, `TestExtract_SinglePDFSelection`, `TestExtract_AmbiguousPDFsGap`.
- [ ] Implement; commit `feat(corpus): OCR store and import of existing Google Vision output`.

### Task 6: Cross-check and fresh OCR clients (hard route)

**Files:** new `apps/corpus/internal/ocrcheck/` (page images from the PDF, request to the Claude API with the page image and its OCR text, structured result listing disagreements in document numbers, dates, article labels and amounts, batch mode, resume, cost accounting), `apps/corpus/internal/ocrvision/` (Google Cloud Vision document text detection for PDFs absent from the store), CLI `corpus ocr check` and `corpus ocr run` with `--limit` and a dry-run cost estimate.

- [ ] Read the provider documentation for model ids, structured output and batch processing before writing the brief; pin the model id and prompt in the checker id.
- [ ] Tests first with recorded responses: `TestCheck_ParsesDisagreements`, `TestCheck_PartialOrMalformedIsIncomplete`, `TestCheck_NeverChangesStoredText`, `TestCheck_ResumeSkipsCompletedPages`, `TestVision_StoresByPDFHash`, `TestOCR_PageAndRunLimits`, `TestClients_NoCredentialInLogsOrArtifacts`.
- [ ] Implement; `gpt-6-astra` review.

### Task 7: Pilot and full cross-check (primary)

- [ ] Import banhmi's OCR output; run the cross-check on 50 pages across 10 documents; record tokens, cost, flagged items and a manual reading of every flag in `docs/research/vn-ocr-check-2026-11.md`; report the measured cost to the owner before the full pass.
- [ ] Full cross-check; fresh OCR for the documents absent from the store; review the flagged list; pull request B.

### Task 8: Run and acceptance (primary)

- [ ] New run; counts by text source and gap; every accepted article-level scope checked against the article count in its file; validity review queue and identity-map changes reviewed; add 35/VBHN-NHNN alternates and re-adjudicate affected entries; real-output check on cases citing recovered documents; research note; mark this plan landed.
