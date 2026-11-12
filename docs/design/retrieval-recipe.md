# Shared retrieval recipe 1

Proposed root package `internal/retrieval`, imported by `apps/corpus` and the
future `apps/server`. It returns provisions from one pinned corpus database. No
API transport, authentication, taxonomy, implicit document scoping, abbreviation
expansion, accent restoration or model selection lives here.
[Retrieval](retrieval.md) owns the product interface; this note owns the
unfiltered engine. [Budgets](budgets.md#retrieval) owns numeric parameters.

## Go boundary

Use package files `types.go`, `recipe.go`, `validate.go`, `engine.go`,
`dense.go`, `lexical.go`, `fusion.go`, `group.go`, `validity.go`, `evidence.go`.
Keep SQL beside its owning operation; all runtime values are bound parameters.
The API is:

```go
func Open(ctx context.Context, db *pgxpool.Pool, embedder Embedder, options Options) (*Engine, error)
func (e *Engine) Search(ctx context.Context, request Request) (Response, error)
func (e *Engine) Prepare(ctx context.Context, request Request) (Prepared, error)
func (e *Engine) Collect(ctx context.Context, prepared Prepared, depths Depths) (Arms, error)
func Rank(arms Arms, recipe Recipe, limit int, mode ArmMode) (Ranking, error)
func Shape(arms Arms, recipe Recipe, limit int, mode ArmMode) (Response, error)
```

`Open` loads the singleton release row and validates schema, recipes, vector
typmod, lexical stats and query embedder report. Do not accept defaults on a
missing release field. Build calibration uses
`OpenDraft(ctx,db,embedder, Release,options)`, restricted to trusted batch
callers; it performs the same validation using explicit metadata before the
singleton is inserted. A future server must only call `Open`. `Options` has
`Now func() time.Time` (required) and `Profile` (`serving` or `evaluation`),
selecting the statement deadline in budgets. Clock/profile are constructor
dependencies, never request options. No global mutable state or cross-release
caches.

Draft metadata omits only the threshold and calibration hash. `OpenDraft`
permits `Collect` for calibration; `Search` refuses an uncalibrated draft. All
final-release fields remain mandatory in `Open`; no placeholder threshold or
self-referencing calibration hash is serialized.

Types and required fields:

| Type          | Fields / behavior                                                                                                                                                                                                                                                                                                                                                                                                                              |
| ------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `Embedder`    | `Recipe(context.Context) (DenseRuntime,error)`; `Embed(context.Context,string) (Vector,error)`                                                                                                                                                                                                                                                                                                                                                 |
| `Vector`      | `Values []float32`, `TokenCount int`, `RecipeHash string`                                                                                                                                                                                                                                                                                                                                                                                      |
| `Mode`        | String enum `exact`, `ann`; empty means `ann`                                                                                                                                                                                                                                                                                                                                                                                                  |
| `ArmMode`     | String enum `hybrid`, `dense`, `lexical`; empty means `hybrid`                                                                                                                                                                                                                                                                                                                                                                                 |
| `Request`     | `Query string`, `Limit int`, `InForceOnly *bool`, `AsOf *Date`, `Mode Mode`, `Arms ArmMode`                                                                                                                                                                                                                                                                                                                                                    |
| `Date`        | Valid civil year/month/day; encode `YYYY-MM-DD`, no time zone ambiguity                                                                                                                                                                                                                                                                                                                                                                        |
| `Depths`      | `Dense int`, `Lexical int`; bounded research-only overrides                                                                                                                                                                                                                                                                                                                                                                                    |
| `Identity`    | `Country`, `DocumentKey`, `Anchor` strings                                                                                                                                                                                                                                                                                                                                                                                                     |
| `Fragment`    | `Identity Identity`, `Start,End int` source rune offsets, `TextStart,TextEnd int` offsets in hit text, `Role string` (`body` or `lead`), `Complete bool`                                                                                                                                                                                                                                                                                       |
| `Hit`         | `Identity`, `Number`, `Citation`, `Title`, `Text`, `Fragments []Fragment`, `BestMember Identity`, `BestTextStart int` (offset in `Text` of the best fused member's first body span; when that span is not in `Text`, `BestMember` is the hit itself and the offset is where its own text starts), `Complete bool`, `NextPart *int`, `SourceURL *string`, `Score`, `BestCosine float64`, `Stamp Validity`, `Relations []Relation`, `Gaps []Gap` |
| `Validity`    | `StatusClass string`, `StatusBasis string` (the document's `status_basis`), `Current bool`, `AsOf *Date`, `EffectiveFrom,EffectiveTo *Date`, `Basis string` (`source_status` or `document_dates`)                                                                                                                                                                                                                                              |
| `Relation`    | Source/target keys and numbers, nullable anchors, kind, raw label, evidence, evidence SHA-256, completeness flag, method, resolved flags                                                                                                                                                                                                                                                                                                       |
| `Gap`         | `Kind,DocumentKey,Anchor,Message string`                                                                                                                                                                                                                                                                                                                                                                                                       |
| `Response`    | `Release string`, `Hits,NonCurrent []Hit`, `Abstain bool`, `Reason,Message string`, `Gaps []Gap`, `Diagnostics Diagnostics`                                                                                                                                                                                                                                                                                                                    |
| `Diagnostics` | Arm counts, eligible count, raw ANN count, fallback flag, per-operation milliseconds, query recipe hash; no query text                                                                                                                                                                                                                                                                                                                         |
| `Arms`        | Release/recipe hashes, route, depths, validation outcome, ordered dense/lexical `[]Candidate`, hydrated grouping/evidence metadata, exact member cosines, validity groups, diagnostics                                                                                                                                                                                                                                                         |
| `Candidate`   | Chunk id/ordinal, owning identity, group identity, rank, cosine, BM25 score, fragments and grouping metadata                                                                                                                                                                                                                                                                                                                                   |

`Recipe` is the versioned JSON below; `Release` mirrors the schema's release
metadata. `Embedder` uses the protocol types of root package
`internal/embedding`: `Recipe(ctx) (embedding.Recipe, error)` and
`Embed(ctx, text) (embedding.Vector, error)`; `DenseRuntime` and `Vector` above
name those types. `Prepare` validates and embeds once and returns
`Prepared{Request, Validation, Vector}`; `PrepareLegacy` skips only the
diacritics gate for trusted evaluation. Callers may reuse one `Prepared` across
limits, modes and arm controls and may export its vector. `Collect` runs arm SQL
and bounded hydration for a `Prepared`, never embedding again. `Rank` fuses,
groups and caps without a threshold and returns the ordered groups with
`FirstBestCosine`; build calibration uses it on a draft engine. `Shape` is
`Rank` plus abstention and evidence over a sufficient cache. If the requested
shaping needs uncached members or deeper pools, fail with `cache_insufficient`,
never silently approximate. Dense-only and lexical-only diagnostics still embed
once to compute the same abstention statistic. Cache collection hydrates the
whole requested arm union as [eval](evaluation.md#cache-and-report-format)
specifies; the smaller fusion hydration window applies only during `Shape`.
`Search` collects recipe depths then shapes with `Request.Arms`; `Collect`
always caches both arms so controls reuse the same candidates. Single-arm
shaping removes the other arm before fusion and hydration, not after ranking.

## Validation and routing

Validate UTF-8, nonempty trimmed input, Unicode rune count, NUL absence and
request options before database work. Limit zero selects the recipe default;
negative or above-maximum limits fail. Preserve accepted query bytes for the
dense model; validation and pattern recognition use a separate NFC view. The
service counts actual model tokens before inference. Unknown mode, invalid date
or limit outside budgets returns typed `invalid_request`; token overflow returns
`query_too_long`. Do not truncate queries.

A Vietnamese mark is `đ`/`Đ`, or a Latin vowel in NFC whose NFD decomposition
contains Vietnamese breve, circumflex, horn, acute, grave, hook, tilde or dot
below. This is a deterministic input gate, not a language detector. Queries with
no such mark and no valid document reference return an empty response,
`abstain:true`, reason `diacritics_required`, message
`Vui lòng nhập câu hỏi tiếng Việt có dấu hoặc số hiệu văn bản.` Check first for
no Latin letters and no valid reference: that returns the same empty abstaining
shape with reason `unsupported_query`. Foreign text that passes the mark gate is
left to the evidence threshold; do not claim reliable semantic language
recognition from a character test.

Recognize a bounded document token at start/end or boundaries that are not
Unicode letters/digits. Grammar: 1 to 4 ASCII digits, `/`, optional four-digit
year and `/`, then a nonempty sequence of Latin letters/digits beginning with a
Latin letter, optionally hyphen-separated segments of Latin letters/digits.
Allow whitespace around separators only. This accepts `50/2024/TT-NHNN` and
`21/VBHN-NHNN`, but not bare `2024`, `50/2024`, partial words or SQL wildcards.
Normalize exactly as [build](normalization.md). Bound token length and reference
count by budgets. Exceeding those limits returns `invalid_request`.

Validated grammar selects routed lexical weight, even if the number is unknown
or ambiguous in this corpus; there is no document lookup needed for routing.
Whole-country eligibility and current-law defaults remain unchanged. Unknown
references may add a coverage gap through exact number lookup, but do not
silently alter either arm or translate the input.

## Candidate SQL and ranking

Both arms use identical document eligibility. Exact dense mode evaluates every
eligible chunk's cosine distance, ordered by distance then chunk id, with no
pre-limit. On one read-only transaction set local indexscan and bitmapscan off
for exact queries; check EXPLAIN in integration evidence to exclude HNSW. ANN
uses a materialized pool over all chunks ordered by `embedding <=> $1`, then
filters eligibility and orders by distance/id outside the pool. Pool size,
ef_search and strict iterative settings are in budgets. Use transaction-local
settings on the same checked-out connection, including force_custom_plan;
rollback restores them before returning the connection. Unsupported pgvector
settings are errors, not a silent older-version fallback.

If eligible ANN results are below requested dense depth, run exact over the full
eligible set; report the raw ANN list and fallback separately. A full ANN pool
is not proof of recall. Sparse SQL scans positive inner products exactly:
`-(sparse <#> $1::sparsevec)`, descending score then chunk id. No sparse HNSW.
Bind vectors, dates, limits and status values; schema/table names are fixed.
Statement deadlines and context cancellation bound every scan. Do not log SQL
arguments, query text, vectors or returned evidence, including on database
errors.

One-based arm ranks feed
`dense_weight/(k+dense_rank) + lexical_weight/(k+lexical_rank)`; an absent arm
contributes zero. Fuse by chunk id; sort decreasing score, then increasing chunk
id. Limit hydration to the recipe multiplier times requested response limit,
bounded by available union. Group each point or deeper descendant under its
nearest `khoan` ancestor; otherwise use its owning provision. Article-owned
chunks stay articles, and non-article chunks stay their owner. Use actual parent
links, not labels. Group score is maximum member fused score, never a sum.
Retain the best member tie key and all included member identities for
provenance.

Compute exact cosine for every chunk mapped to each hydrated group, including
members absent from the dense pool and lexical-only hits. Group best cosine is
the maximum over those members; an empty group is invalid. The first fused
group's best cosine is the abstention statistic before document capping. Record
it explicitly; do not substitute RRF score or the cosine of another group. Clamp
finite cosine values to [-1,1] in both calibration and serving; reject
non-finite values as infrastructure failures.

Consolidation collapse is `off-unverified-alignment` in the current recipe.
Imported `consolidates` edges support family diagnostics, but do not establish
provision equivalence or renumbering. The historical family+label collapse could
discard different amendment articles. Keep both until a later adjudicated
mapping supports collapse. Caching family components for such trials is deferred
to the tuning plan, which derives them from the immutable dump.

Recipe 2 raises the per-document cap from 3 to 5: on run vn/20261111_4 cap 5
took old product recall@20 from 80 to 82 and development from 68 to 75 of 113
with MRR@8 up on both cohorts and compact p95 at 20 hits down; no cap gained 3
more development citations but lowered development MRR@8, so it stays unadopted.
Apply the per-document cap in rank order: take capped hits while collecting
excess hits into a demoted list; if fewer than `limit` remain, append demoted
hits in original order until full. This is a diversity preference, not a hard
loss of evidence. Do not aggregate articles, promote sibling counts or append
unbounded related hits. Use the same processing for dense and lexical controls.

## Validity and evidence

Documents of class `relation_context` are not candidates in either arm unless a
validated number reference in the query equals the document's normalized number;
then they compete like any other document. They stay in the corpus as resolved
relation targets with metadata, text and validity. The recipe field is
`context_scope:"number-routed/1"`. Reason: on run `vn/20261110_4` they took 82
of 736 legacy hit slots at limit 8, matched none of the 96 expected citations,
and three out-of-scope negatives found a context document as their best hit,
which raised the abstention threshold from 0.4812 to 0.5595 and made 14 answered
positives abstain. One-hop context covers a topic by accident, so serving it as
primary evidence misleads on topics the scope never claimed.

Without explicit `as_of`, primary eligibility requires stored `is_current` and
the known date interval containing the caller's civil date in Vietnam. The
engine clock is injected; eval freezes that date. Missing start dates pass the
date test with a gap. Unknown status is not current. A document with
`validity_conflict` is not eligible in default mode and appears only in the
non-current group, marked unresolved; with an explicit `as_of` the dated
determination ignores the flag and the hit still carries it. Partial status is
current at document level only, with targeted repeal relations as warnings.

Set `Validity.Current` to that status/date eligibility result in this default
mode, including eligible missing-start cases with their gap. With an explicit
historical date, use the dated determination below instead.

Copy the document's `status_basis` into `Validity.StatusBasis` in both modes,
including `derived_from_consolidated` when [build](normalization.md) derived its
status. `Basis` continues to distinguish default status/date filtering from
explicit historical date filtering; `StatusBasis` separately exposes the origin
of the status. Return derivation gaps with the hit. Derived status is not
independently adjudicated legal effect.

With `as_of`, primary eligibility uses document dates only, independent of the
latest source status. Missing start dates remain in primary results with a gap,
but `Validity.Current` is false when the date test is indeterminate. A known
start and no known end is an open interval, not a missing-end gap. Every such
response says historical filtering uses document dates, not reconstructed
provision history or past suspension status.

`in_force_only` defaults true. In that default mode the complement (expired and
not-yet documents) still runs the same arms as a separate group with its own
depths and shaping, and the response reserves `non_current_tail` slots (2 in
recipe 2, [budgets](budgets.md#retrieval)) for it: primary hits are shaped to
`limit` minus the effective tail, then up to that many complement hits follow in
`NonCurrent`, marked non-current with their status; when the complement has
fewer hits, primary keeps the freed slots. The effective tail is the smaller of
`non_current_tail` and half of `limit` rounded down, so primary always keeps at
least half of the slots (limit 1: no tail; limit 2 or 3: one; limit 4 and up:
two). Abstention and confidence are judged on the primary group alone, as
before; complement hits never make a response answer. Not-yet documents never
enter the primary group, since current-law precision counts every primary hit.
With `in_force_only` false the complement competes for the remaining response
slots after primary, as before; on this corpus primary fills the limit, so that
request changes nothing today (backlog: reserve slots when the caller asks
explicitly). Primary failures are not rescued by the complement: no primary hits
means abstain. Evidence size includes both groups. `as_of` never silently swaps
expired and primary law based on score. Measured on run vn/20261111_4 (cap 5
with tail 0, 2, 3, 4): old product recall@20 82, 85, 85, 86 and recall@8 77, 77,
75, 73; development recall@20 75, 83, 85, 85 and recall@8 73, 76, 72, 67;
against cap 5 alone, tail 2 raises recall@20 on both cohorts and recall@8 and
MRR@8 on development, and leaves old product recall@8 and MRR@8 unchanged;
against the cap-3 baseline every one of those numbers rises.

Build `Citation` from the group's actual ancestry: join nonempty article, clause
and point labels in ancestor order with a comma and a space; otherwise use its
own label. The number is a separate field. Never derive a citation from an
anchor suffix or infer absent legal levels.

Hit text is the substantive lead spans of the group's ancestors that the chunk
recipe carried (role `lead`, ancestor order), followed by the group's own
representation and identified descendants, using
[chunk rendering](chunk.md#tree-rendering), never by joining prefixes or
guessing text from lexical tokens. Return the full text when within the evidence
cap. Above it, keep the leads first, then the best matched member's body spans
up to the cap, with exact offsets and `Complete:false`, `NextPart:1` for a
document read. A cut or missing lead makes the hit incomplete, so a clause never
looks complete without the article condition that qualifies it. Mark any cut
span incomplete. Each fragment carries `Complete`, true only when it holds every
covered source interval of its identity; child credit requires a complete child
fragment, not a coincidentally repeated sentence or shared label. Fragments
reference slices of `Hit.Text`; they do not send duplicate text. Source/content
aliases follow the chunk span contract. For ambiguous parent containment, return
its own text and an ambiguity gap without child credit. Do not attach a second
full enclosing article to every hit.

Attach incident relations whose provision endpoint is null, the group itself, or
its descendant. Preserve source direction; sort by kind, source key, target key,
artifact hash and ordinal. Apply the relation count/text caps in budgets, mark
omissions with `relations_omitted` and a document-read pointer, and mark cut
evidence incomplete. `EvidenceSHA256` hashes the full stored UTF-8 evidence;
product relation credit requires complete returned evidence and a mapped edge.
Deduplicate edges by source key, artifact hash and ordinal per hit, not across
different hits; two documents can have byte-identical relation artifacts. The
response is bounded and its actual evidence size is scored.

Abstain on validation rejection, no eligible primary hits, or first-group best
cosine strictly below the stored threshold. Equality passes. Low-confidence
responses retain their hits and gaps, as evidence agents may inspect; metrics
still score those returned hits and the abstention decision separately.
Infrastructure failure returns an error, never a successful abstention.
Threshold provenance and provisional status follow [evaluation](calibration.md).

## Serialized recipe and checks

The serialized recipe, its hash and its checks are specified in
[serialized recipe](retrieval-recipe-serialized.md).
