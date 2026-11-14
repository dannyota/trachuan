# Retrieval performance

Proposed. Where a search, a filtered search, an MCP compact search and the topic
page spend their time, and one recommended change per cost. Recipe behaviour is
owned by [retrieval recipe](retrieval-recipe.md); a change that alters returned
candidates is a recipe change judged by
[ADR 0010](../adr/0010-retrieval-evaluation-for-agents.md) item 4 (and item 7
for ANN loss), a change that only reduces size by item 5. Everything else here
returns identical rows.

## Measurement

Production (trachuan.vn, A1 with 4 OCPU and 24 GB, `shared_buffers` 4 GiB, JIT
off, release vn/20261112_2 with 221,114 chunks, main at 22b617c6): unfiltered
web search 0.9 s cold; filtered search 1.55 s and 165 KB; MCP compact search
1.27 s and 77.7 KB for 18 hits; topic page `/vn/topics/1` 2.7 s; the sparse arm
about 270 ms per run. The search deadline is 2 s.

Local (2026-11-14): the real server and query embedder against the same release
on PostgreSQL 18.6 with pgvector 0.8.7, `shared_buffers` 1 GB, JIT off, the
MacBook at load average 11 to 19. `auto_explain` (`log_analyze`, `log_buffers`,
nested statements, parameters) logged every statement of 21 searches and 3 topic
requests in two runs. Times are noisy at that load and the smaller buffer cache
makes HNSW reads slower than on the host; block counts carry over. Per
statement, median and maximum milliseconds across runs, and median shared blocks
hit and read:

| Statement                           | Runs | p50 ms | max ms | Hit     | Read   |
| ----------------------------------- | ---- | ------ | ------ | ------- | ------ |
| Dense ANN pool, current arm         | 19   | 514    | 1,858  | 8,500   | 7,400  |
| Dense ANN pool, non-current arm     | 17   | 36     | 847    | 19,900  | 180    |
| Exact dense fallback, either arm    | 11   | 307    | 765    | 460,000 | 29,700 |
| Sparse arm, either arm              | 34   | 122    | 343    | 232,000 | 33,800 |
| Hydrate chunk rows                  | 31   | 15     | 31     | 4,500   | 55     |
| Relations of groups                 | 29   | 11     | 29     | 6,100   | 32     |
| Hydrate chunk-to-group mapping      | 31   | 9      | 25     | 1,700   | 140    |
| Effective taxonomy nodes            | 28   | 2.5    | 44     | 3,800   | 13     |
| Other statements, summed per search | 15   | 14     | 33     | 6,800   | 36     |
| Filter documents (filtered only)    | 5    | 0.6    | 1.9    | 85      | 11     |
| Topic documents                     | 3    | 324    | 468    | 59,000  | 19,000 |
| Taxonomy tree                       | 5    | 7      | 17     | 790     | 2      |

Each search runs the dense pool, the sparse scan and, when needed, the exact
fallback once per arm: current and non-current. Repeating a search right away
(same vector, warm pages) took the dense pool down to 29 to 68 ms, so its cold
cost is page reads; the sparse scan stayed at 120 to 130 ms per arm in three of
four repeats. For 7 of 15 distinct queries the pool of 1,200 nearest chunks held
fewer than 50 eligible ones for at least one arm, mostly the non-current arm, so
the exact fallback ran. Under load, 7 of 26 requests failed at the 2 s deadline
(503 on the web routes, `unavailable` on MCP), including the topic page twice.

What drives the blocks:

- Half the chunks (110,389) belong to `relation_context` documents, which both
  arms exclude unless the query names them, and about half the rest are not
  current. Most of the ANN pool is therefore ineligible.
- `embedding` (4,100 bytes) is always stored out of line, in a 1.16 GB TOAST
  table: the exact fallback touches about nine blocks per eligible chunk.
- `sparse` averages 601 bytes and never exceeds 163 non-zero elements, but
  42,882 rows (19%) also store it out of line because `content` and `spans` make
  the row wider than 2 KB. That is why one sparse scan touches about 215,000
  blocks while the heap has 44,000.
- The topic query's recursive step joins `provision` on `parent_id` alone, but
  every provision index leads with `document_id`, so each level hashes all
  305,906 provisions.

Expected order on the host, where pages stay cached: the exact fallback when it
runs (140 to 770 ms), the sparse arm (two scans, about 270 ms each), and the
topic documents query (200 to 470 ms, over 2 s under load). The dense pool comes
next (30 to 70 ms warm per arm, up to 1.9 s cold).

## Exact dense fallback

Options:

