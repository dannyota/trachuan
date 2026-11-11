# Retry of source gaps

This note owns what `corpus start` does to a run that ended with source gaps:
which items are retried, what is rewound, and what a restart reuses.
[Source execution](source-execution.md) owns requests, limits and pauses;
[membership](membership.md) owns the phases a rewind clears.

## Selection and rewind

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

## What a rewind clears

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

## Acquisitions

Replanning does not mean reading the source again. Run `vn/20261111_1` showed
the cost: eight expand items fail with HTTP 500 on every pass, so each restart
rewound to expand and fetch read about 3,800 documents again for two hours,
although the run already held them. A run reads a document from the source at
most once:

- Acquisitions live in a workshop table keyed by run, stage and document key. A
  row references the bundle artifact on its own, not through `run_item`, and
  records the generation that acquired it. It is written in the same transaction
  as the successful done-item commit of a fetch or expand item that read the
  source, after the generation, attempt and source-pause checks. A result that
  never committed a bundle (a gap or a failure) creates none; a committed bundle
  that carries attachment gaps such as `missing_attachment` is an acquisition,
  and those gaps are produced again on adoption. Restart and recover never
  delete acquisitions.
- When a later generation of the same run processes an item for a document key
  that has an acquisition in that stage, and the item is not a planned skip, it
  adopts the recorded artifact instead of calling the source. Before adoption
  the whole bundle is verified against the recorded manifest hash, the stage,
  the frozen recipe and the document identity. The item then commits through the
  normal path (rows, resolution, fencing, source-pause checks) with that
  manifest hash as its input hash and the existing artifact row, exactly as if
  it had just downloaded it.
- A missing or invalid bundle is logged with run, stage, key and reason and the
  item reads the source; a successful read replaces the acquisition reference in
  the same commit. Storage that cannot be read at all stays an infrastructure
  error, not a cache miss.
- Adoption is not a download: it creates no `run_fetch_observation` rows and
  leaves `fetched_file` rows, including their times, unchanged. The acquisition
  record identifies the reused bundle; the original resource observations remain
  as immutable history.
- Everything that depends on membership is computed again as today: planned
  skips (`withdrawn`, `scope_removed`), relation supports, seed observations,
  hand-over decisions and other finalizer gaps, the final membership, relate's
  target resolution, and every later item. Their artifacts are reused by input
  hash where the inputs are unchanged, so extract, chunk and embed do no new
  work for an unchanged bundle.
- Expand acquisitions matter only when a rewind to discover deletes and replans
  expand; an expand rewind keeps its unselected done items as today.
- Before the first retry invalidation under this rule, a run that has none yet
  gets acquisitions seeded from its own committed done fetch and expand items
  under their frozen recipes, so a run started by an older worker benefits on
  its next restart. Acquisitions are never inferred from directories without a
  committed item, and never from another run.
- The acquisition table travels in the workshop dump. A checkpoint taken before
  a bundle was recorded has no acquisition for it, even if the directory
  survived, and reads the source again.

Acquisitions belong to one run, one stage and its frozen recipes; another run
reads the source itself, as [sources](sources.md) requires, within the
[reuse rule for unchanged VBPL reads](fetch-reuse.md). This freezes each
successful read separately; it does not promise one source snapshot time for the
run. The run message records how many items adopted an acquisition.
