# Lexical recipe 1

`internal/retrieval/lexical` owns document and query encoding in the root Go
module. `build` calls the document encoder; search calls the query encoder.
There is no lexical pipeline stage because statistics cover the entire built
chunk set. This reimplements the
[study's baseline](../research/retrieval-hybrid-2026-11.md#reproduce-before-tuning).

## Recipe and statistics

Recipe JSON fields: `version: 1`, `algorithm: "bm25-vn-fold/1"`,
`input: "content-space-prefix"`, `normalization: "lower-nfd-mn-dd/1"`,
`tokenizer: "unicode-letter-digit/1"`, `hash: "fnv1a32"`, `dimensions`, `k1`,
`b`, `query: "hashed-presence"`, `collision: "sum-document"`. Numeric values
live in [budgets](budgets.md#indexing). Reject an unknown recipe; never silently
fall back to a normalizer.

Document input is `content + " " + context_prefix`. Lowercase with Go Unicode
case mapping, NFD-decompose, discard Unicode Mn marks, replace `đ` with `d`,
retain Unicode letters and digits, replace everything else with spaces, and
split on Unicode whitespace. Tokens are syllables, including single characters
and numbers. No stopwords, stemming, dictionary expansion or word segmentation.
Pin Go and `golang.org/x/text` versions in the build provenance.

Let N be the count of chunks actually inserted, including zero-token chunks.
Count df once per distinct normalized token per chunk, before hashing. dl is the
number of tokens including repeats. avgdl is total dl/N, or 1 when N or total dl
is zero. Use float64 for statistics and weights:

```text
idf(t) = ln(1 + (N - df(t) + 0.5) / (df(t) + 0.5))
weight(t,c) = idf(t) * tf(t,c) * (k1+1)
              / (tf(t,c) + k1*(1-b+b*dl(c)/avgdl))
dimension(t) = (FNV-1a-32(UTF-8(t)) modulo dimensions) + 1
```

Sum document weights that collide at a dimension in sorted token order, then
convert to finite float32 and sort dimensions ascending. Query input is the raw
query: tokenize identically and set each distinct hashed dimension to 1,
irrespective of token frequency or collisions. Score is the sparse inner
product. Keep positive scores only, descending score then ascending chunk id.
Empty queries or zero-token chunks have empty sparse vectors and no lexical
hits. Never prune nonzero entries to fit storage limits.

`lexical_stats` is
`{version,N,total_tokens,avgdl,vocabulary_sha256,df_sha256, distinct_terms,occupied_dimensions,collision_terms,nnz:{p50,p95,max}}`.
`lexical_df(token text PRIMARY KEY, dimension integer, df bigint)` stores the
complete statistics. Hash UTF-8 token-sorted JSON arrays of `[token,dimension]`
for vocabulary and `[token,df]` for df, with no whitespace. Collision count is
distinct terms minus occupied dimensions. Statistics change per corpus;
`lexical_recipe` changes only when encoding changes.

## Go contract and build

Package files: `recipe.go`, `tokenize.go`, `hash.go`, `stats.go`, `encode.go`.
Types: `Recipe` holds the fields above; `Entry{Index int32, Value float32}`;
`Vector []Entry`;
`Stats{N,TotalTokens int64, AvgDL float64, DF map[string]int64}`. Functions:
`Tokenize(text string, recipe Recipe) ([]string,error)`,
`Train(ctx context.Context, texts TextIterator, recipe Recipe) (Stats,error)`,
`Document(text string, stats Stats, recipe Recipe) (Vector,error)`,
`Query(text string, recipe Recipe) (Vector,error)`.
`TextIterator.Next(context.Context) (string,bool,error)` streams ordered chunks.

Build makes two passes over inserted chunks, retaining only token df in memory,
then writes sparse vectors in bounded batches. Query encoding does not load df;
release startup validates the stats hashes and recipe. Exact sparse scanning
keeps the baseline's scoring and avoids sparse HNSW's tighter nonzero limit.
Cost grows with chunk count and distinct tokens; limits fail build rather than
drop terms. Required checks cover Unicode normalization, collision addition,
query presence, empty corpus, repeated terms, deterministic summation and
build/query encoding parity.

Verified locally on 2026-11-09 against the pinned image (PostgreSQL 18.6,
pgvector 0.8.7, ltree 1.3): `sparsevec(1048576)` stores 16,000 nonzeros and
rejects 16,001 with "sparsevec cannot have more than 16000 non-zero elements",
matching the
[pgvector storage reference](https://github.com/pgvector/pgvector#reference).