1. Partitioned ANN pools (recipe change). Build stores each chunk's static
   eligibility class: `current` (current, no validity conflict, not
   `relation_context`), `other` (not `relation_context`, not `current`) and
   `context`. Partial HNSW indexes cover `current` and `other`; `context` has
   none. The current arm takes its pool from the `current` index; the
   non-current arm from `other`, plus an exact pass over the few `current`
   documents outside the validity window of the resolved collection date (the
   request date, or the date a cursor pins); historical (`as_of`) searches merge
   both pools and apply date-only eligibility to both, conflict documents
   included; named context documents are scored exactly through
   `chunk(document_id, …)` and join their arm before its depth cap. Pools become
   almost all eligible, so the fallback becomes rare, and the indexed set falls
   from 221,114 to about 110,700 chunks (HNSW about 0.85 GB instead of 1.73 GB,
   a better fit in the buffer cache). Results change wherever ANN loss changes,
   so this is a recipe candidate measured under item 4, with ANN loss and cost
   reported under item 7.
2. Narrow vector storage (identical rows). Move `embedding` to its own table
   with `STORAGE PLAIN`, reached through the eligible documents. That replaces
   the nine TOAST blocks per chunk with one heap block, but the exact pass still
   reads 50,000 to 60,000 vectors. Expected 2× to 4× on the fallback only, an
   estimate not measured; it is a build schema change.
3. `halfvec` storage. It halves the bytes read but changes cosines, so it is a
   recipe change with a smaller gain than option 1.

Measured on vn/20261114_3 (2026-11-14, 243,647 chunks; the 357 queries of the
comparison set, each collection timed outside the server, warm pages): the pool
of 1,200 held fewer than 50 eligible chunks for one or both groups in 18 of 354
default searches (5%) and 10 of 90 `as_of` searches (11%). Each exact fallback
statement took 120 to 147 ms (p50 132, p95 147; 548 ms at p95 with cold pages),
about 250 ms per affected search; the shared pool statement took 32 ms at p50.
The fallbacks come from the artificial queries: 16 of 120 nine-word chunk
excerpts (often text that only `relation_context` documents contain) against 2
of 157 evaluation questions, 0 of 80 titles and 0 of 94 queries naming a number.
Filters, detail and limit do not change collection, so they do not change these
numbers. On questions like the evaluation set the fallback costs about 3 ms per
search on average.

Recommendation: defer option 1. It is a recipe change that needs an evaluated
run under item 4 and item 7, for about 3 ms per realistic search. Revisit when
production shows the fallback in more than 5% of searches, which needs the
fallback counted per engine and logged (backlog), or when the release grows
enough that the eligible share of the pool falls.

## Sparse arm

Options:

1. Keep `sparse` in the heap (identical rows). Build sets
   `ALTER TABLE chunk ALTER COLUMN sparse SET STORAGE MAIN`, so TOAST moves
   `content` and `spans` out first. Measured on a copy of the release's chunk
   columns: 0 of 221,114 vectors out of line instead of 42,882, and one arm scan
   took 66 to 75 ms instead of 95 to 111 ms (−30%). `MAIN` still allows out of
   line storage as a last resort, so build reports how many vectors stay out of
   line.
2. One scan for both arms in SQL. Measured on the same copy, a single scan that
   ranks each arm with `row_number()` took 181 to 321 ms, slower than two
   parallel top-N scans. Rejected.
