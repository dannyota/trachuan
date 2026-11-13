# Sparse postings

Proposed. The server answers the sparse arm from postings in memory instead of
scanning every chunk in SQL, and returns exactly the rows the SQL arm returns.
The arm costs about 270 ms per scan on the host, twice per search
([retrieval performance](retrieval-performance.md#sparse-arm)); the SQL arm
stays the reference and the fallback. Ranking rules are in
[retrieval recipe](retrieval-recipe.md#candidate-sql-and-ranking); this note
changes no recipe.

## Exactness without replaying pgvector's arithmetic

The SQL arm scores a chunk as `-(c.sparse <#> q)`, keeps scores above 0 and
orders by score descending, then chunk id. pgvector 0.8.7 adds the products of
shared indices into a float32 accumulator and widens it to float64
([sparsevec.c](https://github.com/pgvector/pgvector/blob/v0.8.7/src/sparsevec.c)),
but it is compiled with `-fassociative-math`
([Makefile](https://github.com/pgvector/pgvector/blob/v0.8.7/Makefile)), so the
production binary may add in another order. The postings therefore never decide
the final order; they only choose a candidate set that provably contains the SQL
answer, and SQL scores that set:

1. Memory scores are float32 sums of the chunk's weights at the query's
   dimensions. Every query weight is 1 (`lexical.Query`), so every product is
   exact.
2. Any two orders of adding n positive float32 numbers of total W differ by at
   most 2·γ(n−1)·W, with γ(k) = k·u/(1−k·u) and u = 2⁻²⁴. At load the engine
   computes δ, the largest such bound over all chunks (n is the chunk's nonzero
   count, W its weight sum in float64), times 1.01. For vn/20261112_2, n ≤ 164
   and W ≤ 654, so δ ≈ 0.013.
3. Per group, with m₅₀ the memory score at rank `lexical_depth` among eligible
   chunks with a positive score, the candidates are every eligible chunk with a
   positive memory score of at least m₅₀ − 2δ (all of them when fewer qualify).
   Every chunk in the SQL top `lexical_depth`, including every chunk tied with
   its last score, satisfies that bound.
4. In the collection's transaction,
   `SELECT id, -(sparse <#> $1::sparsevec) FROM chunk WHERE id = ANY($2)` scores
   the candidates of both groups; each group is ordered by that score
   descending, then chunk id, and cut to `lexical_depth`.

Positivity agrees exactly: every stored weight is positive, so a chunk scores
above 0 in either computation exactly when it shares a dimension with the query.
Eligibility is pure logic over stored fields (no arithmetic), evaluated as the
SQL expression does: the resolved collection date (including a cursor's pinned
date), `as_of`, current and conflict flags, start-inclusive and end-exclusive
dates with NULL open ends, `class` and exact `number_norm` membership (`C`
collation, empty list matches nothing), and the not-yet trial for evaluation. A
query with any weight other than 1 uses the SQL arm.

## Data in memory

For the active release only, the server holds:

- a directory of the dimensions present and, for each, the chunk positions and
  float32 weights in ascending chunk id;
- per chunk position: chunk id and document position;
- per document position: `is_current`, `validity_conflict`, `eff_from`,
  `eff_to`, `class`, `number_norm` and `status_class`;
- δ.

vn/20261112_2 has 16,203,127 postings (8 bytes each), 221,114 chunks and 3,842
documents: about 140 MB steady, in the server row of
[host budgets](budgets-host.md#host-memory-estimates). Requests reuse float32
score arrays of one entry per chunk (0.9 MB each) from a pool. Building needs
the flat triples (12 bytes per posting) before the postings exist, so the build
peak is about 340 MB; during a switch the retiring engine's postings stay until
its last lease ends, so the peak is about 480 MB, inside the activation headroom
row. Before building, the engine reads the chunk and posting counts and stays on
the SQL arm if the estimated peak exceeds 600 MB.

## Decoding

Rows are read as `sparse::text` (`{i:v,...}/d`, one-based indices, values
printed as the shortest decimal that round-trips) and parsed with
`strconv.ParseFloat(v, 32)`, which recovers the stored float32 exactly. The
one-based indices are the same numbers `sparseVector` writes for queries. Any
row with indices not strictly ascending, outside 1..d, or a value that is not
finite and positive disables the postings for the engine.

## Per request

Scores accumulate dimension by dimension into a pooled array indexed by chunk
position, with a list of touched positions. Each touched chunk goes to the
current or non-current group by eligibility; each group selects its candidates
as above; one SQL statement scores both groups' candidates; the result fills
`ArmGroup.Lexical` exactly as `lexicalQuery` does. Dense, hydration and the
transaction are unchanged.

## Lifecycle

- The engine owns a context created at `Open`, independent of the request and of
  the resolver's opening context, and a `Close` that cancels it and waits for
  the loader to return. The resolver's cache entry calls the engine's `Close`
  before closing its pool, on every path: open failure after the engine exists,
  identity mismatch, retirement and shutdown.
- After `Open` returns for the active release, the engine starts one loader
  goroutine. It reads chunks and documents in one `REPEATABLE READ READ ONLY`
  transaction (one snapshot), builds the postings, runs the smoke check below
  and publishes them through an atomic pointer only if the engine is not closed
  or retired; otherwise it drops them.
- Retirement cancels a running load. Published postings of a retired engine
  serve its remaining leases and are released at `Close`.
- Until publication, and forever after any failure, the engine serves the SQL
  arm. Requests never wait for the load. The outcome (ready, disabled and why,
  load seconds, bytes) goes to the server log and metrics, with no query text.

The smoke check runs 20 probe queries built from the release (titles of the 20
documents with the smallest ids) through both arms on today's date and one
`as_of` date and requires identical candidate lists. It guards wiring and
decoding; exactness rests on the candidate bound, not on the probes.

## Checks

The primary runs these; they are requirements, not results.

- Unit: candidate selection keeps every SQL top-`lexical_depth` chunk and every
  tie at the boundary when memory scores are perturbed by up to δ in either
  direction; decoding rejects unordered, out-of-range, zero, negative and
  non-finite entries; a non-1 query weight uses the SQL arm.
- Integration, each case asserting that postings are published and served the
  request (the engine's per-collection source counter): both groups equal
  `lexicalQuery` for default, cursor dates on both sides of midnight in Vietnam,
  `as_of`, NULL and boundary effective dates, conflict documents, named
  relation_context numbers (present, absent, empty list) and not-yet collection;
  a closed or retired engine never publishes; `Close` waits for the loader.
- On vn/20261112_2: full search responses and arms byte-identical to the SQL arm
  for the comparison set used for the dense pool change (357 queries, `as_of`
  and not-yet variants), with the source counter showing postings served them,
  and `auto_explain` showing no sparse scan; load time and peak memory measured.
- On the host after deploy: the published outcome in the log, server memory
  against the budget row, and search p95.
