# Compact Results and Batch Read Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Routes (`instructions/workflow.md`): Tasks 1 to 5 and 7 go to Codex `implementer` on `gpt-6.1-terra`, reviewed by `gpt-6.1-sol`; Task 6 (read engine: units, parts, tokens, call cap) goes to `implementer` with `--model gpt-6.1-sol`, reviewed by `reviewer` with `--model gpt-6-astra`; Task 8 is the primary's. The primary verifies every task, runs `make pre-push` and `make test-integration`, and owns Git.

**Goal:** Give the shared retrieval engine the agent-facing result contract of ADR 0013: compact or full search presentation with document entries and relation pointers, and a batch `read` with fixed parts and `next` tokens; measure compact size and a compact diagnostic score in evaluation.

**Architecture:** `internal/retrieval` keeps recipe 1 unchanged (the release still opens only when its recipe equals `Recipe1()`). `Hit` gains its citation path and best-member offset. New engine methods `Present` (search presentation) and `Read` (batch reading) run on the release database the engine already pins; presentation limits are package constants, not recipe fields. The eval stage calls `Present` on the responses it already shapes. MCP tools, `guide` text, OpenAPI and the agent-in-the-loop check belong to Plan 9.

**Tech Stack:** Go 1.27.3, pgx v5, PostgreSQL 18 with the corpus schema in `internal/corpusdb/schema.sql`; integration tests under build tag `integration` with `internal/testpg`.

**Spec:** [ADR 0013](../adr/0013-compact-search-and-batch-read.md), [search results](../design/mcp-results.md), [budgets](../design/budgets.md) ("Retrieval", "Request limits"), [retrieval recipe](../design/retrieval-recipe.md) ("Go boundary", "Validity and evidence"), [chunk](../design/chunk.md) ("Tree rendering"). The spec wins over this plan.

## Scope

|In|Out (later plans)|
|-|-|
|`Hit.Path`, `Hit.BestMember`, `Hit.BestTextStart`; `Snippet`; handles and `next` tokens; `Engine.Present` (documents, pointers, compact and full hits); `Engine.Read` (outline, provision, range, relations, parts, call cap); eval compact diagnostic, compact size and the size gate report; release measurement|MCP tool registration, `outputSchema`, `guide` text, web API and OpenAPI (Plan 9); `in_filter`, `nodes`, `outside_filter_count` and search `next_cursor` (land with taxonomy and the subject filter, Plan 6); routing a `next` token to a retained release database (Plan 9); agent-in-the-loop check (after Plan 9)|

## Global Constraints

- Limits in `internal/retrieval/limits.go`, each with a comment naming budgets: `SnippetCap = 600`, `CompactDefaultLimit = 20`, `FullDefaultLimit = 8`, `MaxLimit = 50`, `PointersPerDocument = 5`, `ReadItemsMax = 8`, `ReadCallCap = 40000`, `PartSize = 16000`, `RelationPageSize = 20`. Relation evidence per relation uses the recipe's `RelationEvidenceRuneCap` (1,000). Sizes count Unicode code points.
- `Recipe`, `Recipe1()` and the retrieval recipe hash do not change.
- Omission marker `OmissionMarker = "[…]"`, separated by one space from adjacent text; the only text not taken from the source.
- Handle: `<key>` or `<key>#<anchor>`; keys start `vn:` and contain no `#`; split at the first `#`.
- Pointer kind order: `repeals`, `replaces`, `suspends`, `amends_supplements`, `consolidates`, `implements`, `guided_by`, `legal_basis`, `cites`, `references`, `unknown`; then `in` before `out`; then the other document's `issue_date` descending with nulls last; then other number; then `(source_key, artifact_sha256, ordinal)`.
- Warning kinds (provision-level relations shown on a hit): `repeals`, `replaces`, `suspends`, `amends_supplements`.
- JSON: null and empty fields omitted; booleans always present; keys exactly as in `mcp-results.md`. Item error codes: `not_found`, `ambiguous_number`, `release_expired`, `invalid_range`, `invalid_next`, `invalid_item` (not exactly one item form); `not_read` value `call_cap`.
- Cuts: last whitespace at or before the limit; without whitespace, the last code point at or before the limit that does not separate a combining mark (Unicode category Mn) from its base.
- No query text, returned text or tokens in logs or error messages; errors wrapped with `%w`; Go files at most 400 lines, one concern each; SQL binds every value; statements run under the engine's existing deadline.