3. Exact in-memory postings in the server (identical rows once verified). At
   activation the server loads every chunk's sparse vector into postings by term
   (about 16 million entries, about 130 MB) with each chunk's document; per
   request it accumulates scores for the query's terms, applies the same
   eligibility over the release's documents, picks a candidate set that provably
   contains each arm's top 50, and lets SQL `<#>` score only that set
   ([sparse postings](sparse-postings.md)). Expected under 10 ms for both arms
   instead of about 540 ms, an estimate to measure. Memory goes into the server
   row of [host budgets](budgets-host.md#host-memory-estimates).
4. HNSW on `sparsevec` (recipe change). Every chunk fits the 1,000 non-zero
   limit, but results are approximate, the same eligible-pool problem as dense
   applies, and the recipe says no sparse HNSW. Rejected.

Recommendation: option 3, which removes the cost; option 1 ships first because
it is one build line and helps until option 3 lands.

## Dense ANN pool

Both arms run the same pool query (same vector, pool size and transaction-local
settings) and differ only in the eligibility filter applied outside the pool.

Options:

1. Run the pool once per search and filter it for both arms (identical rows,
   because the strict iterative scan is deterministic for one index and vector).
   Saves one HNSW scan: 30 to 70 ms warm, up to about 1.8 s cold.
2. Prewarm at activation with `pg_prewarm` (contrib, in the official postgres
   image the pgvector image builds on) for the HNSW index, the chunk heap and
   its TOAST table, within the 4 GiB buffer cache. Expected to cut the cold
   first search (0.9 s on the host); prewarmed pages have no eviction
   protection, so activation records the prewarm time and the first-search
   latency ([pg_prewarm](https://www.postgresql.org/docs/18/pgprewarm.html)).

Recommendation: option 1, plus option 2 at activation (step 4 of
[deployment](deployment.md) already requires it; the manual
[host runbook](../runbooks/host.md) gains it); both return identical rows.
Partitioned pools (above) later shrink this cost again.

## Topic page

The documents query for node `1` takes 200 to 470 ms because of the `parent_id`
join. Carrying `document_id` through the recursion and joining with
`provision WHERE document_id = … AND parent_id = …` uses
`provision_document_id_parent_id_ordinal_idx`. Measured on the release: 8 to 11
ms instead of 250 to 330 ms; over all 1,211 served nodes the query output was
identical (5,378 rows) and took 2.7 s in total instead of 214.7 s.

Recommendation: that rewrite, and a cache of the node page per release and code
(the page depends on nothing else) under the [server caches](caching.md) rules.
The rest of the 2.7 s page time is outside these two statements and needs a
trace of the Nuxt page.

## Filter join

The arms join `chunk` to `document` by hash over the release's 3,842 documents
(1 to 12 ms inside each arm). Type and issuer filters run after collection,
through one `document` lookup by key (0.2 to 1.9 ms). Filtered searches were
slower only when the exact fallback ran. Pushing filters into the arms would
change which candidates fill the depth, a recipe change with no measured cost to
remove.

Recommendation: no change.

## Result caches

The search response cache (1 hour, keyed by release, Vietnam date and canonical
request) already serves web and MCP: a repeat took 3 to 6 ms. It misses whenever
a filter, detail level, limit or cursor changes, although collection does not
depend on filters, detail or limit.

Options:

1. Cache collected arms with their hydration. Measured on the 476 comparison
   records of vn/20261112_2, a serialized `Arms` is 13.7 MB at p50 and 125 MB at
   p95 (hydrated members carry provision text, ancestors, descendants and
   relations), so a useful cap would hold only a handful of entries. Rejected.
2. Cache only the candidate phase
   ([search candidates](caching.md#search-candidates)): per arm group, the
   ordered raw ANN, dense and lexical candidates with their scores and the
   fallback flag, about 10 KB per entry. A hit skips the dense pool, the exact
   fallback (140 to 770 ms when it runs) and the sparse arm; hydration, which
   read about 15 to 60 ms of immutable release rows in the measurements, runs as
   on a miss, so every later value is computed exactly as without the cache.
3. Longer response cache lifetime. It helps only identical repeats.

Recommendation: option 2, with the topic page cache above.

## Compact payload

A compact MCP result at 18 hits was 90 KB on the wire: the same JSON appears
once in `structuredContent` and once as text, as the
[MCP tools specification](https://modelcontextprotocol.io/specification/2025-06-18/server/tools#structured-content)
recommends ("a tool that returns structured content SHOULD also return the
serialized JSON in a TextContent block"). One copy of 45 KB holds the documents
(22 KB, of which the `related` pointers 9 KB and `related_counts` 1.7 KB),
snippets (10 KB), paths (5 KB) and the rest. Payload size does not explain the
latency, which is SQL.

Options:

1. Drop `related` from compact document entries and keep `related_counts`;
   `read` and full search still return the pointers. About −20% per copy.
2. Drop the text copy. It breaks clients that read only `content`. Rejected.
3. Shorten paths or snippets. Snippets are the evidence; path headings are small
   per hit. Not worth the loss.

Recommendation: option 1, which needs a revision of
[ADR 0013](../adr/0013-compact-search-and-batch-read.md) item 2 (bounded
pointers with counts) and matching [MCP results](mcp-results.md) and guide
changes first; it ships under item 5 and stays only if the agent-in-the-loop
check in [MCP results](mcp-results.md#evaluation) shows no fall in cited
provisions and lower total input tokens.

## Order and expected effect

1. Topic query rewrite and node page cache: page SQL from 200 to 470 ms to about
   10 ms, then a cache hit.
2. Dense pool once per search, prewarm at activation, `sparse` stored in the
   heap (build storage, needs the next build): about −100 to −250 ms per search,
   and a shorter cold first search.
3. In-memory sparse postings: about −500 ms per search on the host.
4. Candidate cache: a filter or detail change after a search skips both arms;
   hydration and shaping remain.
5. Compact `related` removal after the ADR 0013 revision: about −20% compact
   size.
6. Partitioned ANN pools: deferred until production shows the fallback in more
   than 5% of searches (measured 5% on artificial queries, about 1% on
   evaluation questions).

Items 1 to 4 return identical rows; item 5 changes the MCP contract; item 6
changes the recipe and ships only under ADR 0010.

## Checks

The primary runs these; they are requirements, not results.

- Every identical-row change compares the full search responses (both arms,
  `detail:"full"`) before and after on the evaluation cases and 200 sampled
  queries, byte for byte.
- The in-memory sparse arm runs the parity checks in
  [sparse postings](sparse-postings.md#checks).
- `auto_explain` over a fixed request set (unfiltered, filtered, MCP compact,
  topic page) before and after each change, reporting statement time and shared
  blocks; on the host after deploy, the same set through the public routes.
