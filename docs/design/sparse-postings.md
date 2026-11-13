# Sparse postings

Proposed. The server answers the sparse arm from postings in memory instead of
scanning every chunk in SQL, and returns exactly the rows the SQL arm returns.
The arm costs about 270 ms per scan on the host, twice per search
([retrieval performance](retrieval-performance.md#sparse-arm)); the SQL arm
stays the reference and the fallback. Ranking rules are in
[retrieval recipe](retrieval-recipe.md#candidate-sql-and-ranking); this note
changes no recipe.

## The score to reproduce

The SQL arm scores a chunk as `-(c.sparse <#> q)` and keeps scores above 0,
ordered by score descending, then chunk id. pgvector 0.8.7 computes the inner
product by walking the two index lists in ascending order and adding
`ax[i] * bx[j]` for each shared index into a float32 accumulator, then widens it
to float64
([sparsevec.c](https://github.com/pgvector/pgvector/blob/v0.8.7/src/sparsevec.c)).
Every query weight is 1 ([lexical](lexical.md); `lexical.Query`), so each
product equals the chunk weight exactly and contraction into a fused
multiply-add cannot change a sum. The memory score is therefore the float32 sum
of the chunk's weights at the query's dimensions, added in ascending dimension
order, widened to float64. A query with any weight other than 1 uses the SQL
arm.

## Data in memory

For the active release only, the server holds:

- postings by dimension, in ascending dimension order: for each dimension the
  chunk positions and float32 weights, in ascending chunk id;
- per chunk position: chunk id and document position;
- per document position: the fields both arms filter on (`is_current`,
  `validity_conflict`, `eff_from`, `eff_to`, `class`, `number_norm`,
  `status_class`).

For vn/20261112_2 that is about 16 million postings (8 bytes each) plus 221,114
chunks and 3,842 documents: about 130 MB, in the server row of
[host budgets](budgets-host.md#host-memory-estimates). A retired or pinned older
release never gets postings.

## Per request

Scores accumulate dimension by dimension, in ascending dimension order, into a
per-request float32 array indexed by chunk position (reused from a pool), with a
list of touched positions. Each touched chunk with a score above 0 goes to the
current or non-current group by the same eligibility as the SQL arm, evaluated
on the request's resolved collection date, `as_of`, the named references and,
for evaluation, the not-yet trial. Each group keeps its top `lexical_depth`
candidates by score descending, then chunk id. The result fills
`ArmGroup.Lexical` exactly as `lexicalQuery` does, inside the same collection;
dense, hydration and the transaction are unchanged.

## Loading and the gate

When the resolver opens the engine for the active release, the engine keeps
serving the SQL arm and starts a background load bound to the engine's life: it
reads `chunk (id, document_id, sparse)` and the document fields in one read-only
snapshot, then builds the postings. Closing the engine cancels the load before
its pool closes.

Before the postings serve a request, the engine checks them against the database
it serves, on the production pgvector binary:

1. For 20 probe queries built from the release itself (the titles of the 20
   documents with the smallest ids), it compares the memory score of every chunk
   with a positive SQL score, bit for bit as float64, and requires the same set
   of chunks above 0.
2. For the same probes, on today's date and on one `as_of` date, it compares
   both groups' candidate lists with `lexicalQuery`.

Any difference, error or timeout keeps the SQL arm for the life of the engine
and records the outcome in the server log and metrics (no query text). A load
that has not finished also leaves the SQL arm in place. Requests never wait for
the load.

## Checks

The primary runs these; they are requirements, not results.

- Unit: the score order and float32 accumulation match a reference built from
  pgvector's algorithm, including ties, a zero score, duplicate dimensions
  merged by the encoder and a non-1 weight (SQL arm used).
- Integration: on a fixture release, both groups equal `lexicalQuery` for
  default, `as_of`, named relation_context references and not-yet collection;
  the gate rejects a deliberately altered weight.
- On vn/20261112_2: full search responses and arms byte-identical to the SQL arm
  for the comparison set used for the dense pool change (357 queries, `as_of`
  and not-yet variants), and `auto_explain` showing no sparse scan.
- On the host after deploy: the gate's outcome in the log, server memory against
  the budget row, and search p95.