## Review Focus

- A hostile or stale `next` token (edited unit, negative or huge part, unknown field, other release, wrong item kind) must return `invalid_next` or `release_expired` for that item, never panic, loop or read out of range: Task 4 `TestDecodeNext_RejectsTampered`, Task 6 `TestRead_TamperedNextIsItemError`.
- A document with thousands of relations (a Luật with about 2,000 incoming `legal_basis` edges) must present 5 pointers and exact counts without loading every row: Task 3 `TestDocuments_ManyRelationsCountsAndTopFive`.
- NFD Vietnamese text or a long span without whitespace must never be cut between a base letter and its combining mark: Task 2 `TestSnippet_NoWhitespaceKeepsCombiningMark`, Task 6 `TestSplitParts_NoWhitespaceKeepsCombiningMark`.
- The same handle twice in one call, or a call whose first item alone exceeds the cap, must still make progress and return deterministic pages: Task 6 `TestRead_DuplicateItemsIndependent`, `TestRead_FirstItemAlwaysReturnsUnit`.
- Anchors with `/`, `~2` or Vietnamese letters must round-trip through handles and tokens: Task 4 `TestHandle_RoundTripAnchors`.

---

### Task 1: Hit path and best member

**Files:**
- Modify: `internal/retrieval/response.go` (types `PathNode`, `Hit`), `internal/retrieval/evidence.go` (`evidence` returns the best offset), `internal/retrieval/shape.go` (`shapeHit` fills the fields)
- Test: `internal/retrieval/shape_test.go`

**Interfaces:**
- Produces: `type PathNode struct{ Identity Identity \`json:"identity"\`; Kind string \`json:"kind"\`; Label string \`json:"label"\`; Heading string \`json:"heading,omitempty"\` }`; `Hit.Path []PathNode \`json:"path"\`` (root first, the hit last, labels and headings trimmed); `Hit.BestMember Identity \`json:"best_member"\``; `Hit.BestTextStart int \`json:"best_text_start"\`` (rune offset in `Hit.Text`).

