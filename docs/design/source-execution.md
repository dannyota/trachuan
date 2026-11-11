# Source execution contracts

Proposed shared prerequisite for [source scope](sources.md). Implement these
interfaces, artifact envelopes and fenced transactions before source-specific
stages. [Data](data.md#scope-workshop-contracts) owns persisted fields;
[index](index.md#build-stage) owns inventory hashes and compatibility. This
keeps adapters from inventing competing membership and failure contracts.

## Lookup and search results

Both clients expose `LookupNumber(ctx, number) (LookupResult, error)`; the VBPL
central listing and chinhphu search return the same envelope. `LookupResult`
contains `items` and `evidence`, even for zero results or partial results
returned with an error. The envelope has `version:1`. Items carry source, source
id, raw number, title, issuer, issue date and the page evidence reference.
Missing metadata stays explicit. VBPL listing items also retain `agencyIds` for
local selection. Keep the full listing evidence and record matching rules
separately from the query identity; local filtering never changes the reported
total or completeness.

Evidence contains `query_identity`, the canonical query definition,
`pages_fetched`, `total_reported` and `completeness` (`complete`, `incomplete`).
Each page records ordinal/cursor, reported total when available, result ids,
redacted response SHA-256 and artifact-relative response path. The overall total
is nullable when the source supplies no verifiable total. A page cursor is a
nonsecret locator; session tokens are never evidence values.

Mark complete only when every page was read successfully, pagination ended
normally, page totals agree and the distinct returned-id count equals the
reported total. Unknown totals, truncation, duplicate-only page loops, parser
uncertainty, challenges and failed pages are incomplete. Do not infer
completeness from an empty slice or an exhausted budget. Empty results prove
absence only when complete, with a recorded empty response and reported total
zero.

For chinhphu, request `drdRecordPerPage` at the cap in
[budgets](budgets.md#pipeline). Page info `a - b | N` reports the lesser of the
match count and that cap. Completeness additionally requires `N` below the cap;
equality is incomplete even after every page is read. Do not narrow capped
queries in this work. `Không có thông tin văn bản!` with blank page info is the
recorded zero total. A detail response with `Không tìm thấy văn bản này` is not
found even at HTTP 200, not a transport failure. Stale postback tokens are not a
failure signal; validate the returned page and pagination evidence.

This is per-observation completeness; the stable-listing pass rule in
[pipeline](pipeline.md#source-ingest-rules) still applies. Persist the evidence
envelope with discover output before resolution. Reused artifacts reproduce the
same completeness and gaps. Hash only redacted response bytes; keep tokens in
memory. A network error is not a replacement for incomplete-result evidence. An
incomplete result without a transport failure resolves as `listing_unstable`;
transport failures retain their structured cause and retry outcome.

## Shared membership and artifacts

Replace VBPL-specific expected entries with `MembershipEntry`: document key,
source, source id, class, disposition and all typed support tuples defined in
[data](data.md#scope-workshop-contracts). Shared readers take run and phase
(`seeds`, `expanded`, `final`) and return sorted entries plus the phase hash.
They expose active and retained-removal views explicitly, never derive
membership from fetch items. Planning and finalization use transaction-bound
readers under the run lock.

Discovery close records the chosen
[membership predecessor](membership.md#membership-predecessor) run id, phase and
snapshot hash in `run_scope`, atomically with freezing seeds. Expand and fetch
close read that record and verify the hash before using earlier membership. Once
seeds are frozen, no finalizer reselects the predecessor, including when the
recorded choice is none. The required workshop schema addition lives in
[data](data.md#scope-workshop-contracts).

Discover emits lookup envelopes; expand emits the
[seed observation](data.md#expand-observation-artifact) envelope. Fetch uses the
[source-specific bundle formats](fetch-bundles.md#bundle-formats): retain VBPL
files and use version 2 envelopes for chinhphu. `Rows` decodes the common
content projection only from verified bundle files. Item commit separately
persists [fetch observations](fetch-bundles.md#observation-commit-and-recovery),
including when an artifact already exists. `Resolve` writes run membership
evidence and gaps. No artifact embeds observation times, final class or
precedence decisions.

Source-independent document and file projections, structured failure causes,
membership readers and finalization are mandatory workshop migrations and shared
interfaces. Preserve old artifacts; decode their known formats into the common
projection with unknown provenance explicit. Do not silently treat old rows as
complete evidence or fill chinhphu ids into `vbpl_id`.

## Stage close

Add an optional stage interface
`Finalize(ctx, env, tx, run) (Finalization, error)` to the existing `CloseStage`
control activity. `Finalization` carries a durable failure code, reason and
evidence references, or success. Stages without it retain current close
behavior. The control activity owns the transaction; finalizers read artifacts
only through `env.Artifacts` and access the database only through `tx`, never
`env.DB`. They never commit, perform network requests or mutate immutable
artifacts. Discover and expand use the same hook to publish their seed and
expanded membership snapshot references before advancing. Build uses the final
snapshot; drift checks always use the earlier frozen expanded active snapshot.

After all batches return, lock the run row and verify generation. A cancelling
or cancelled run returns cancellation without invoking the finalizer. A recorded
blocking run failure returns its committed failure without invoking it again.
Subject to the [source pause](#source-pause) barrier, a stage that already
advanced returns its committed result. Otherwise verify stage identity and
require no pending or failed items before invoking the finalizer.

Expand has no close input hash. Repeating its finalizer must reproduce the
frozen expanded snapshot bytes, membership evidence and seed observations
exactly; any difference errors. Item-state changes alone do not change a frozen
result when those outputs remain identical.

## Fetch-close finalization

Fetch finalization:

1. Reads committed fetch outputs, lookup evidence and the expanded active
   membership snapshot. For drift, verify the artifact manifest hash and the
   hashes of the files read (`detail.json` and `diagram.json`), without
   rereading every attachment. This is not a fresh attachment-integrity audit.
2. Checks each primary seed's qualified targets for
   [drift](membership.md#fetch-drift). A drift commits
   `run_failure(scope_changed)`, marks the run failed and leaves `next_stage` at
   fetch; it does not publish final membership or advance.
3. Otherwise applies the hand-over table, updates final membership and writes
   replacement/ambiguity gaps on the secondary fetch item, including skipped
   secondary candidates. It never adds an id outside the planned superset. Read
   identity through the verified
   [bundle hand-over reader](fetch-bundles.md#hand-over-reader), not workshop
   latest rows; skipped candidates use their committed discovery evidence.
4. Stores the final membership hash and input hash, recomputes stage/gap counts
   and advances `next_stage` in that same transaction.

The fetch plan includes one item for every secondary candidate, including
precedence skips, so finalization always has a gap owner. Downloading an
already-proven replacement is unnecessary. Downstream stages read only final
active membership; build also reads excluded membership and gaps for audit.

With selection version 1 and no `expand` in the frozen stage list, final
membership comes from the seeds phase; no expansion drift check runs. Do not
insert a stage into that frozen run. Otherwise use the expanded phase.

The fetch-close input hash covers the selected phase hash (`expanded_hash`, also
used for the seeds fallback) and every fetch item's key, state and artifact
hash, including a null hash for an item without an artifact. Serialize items in
key order. The phase hash binds membership evidence; artifact hashes bind the
comparison bytes. Repeating finalization with the same input is idempotent.
Changed input after finalization is an error unless an authorized retry first
invalidated that finalization. An infrastructure error rolls back; a domain
failure must commit its reason before returning `Failed`. The drift policy owns
the restrictions on a failed run. Reason: fetch completion and membership
publication must be atomic and fenced.

## Source pause

The first recognized challenge or HTTP 403 atomically records
`run_source.state=source_paused` for that run/source, with reason and redacted
evidence. It is a persistent source-wide state, not an item retry or a gap.
Every later item for that source fails fast with `source_paused`. Compound items
also stop before any further request to the paused source.

All clients, batches and workers use a shared request-admission gate backed by
the workshop row. It serializes request admission with pause updates, checks
state before every attempt, retry and redirect. The gate owns admission and
pause only; clients enforce [request limits](sources.md#fetch-and-safety).
Already admitted requests may finish; they cannot admit a follow-up request.
Database failure fails admission closed; process-local caches cannot authorize
requests.

Every item output lists all sources it contacted or depends on. Item commit
first applies the [terminal replay fence](pipeline.md#items-artifacts-and-gaps).
For a new committed result, recheck those `run_source` rows under `FOR SHARE`
locks. If any source is paused, commit a failed `source_paused` item without its
artifact, derived rows, gaps or observations. PostgreSQL's
[row locks](https://www.postgresql.org/docs/current/explicit-locking.html#LOCKING-ROWS)
hold concurrent updates to those rows until the transaction ends.

Stage close rechecks source state under the run lock. A paused source marks the
run failed with a `source_paused` message, without invoking the finalizer or
advancing. It writes no `run_failure` row: the durable pause lives in
`run_source`. A paused source also blocks release.

Worker startup, `corpus start` and `corpus recover` read and preserve the pause.
Only `corpus source-resume <cc> <rel> --source <name> --note <reason>` clears
it. The command requires no open workflow and an unreleased run, locks the run
and source rows, and appends a clear event. It sends no request and does not
start work; the owner then runs `corpus start`. Resume resets `source_paused`
failed items through the normal failed-item path, retaining completed evidence.
Reason: concurrent batches and restarts must not bypass an access denial.

## Retry source gaps

On an otherwise permitted `corpus start`, inspect all stages for items with a
nonempty gap set consisting only of `source_unavailable` or gateway-caused
`invalid_source_data`. Include done items carrying gaps and accepted gaps. Use
the structured causes in data, never message matching. Malformed legal data,
duplicate ids and database text errors do not qualify. A `source_unavailable`
gap is retry-eligible whatever its origin, as [data](data.md) states; a
historical `invalid_source_data` gap without origin evidence stays unchanged
pending classification from evidence.

In the generation-increment transaction, delete selected gaps and item-owned
derived rows, clear artifact/input references and attempt state, and set items
pending. Rewind to the earliest affected stage, including an empty next stage;
validate `--until` after choosing it.

Clear later stage run items, derived rows and gaps so they replan.

Replanning does not mean reading the source again. Run `vn/20261111_1` showed
the cost: eight expand items fail with HTTP 500 on every pass, so each restart
rewound to expand and fetch read about 3,800 documents again for two hours,
although the run already held them. A run reads a document from the source at
most once:

- When a fetch or expand item commits a bundle it read from the source, the run
  records an acquisition: run, stage, document key and the bundle artifact. The
  record is not an item, carries no gap and no membership decision, and a
  restart never deletes it.
- When a later generation of the same run processes an item for a document key
  that has an acquisition in that stage, it adopts the recorded artifact instead
  of calling the source: the item commits with that artifact's manifest hash as
  its input hash, exactly as if it had just downloaded it. Adoption writes no
  source observation and leaves `fetched_file.fetched_at` unchanged; it is not a
  download.
- Everything that depends on membership is computed again as today: planned
  skips (`withdrawn`, `scope_removed`), hand-over decisions and other finalizer
  gaps, relate's target resolution, and every later item. Their artifacts are
  reused by input hash where the inputs are unchanged, so extract, chunk and
  embed do no new work for an unchanged bundle.
- An item that ended as a gap has no acquisition and is read from the source
  again, which is what a retry is for.

Acquisitions belong to one run and its frozen recipes; another run reads the
source itself, as [sources](sources.md) requires. The run message records how
many items adopted an acquisition.

Delete all membership evidence and seed observations produced at or after the
rewind stage, including evidence from unselected items at that stage. Every
`relation` tuple counts as produced by expand, regardless of its attributed
stage/item. Then delete run documents left without evidence and clear affected
phase snapshots and hashes. Clear the fetch-close input hash only when the final
phase is invalidated. Clear the predecessor record with the seeds phase;
discovery close then selects and freezes it again. Evidence from earlier stages
survives. Finalizers rebuild the cleared phases from remaining artifacts and
earlier-run membership under the [loss rules](membership.md). Keep immutable
artifacts for reuse. Record counts and rewind stage in the run message, and for
each later stage the number of items kept, reset, deleted and added; recomputed
gaps require fresh acceptance.

Do not clear source pauses or durable run failures. Still refuse changed frozen
recipes, open workflows, evaluated/released runs or stage insertion into an old
snapshot. The generation/attempt fence applies to reset and every later commit.
Reason: retry transport failures without preserving stale downstream results,
and without repeating downstream work whose inputs did not change.

A bounded non-JSON 200 from a JSON gateway endpoint, including raw tree, is
retryable before source decoding. After bounded retries emit
`source_unavailable` with structured endpoint, status and cause, not body text.
Oversize responses stay `response_too_large`; valid JSON with invalid fields
stays `invalid_source_data`. Recognized challenges take the source-pause path
first. Reason: a gateway error page is not a permanent legal-data defect.

## Required evidence

The primary must verify incomplete empty and truncated lookups, restoration of
shared projections from artifacts, stale-generation finalization, crash before
and after close commit, and retry invalidation. Also cover drift to retained
removal keys and rejected acceptance; conflicting/missing hand-over fields;
unrelated missing context metadata; concurrent source pause, restart and
explicit clear. Use injected clocks, transport, DNS and reduced limits. These
are acceptance requirements, not claims that checks have run.

Stage commit package tests must cover identical terminal same-attempt replay,
changed artifact hash, changed resource evidence (including timestamps), and
both replay cases after a later source pause. Prove no writes on a no-op, full
rollback on conflict, and no observations when a pause converts a new result to
failed `source_paused`. Cover each terminal state and stale fences.

Also cover local selection from the recorded central listing, equal numbers with
different or missing issue dates, capped chinhphu totals, the empty-result
marker and HTTP 200 missing details under the source-specific rules above.

Membership evidence must cover per-rule incompleteness, named ambiguity, first
catalog absence, supersession across repeated runs and discovery-attributed
relation removal during an expand rewind. Verify incomplete expansion followed
by fetch with an unplanned target still fails, while targets already active do
not cause drift. Verify unaccepted removal carries across runs, acceptance ends
the carry, incomplete observations do not revive removals, and readmission
starts a new removal event on later loss. Earlier audit and acceptance records
must remain unchanged. Verify final-only predecessor selection, legacy fallback,
recorded absence, hash verification and invalidation with the seeds phase.
