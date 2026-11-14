# Server caches

The host has far more memory than the other services use (about 14 GB before
these caches, in [host budgets](budgets-host.md#host-memory-estimates)), so the
Go server keeps in-memory caches that remove repeated work from the read path.
The owner decided this on 2026-11-11 ("we have many RAM, so can delay can
cache"). Sizes are in host budgets; this note owns what is cached, the keys and
the rules.

## Rules for every cache

- Memory only, inside the `server` process. Nothing is written to disk, a
  database, logs, metrics labels or backups, and a restart empties every cache.
- Keys are SHA-256 digests of a canonical byte string, never the text itself. A
  cache never holds query text; values hold only vectors, responses the server
  would return anyway, or [search candidates](#search-candidates) (chunk ids,
  ranks, scores and fallback flags).
- Every key that depends on corpus content includes the release, so activating a
  release makes older entries unreachable; the server also drops them at once
  when the active database changes. Query vectors depend only on the query
  recipe and the text (see below).
- Each cache is a least-recently-used map with a byte cap and a maximum age. An
  entry's charged size is its key, its value and a fixed 128-byte overhead;
  entries are evicted least recently used first until the total is under the
  cap, and an entry older than its maximum age is a miss. A value larger than
  one sixteenth of its cache's cap is never cached: the request computes it as
  without a cache.
- A cache never adds work, waits or new failure modes to a miss. A miss computes
  exactly as the server does without a cache, with the same allocations,
  deadline and cancellation, and stores the value afterwards only if it fits.
  Concurrent misses on one key compute independently, each under its own
  deadline and cancellation, and each may store its value; there is no shared
  in-flight computation. Memory used while computing is therefore the same as
  without a cache and is bounded by the read path itself (per-account limits,
  admission, the read call cap and item limits); the cache adds at most its byte
  cap of retained values.
- A hit still counts against the caller's per-account limit, keeps the
  credential checks, and returns exactly the bytes a miss would have produced.
- Metrics report hits, misses, evictions and bytes per cache, with no key or
  account labels.

## Query vectors

Before calling the query embedder, the server looks up the vector by
`sha256(query recipe hash, 0x00, exact query text sent to the embedder)`. The
value is the float32 vector and its token count. A vector depends only on the
query recipe and the text, so the release is not part of this key, and a recipe
change makes old keys unreachable. This removes the embedding step (64 to 485 ms
on the A1 host) for repeated questions, including agents that retry or page
through the same query. Maximum age 24 hours.

## Search responses

The server caches the serialized `search` result (the `structuredContent` JSON)
under `sha256(release, civil date in Vietnam, canonical request JSON)`, where
the canonical request is the validated tool input with defaults filled in and
keys sorted. The civil date is part of the key because default validity
filtering depends on the current date. A hit skips embedding, both retrieval
arms and presentation. Maximum age 1 hour.

## Read items

The server caches the immutable part of a `read` item's fixed unit and part
sequence (the document entry without its status, leads, provisions, outline
entries or relations and their parts). The key is
`sha256(release, canonical original item JSON, mode)`. For an item given as
`{next}`, the server first validates and decodes the token exactly as without a
cache, then keys by the token's original item and mode; the token's position is
applied only when paging. For every response the server computes the document
status from the injected clock, adds it to the document entry, and only then
sizes units, cuts pages under the call cap and issues `next` tokens, so paging,
continuation and completeness behave exactly as without the cache. Release
content is immutable, so the maximum age is the life of the release, bounded at
24 hours.

## Document entries

`Engine.Documents` results are cached per release and document key: metadata,
the first relation pointers and the counts. They are immutable within a release.
Status, which comes from the hit stamp at present time, is never cached in this
entry.

## Search candidates

The candidate phase of a search collection is cached: for the current and the
non-current group, the ordered raw ANN, dense and lexical candidate lists (chunk
id, rank, cosine or score) and whether the exact fallback ran. Hydration,
validity, presentation and paging always run as on a miss, so a hit returns
exactly the bytes a miss returns. The search response cache still answers
identical repeats first; this cache serves the same question with a different
filter, detail level, limit or cursor page, none of which change collection.

The key is the SHA-256 of the canonical JSON of: the query recipe hash, the
retrieval recipe hash, the lexical recipe hash, the exact query text the
embedder receives, the resolved collection date (the Vietnam civil date of the
request, or the date a cursor pins, after cursor validation), the `as_of` date
or none, the dense mode (`ann` or `exact`) and both depths. The candidate phase
reads the date and whether `as_of` is given; the date already equals `as_of`
when it is given, and the key carries `as_of` itself so that no change in that
derivation can serve a wrong entry. The release is bound by the release-scoped
handle, as for the other corpus caches. Validation references, the query vector
and the sparse query are functions of the text and recipes, so they are covered.
Filters, nodes, documents, `in_force_only`, detail, limit, arm mode and cursor
position are not in the key: collection does not read them (`in_force_only` only
selects which collected group is shaped). Only serving collections are cached;
evaluation overrides (not-yet eligibility) never read or write it. Rejected
queries, errors, cancelled and timed-out collections are not cached. A hit still
runs request validation, served-filter checks and the query embedding lookup,
and hands hydration its own copy of the lists. A collection given precomputed
lexical arms bypasses the cache.

## Topic node pages

The public node page (`GET /api/v1/public/taxonomy/{code}`) is cached per
release and code: the node, its ancestors, its children and its leading
documents. The page reads only release content, so it is immutable within a
release and holds no account data. The key is `sha256(release, code)` after the
code passes validation; an unknown code, an invalid code and any error are not
cached. A hit still passes the public route's rate limit.

## Not cached

Credentials beyond the existing 10-second lookup cache in [auth](auth.md),
account data, tool errors (rate limits, unavailable services, inference
failures), and anything for a release that is not active.