- [ ] **Step 1: Write the failing tests** with `shapeFixture`-style groups: `TestShapeHit_PathFromAncestors` (chuong "Chương II" heading "Quyền", dieu "Điều 5" heading "Yêu cầu", khoan "Khoản 2": `Path` has three nodes in that order with those labels and headings, last identity equals `Hit.Identity`); `TestShapeHit_BestTextStartCompleteHit` (complete article whose best chunk owns clause 2: `[]rune(h.Text)[h.BestTextStart:]` starts with clause 2's trimmed text); `TestShapeHit_BestTextStartTruncatedHit` (hit above the evidence cap with a lead: `BestTextStart == len([]rune(lead))+1`); `TestShapeHit_BestTextStartWithoutLeads` (truncated, no lead: 0); `TestShapeHit_BestMemberIsBestChunkOwner`.
- [ ] **Step 2: Run** `go test ./internal/retrieval/ -run 'TestShapeHit_'`. Expected: FAIL (fields undefined).
- [ ] **Step 3: Implement.** `evidence` also returns the offset: truncated hits start the body right after the leads (plus the joining newline when leads exist); complete hits map the best chunk's first `body` span (lowest `ContentStart`) through the tree fragment with the same identity: `f.TextStart + span.Start - f.Start`. `BestMember` is that span's owning provision identity. `Path` comes from `Candidate.Ancestors` in root-to-group order.
- [ ] **Step 4: Run** `go test ./internal/retrieval/` and `make test-integration` for `./internal/retrieval/...`. Expected: PASS; existing shape tests unchanged.
- [ ] **Step 5: Commit** `feat(retrieval): citation path and best member on hits`.

### Task 2: Snippet

**Files:**
- Create: `internal/retrieval/snippet.go`, `internal/retrieval/limits.go`, `internal/retrieval/snippet_test.go`

**Interfaces:**
- Consumes: Task 1 `Hit.BestTextStart`, `Hit.Fragments` (role `lead`).
- Produces: `const OmissionMarker = "[…]"`; the limit constants of Global Constraints; `func Snippet(h Hit, capRunes int) (text string, complete bool)`; `func cutAt(r []rune, limit int) int` (shared cut rule, also used by Task 6).

- [ ] **Step 1: Write the failing tests:** `TestSnippet_ShortHitIsWholeText` (text of 300 runes, `Complete:true`: same text, true); `TestSnippet_ShortButRecipeIncomplete` (false); `TestSnippet_LeadThenBody` (lead 100, body from `BestTextStart` at the hit's own text start: lead, newline-joined body, trailing ` […]`, rune count at most 600, false); `TestSnippet_LongLeadCutAtHalf` (lead 900: lead part at most 300 runes ending ` […]`); `TestSnippet_BodyNotAtStartGetsLeadingMarker`; `TestSnippet_CutAtWhitespace` (cut never ends inside a word); `TestSnippet_NoWhitespaceKeepsCombiningMark` (a 700-rune NFD span of `a` plus U+0301 pairs: output never ends with a base letter whose mark was dropped); `TestSnippet_MarkersCountTowardCap`.
- [ ] **Step 2: Run** `go test ./internal/retrieval/ -run TestSnippet`. Expected: FAIL.
- [ ] **Step 3: Implement** the snippet rule of `mcp-results.md` exactly; `cutAt` returns the cut index under the Global Constraints cut rule and at least 1 for a nonempty span.
- [ ] **Step 4: Run** the same command. Expected: PASS.
- [ ] **Step 5: Commit** `feat(retrieval): verbatim snippets with omission markers`.

### Task 3: Document entries and relation pointers

**Files:**
- Create: `internal/retrieval/documents.go`, `internal/retrieval/pointers.go`, `internal/retrieval/pointers_test.go`, `internal/retrieval/documents_integration_test.go`

**Interfaces:**
- Produces: `type Pointer struct{ Dir, Kind, Key, Number string; InRelease bool; Handle string }` (JSON `dir`, `kind`, `key,omitempty`, `number`, `in_release`, `handle,omitempty`); `type Status struct{ Class, Basis, StatusBasis string; Current bool }` (JSON `class`, `basis`, `status_basis`, `current`); `type DocumentEntry struct{ Number, Title, Type, Issuer string; Issued, EffectiveFrom, EffectiveTo *Date; Status Status; SourceURL string; Related []Pointer; RelatedCounts map[string]int }` (JSON `number`, `title`, `type`, `issuer`, `issued`, `effective_from`, `effective_to`, `status`, `source_url`, `related`, `related_counts`); `func (e *Engine) Documents(ctx context.Context, keys []string) (map[string]DocumentEntry, error)` (metadata, `Related` and `RelatedCounts`; `Status` is filled by Task 5 from hit stamps); `func pointerKindRank(kind string) int`.

- [ ] **Step 1: Write the failing tests.** Unit: `TestPointerKindRank_Order` (Global Constraints order; unknown kind sorts last). Integration, seeding like `fixture` in `internal/retrieval/engine_integration_test.go`: `TestDocuments_MetadataAndCounts` (keys `in:implements` 2, `out:legal_basis` 1); `TestDocuments_PointerOrderTotal` (two pointers equal on kind, dir, date and number order by `(source_key, artifact_sha256, ordinal)`); `TestDocuments_EndpointOutsideRelease` (stored `target_key` kept, `in_release:false`, no `handle`); `TestDocuments_UnresolvedEndpointNumberOnly`; `TestDocuments_ManyRelationsCountsAndTopFive` (3,000 incoming `legal_basis` plus one incoming `repeals`: `repeals` first, five pointers, count 3,000); `TestDocuments_UnknownKeyAbsent`.
- [ ] **Step 2: Run** `go test ./internal/retrieval/ -run TestPointerKindRank` and `go test -tags integration ./internal/retrieval/ -run TestDocuments`. Expected: FAIL.
- [ ] **Step 3: Implement** with three bound queries in one read-only transaction: metadata `WHERE key = ANY($1)`; counts `GROUP BY` document, direction and kind; the first `PointersPerDocument` pointers per document by `ROW_NUMBER() OVER (PARTITION BY document ORDER BY array_position($kinds, kind), dir, other.issue_date DESC NULLS LAST, other_number, source_key, artifact_sha256, ordinal)`. `in` means this document is the target; a self-relation appears once per direction.
- [ ] **Step 4: Run** the Step 2 commands. Expected: PASS.
- [ ] **Step 5: Commit** `feat(retrieval): document entries with relation pointers`.

### Task 4: Handles and next tokens

**Files:**
- Create: `internal/retrieval/handle.go`, `internal/retrieval/token.go`, `internal/retrieval/handle_test.go`, `internal/retrieval/token_test.go`

**Interfaces:**
- Produces: `func Handle(id Identity) string`; `func ParseHandle(s string) (key, anchor string, err error)`; `type ReadItem struct{ Handle, Number, From, To string; Relations bool; Next string }` (JSON `handle`, `number`, `from`, `to`, `relations`, `next`, all omitempty); `type cursor struct{ V int; Release string; Item ReadItem; Unit, Part int }`; `func encodeNext(c cursor) string`; `func decodeNext(s string) (cursor, error)`; `var errInvalidNext`.

- [ ] **Step 1: Write the failing tests:** `TestHandle_RoundTripAnchors` (anchors `dieu-1/khoan-1/diem-a`, `dieu-3~2`, `phụ-lục-1`; document handle without `#`); `TestParseHandle_Rejects` (empty, no `vn:` prefix, empty key before `#`, empty anchor after `#`); `TestNext_RoundTrip`; `TestDecodeNext_RejectsTampered` (bad base64, unknown field, `V != 1`, negative unit or part, nested `Next` inside `Item`, both `Handle` and `Number`, trailing data: all `errInvalidNext`).
- [ ] **Step 2: Run** `go test ./internal/retrieval/ -run 'TestHandle|TestParseHandle|TestNext|TestDecodeNext'`. Expected: FAIL.
- [ ] **Step 3: Implement.** Tokens are unpadded base64url of strict JSON (`DisallowUnknownFields`, exactly one value); `Unit` and `Part` are checked against real sequences in Task 6, not here.
- [ ] **Step 4: Run** the same command. Expected: PASS.
- [ ] **Step 5: Commit** `feat(retrieval): handles and read continuation tokens`.

### Task 5: Present search results

**Files:**
- Create: `internal/retrieval/present.go`, `internal/retrieval/present_types.go`, `internal/retrieval/present_test.go`, `internal/retrieval/present_integration_test.go`

**Interfaces:**
- Consumes: Tasks 1 to 4.
- Produces: `type Detail string` with `DetailCompact = "compact"`, `DetailFull = "full"`; `func DefaultLimit(d Detail) int` (20, 8); `type PresentedHit struct{ Handle, Doc, Citation string; Path []PresentedPathNode; Snippet string; Complete bool; Warnings []Pointer; Gaps []Gap; Text string; Fragments []Fragment; Score *float64; Relations []Relation; Next string }` (JSON in `mcp-results.md` order; `snippet`, `text`, `fragments`, `score`, `relations`, `next` omitempty); `type PresentedPathNode struct{ Label, Heading, Handle string }`; `type SearchResult struct{ Release string; Abstain bool; Reason, Message string; Documents map[string]DocumentEntry; Hits, NonCurrent []PresentedHit; Gaps []Gap }`; `func present(resp Response, docs map[string]DocumentEntry, d Detail) SearchResult` (pure); `func (e *Engine) Present(ctx context.Context, resp Response, d Detail) (SearchResult, error)`.

- [ ] **Step 1: Write the failing tests.** Unit on `present`: `TestPresent_DocumentsOnce` (three hits of one document: one entry; `Status` from the first hit's stamp); `TestPresent_CompactHitFields` (handle, doc, citation, path handles, snippet from `Snippet`, `complete`; no `text`, `score` or `relations` in the JSON); `TestPresent_FullHitFields` (recipe `text`, `fragments`, `score`, `relations`, `complete` equals `Hit.Complete`, no `snippet`; incomplete hit has `next` that `decodeNext` reads as item `{handle}` at unit 0); `TestPresent_WarningsOnlyProvisionLevel` (a document-level `repeals` is not a warning; a `repeals` targeting a descendant anchor is); `TestPresent_JSONOmitsEmptyKeepsBooleans` (golden JSON with `"abstain":false` and `"complete":false`); `TestPresent_NonCurrentDocumentsIncluded`; `TestDefaultLimit`. Integration: `TestPresent_LoadsDocumentsForAllHits`.
- [ ] **Step 2: Run** `go test ./internal/retrieval/ -run 'TestPresent|TestDefaultLimit'` and `go test -tags integration ./internal/retrieval/ -run TestPresent`. Expected: FAIL.
- [ ] **Step 3: Implement**; `Present` calls `Documents` once for the distinct keys of `Hits` and `NonCurrent`, then `present`.
- [ ] **Step 4: Run** the Step 2 commands. Expected: PASS.
- [ ] **Step 5: Commit** `feat(retrieval): compact and full search presentation`.

### Task 6: Read

**Files:**
- Create: `internal/retrieval/read.go` (entry, item dispatch, call cap), `internal/retrieval/read_units.go` (unit sequences, leads, parts), `internal/retrieval/read_sql.go` (loaders), `internal/retrieval/read_types.go`, `internal/retrieval/read_test.go`, `internal/retrieval/read_integration_test.go`

**Interfaces:**
- Consumes: Tasks 2 to 5 (`cutAt`, `Pointer` order, handles, tokens, `DocumentEntry`); `renderTree`.
- Produces: `type ReadRequest struct{ Release string; Items []ReadItem }`; `type Unit struct{ Kind, Handle, Citation, Label, Heading, Text string; Size int; Relation *ListedRelation; Part, Parts int }` (JSON `kind` in `lead`, `provision`, `outline`, `relation`; others omitempty); `type ListedRelation struct{ Pointer; RawLabel, Evidence string; EvidenceComplete bool }`; `type ItemResult struct{ Document *DocumentEntry; Units []Unit; Complete bool; Next, NotRead, Error string; Gaps []Gap }`; `type ReadResult struct{ Release string; Items []ItemResult }`; `var ErrTooManyItems`; `func (e *Engine) Read(ctx context.Context, req ReadRequest) (ReadResult, error)`; `func splitParts(r []rune, partSize int) [][]rune`.

- [ ] **Step 1: Write the failing tests.** Unit: `TestSplitParts_LosslessAndPositive` (concatenation equals input; each part 1 to 16,000 runes); `TestSplitParts_NoWhitespaceKeepsCombiningMark`; `TestSplitParts_FixedBoundaries` (same input gives the same parts). Integration with a seeded document (phan, chuong, muc, dieu, khoan, diem; one 40,000-rune article; one document without `dieu`; relations): `TestRead_DocumentOutline` (entry plus `phan`, `chuong`, `muc`, `dieu` units with sizes; top-level provisions for the document without `dieu`); `TestRead_ProvisionWithLeads` (clause: lead units of its article first, then the clause with descendants; `complete:true`, no `next`); `TestRead_LabelOnlyLeadOmitted`; `TestRead_AmbiguousTreeIncomplete` (gap `ambiguous_tree_text`, `complete:false`); `TestRead_RangeInclusiveOwnTextOnce` (`from` khoản 1 `to` khoản 3: each provision once, own text only, leads of ancestors outside the range first); `TestRead_InvalidRange` (`to` before `from`, different documents); `TestRead_LongProvisionParts` (40,000-rune article: parts of at most 16,000, `part`/`parts`, chain via `next`; only the last page `complete:true`; concatenated parts equal the article); `TestRead_CallCapNotRead` (second item that does not fit returns `not_read:"call_cap"` and a `next` whose result equals reading that item first in a new call); `TestRead_FirstItemAlwaysReturnsUnit`; `TestRead_DuplicateItemsIndependent`; `TestRead_RelationsPaged` (45 relations: pages of 20 in pointer order with `raw_label` and evidence cut at 1,000 runes, `evidence_complete`); `TestRead_NumberLookup` (unique number reads like its handle; duplicate `number_norm` gives `ambiguous_number`; missing gives `not_found`); `TestRead_ItemErrorsDoNotFailCall`; `TestRead_TamperedNextIsItemError` (unit past the end, wrong item kind, other release gives `release_expired`); `TestRead_TooManyItems` (nine items: `ErrTooManyItems`); `TestRead_ResultSameWhateverElseCallReads`.
- [ ] **Step 2: Run** `go test ./internal/retrieval/ -run TestSplitParts` and `go test -tags integration ./internal/retrieval/ -run TestRead`. Expected: FAIL.
- [ ] **Step 3: Implement.** Each item resolves to a fixed unit sequence (document entry with the first page; then leads, provisions, outline entries or relations); a unit's size is the rune count of its JSON encoding; units above `PartSize` split with `splitParts`; the first item always returns its next unit or part; later items return whole units or parts while they fit `ReadCallCap`; a page with more to return carries `next = encodeNext(...)`; the last page has `Complete` true when nothing in the chain was cut or ambiguous. Leads follow the chunk rule: the ancestor's text before its first child, never label-only. Load provisions by `(document_id, anchor)` and preorder ranges; relations by the Task 3 order with `LIMIT`/`OFFSET` from the cursor unit.
- [ ] **Step 4: Run** the Step 2 commands, `go test -race ./internal/retrieval/`, `make test-integration`. Expected: PASS.
- [ ] **Step 5: Commit** `feat(retrieval): batch read with fixed parts and continuation`.

### Task 7: Evaluation compact diagnostic and size gate

**Files:**
- Modify: `apps/corpus/internal/evaluation/calibrate.go` (`Engine` interface gains `Present`), `apps/corpus/internal/evaluation/run.go` (hybrid arms, limits 8 and 20), `apps/corpus/internal/evaluation/report.go` (new fields), `apps/corpus/internal/evaluation/metrics_report.go`, `apps/corpus/internal/stage/eval/eval.go` (recipe string gains `compact/1`)
- Test: `apps/corpus/internal/evaluation/evaluation_test.go`, `run_test.go`

**Interfaces:**
- Consumes: Task 5 `Engine.Present`, `DetailCompact`.
- Produces: `type PresentedSummary struct{ Cohort string; Mode retrieval.Mode; Detail retrieval.Detail; Limit, Cases, Expected, Found int; SizeP50, SizeP95 *float64 }` (`Found` is the compact diagnostic for `compact`, the product count for `full`); `type CompactSizeGate struct{ FullP95At8, CompactP95At20 *float64; Pass bool }` (old product, exact mode); `Report.Presented []PresentedSummary \`json:"presented"\``; `Report.CompactSizeGate CompactSizeGate \`json:"compact_size_gate"\``.

- [ ] **Step 1: Write the failing tests:** `TestRun_CompactDiagnosticOwnIdentityOnly` (a parent hit that earns child credit in full earns nothing in compact; an own-identity match counts); `TestRun_PresentedSizeIsSerializedResult` (for both details, size equals the rune count of `json.Marshal(SearchResult)` without whitespace); `TestRun_CompactSizeGate` (pass when compact p95 at 20 is at most full p95 at 8, else false; full scoring, floors and promotion fields unchanged); `TestRecipe_IncludesCompact`.
- [ ] **Step 2: Run** `go test ./apps/corpus/internal/evaluation/ ./apps/corpus/internal/stage/eval/`. Expected: FAIL.
- [ ] **Step 3: Implement**: after each hybrid `Shape` at limits 8 and 20, call `Present` with `DetailCompact` and `DetailFull`; score compact with `MatchProduct` over hits stripped of `Fragments` and `Relations`; the gate compares serialized sizes and does not fail the run.
- [ ] **Step 4: Run** the Step 2 command and `make test-integration`. Expected: PASS.
- [ ] **Step 5: Commit** `feat(eval): compact diagnostic and response size gate`.

### Task 8: Measure and record (primary)

- [ ] **Step 1:** `go run ./apps/corpus/cmd/corpus eval vn <current release> --out work/eval-compact` (runbook `docs/runbooks/corpus-local.md`); read `presented`, `compact_size_gate` and the unchanged full metrics.
- [ ] **Step 2:** If the gate fails, lower `SnippetCap` or `CompactDefaultLimit` in `budgets.md` and `limits.go` together, rerun, and record the reason in ADR 0013 History.
- [ ] **Step 3:** Add a research note `docs/research/compact-results-2026-11.md` (sizes p50/p95 for compact at 8 and 20 and full at 8 and 20, compact diagnostic recall, gate result); update `docs/architecture.md` with `Present` and `Read`; `make pre-push`; commit `docs: compact result measurements`.
- [ ] **Step 4:** Tell the Plan 9 owner that `Present`, `Read`, `DefaultLimit` and `ReadItemsMax` are ready for the MCP tools and web API.
