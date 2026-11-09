# Hybrid retrieval experiments (2026-11-09)

Status: **proposal, not authority**. Decisions on its open points are in
[ADR 0010](../adr/0010-retrieval-evaluation-for-agents.md). No experiments ran
for this note. Reproduce banhmi, then reuse cached rankings before lexical or
chunk rebuilds. Judge the final response. Keep
[ADR 0009](../adr/0009-dense-embedding-recipe.md) until hybrid evidence supports
an owner-approved change.

## Authorities and evidence

[Retrieval](../design/retrieval.md) owns responses; [data](../design/data.md)
owns release fields; [budgets](../design/budgets.md#retrieval) and
[testing](../../instructions/testing.md) own floors. ADRs
[0002](../adr/0002-evidence-only-vietnamese-queries.md) and
[0003](../adr/0003-postgres-per-release-corpus.md) prohibit served models other
than the query embedder and a second search engine.

Local evidence below comes from `/Users/danny/src/dannyota/banhmi/`:
`docs/design/rag.md`, `PLAN.md` (Current state), `deploy/eval/golden.json`,
`pkg/rag/lexical/bm25.go`, `pkg/rag/retrieve/{retrieve,rrf,evidence}.go`,
`pkg/pipeline/index_activities.go`, `internal/corpus/chunking/split.go`,
`pkg/base/config/config.go`, `pkg/eval/eval.go` and migrations. These describe
recipes, not code to copy. The [dense spike](embedder-2026-11.md) owns dense
numbers: 87 positive cases, 96 citations (two allow relation credit), 5
negatives.

## Evaluation protocol

Freeze corpus, golden hash, recipes, query vectors, software versions, date, tie
rules and configuration before running. Save case IDs, citation ranks, arm
ranks/scores, route, validity gaps, abstention reason and timings in the brief's
scratch directory. Use authored fixtures, never production queries. Score dense,
BM25 and hybrid with identical validity and response processing. Keep raw chunk
scores as diagnostics, separate from provision results.

| Measure               | Report                                                                                        | Gate or diagnostic                                                    |
| --------------------- | --------------------------------------------------------------------------------------------- | --------------------------------------------------------------------- |
| Recall@8              | Matched/expected citations, micro-average; also macro-average per case                        | Existing floor 0.90; macro informs                                    |
| MRR@8                 | Mean first correct reciprocal rank over positive cases; misses are zero                       | Existing floor 0.66                                                   |
| Recall@20             | Same citation denominator; request limit 20 separately                                        | Informs coverage versus ranking; proposed no regression for promotion |
| Current-law precision | Current/returned primary hits, micro-average; report unknown and contradictory-status counts  | Existing floor 0.99; never infer current law from retrieval score     |
| Abstention accuracy   | Correct decisions/all cases, plus TP/FP/TN/FN, negative recall and positive answer rate       | Existing floor 0.95; propose zero new negative-case failures          |
| Per-case difference   | Separate recall and reciprocal-rank wins/ties/losses; list each lost/gained citation and rank | Every loss needs review; no aggregate score hides a contract failure  |
| Subject filters       | Evaluate the exact paginated response under the contract below                                | Every case must pass; no averaging                                    |
| Cost                  | End-to-end and per-arm p50/p95/p99, RSS, relation/index bytes, build time                     | Host gates in budgets; Mac timings inform only                        |

Legacy parity retains number containment (`pkg/eval/eval.go:318`), alternates
and `relation_ok` credit; report direct-citation scores alongside it. The dense
spike omits relation credit. Product scores require exact country/document key
and provision anchor identities, mapped to the frozen corpus before rankings.
Adjudicate each alternate document/provision or relation-backed equivalent from
official evidence and freeze its explicit identity mapping with the labels. No
substring credit: `09/2020/TT-NHNN` cannot match `109/2020/TT-NHNN`. Ambiguous
or missing mappings block scoring until adjudicated, never silently remove
cases. Count each expected citation once. A parent earns child credit only when
returned evidence identifies and contains that child. Freeze this adapter before
chunk comparisons; all product promotion scores use it.

Legacy precision trims the trailing non-current run, but counts all hits if none
are current. Preserve that only for parity. Product default precision counts
every returned primary hit; an explicitly separated expired group is reported
separately. `partial` is document-level current, with provision repeal warnings,
not proof every clause survives. Missing dates remain visible with the required
gap; unknown status is not counted as current. Independently adjudicated
validity precision informs source-quality work and cannot be replaced by the
source-status metric. Zero returned hits is N/A, never 100%.

Subject filtering never constrains either retrieval arm. For each case whose
expected citations are inside selected nodes, filtered recall@8 must be at least
its unfiltered recall@8. For other cases, every expected citation must be
reachable within the first 100 paged hits. Verify `in_filter`, stable
inside-first ordering, `outside_filter_count`, no cursor duplicates or losses,
and unchanged unfiltered recall. Test all-inside, mixed, all-outside and empty
node selections. A 50+50 chunk union may contain fewer than 100 provisions;
report failure, never manufacture reachability by resetting a page's ranking.
Use fixed provision-to-node fixture assignments until taxonomy data exists; that
validates ordering only, not real taxonomy quality.

### Protect the gate set

The three unaccented positive cases (`edge-no-diacritics-infosec`, `-payment`,
`-mangluoi`) conflict with ADR 0002; ADR 0010 defines the cohorts that score
them.

The 92 old cases influenced model selection; ADR 0010 freezes them as cohorts.
**ADR 0010:** stage about 40 development cases, then about 40 sealed cases from
reserved groups, targeting 30 positive and 10 negative in each. The owner labels
with help against official evidence before rankings; record reason, source and
date. Cover identifiers, clauses, multiple documents, abbreviations, diacritics,
missing dates, historical law and out-of-domain queries.

Before assignment, connect cases sharing a document family (including
amendments, consolidations and adjudicated alternates) or the same narrow legal
intent or scenario, including paraphrases. A multi-document case joins all its
families; keep each connected component of new cases in one cohort. Group
document-free negatives by shared intent/scenario, including links to
document-backed cases. Components touching the old set cannot be sealed. Reserve
sealed components before development; freeze labels and group manifests, never
split a component to hit the target count. Report actual case and
independent-group counts.

Primary holds sealed labels. Use one development cohort, no folds. Log every
trial; fit all settings there, including model, ANN and abstention. Freeze one
complete finalist and baseline before the primary opens sealed results; run each
once. Never choose among models on sealed scores. Report old, development and
sealed results separately. A failed sealed gate retains baseline; further tuning
needs fresh sealed cases. Never edit expectations or floors to pass. Without new
labels, allow diagnosis only, not superiority claims.

Report paired 95% bootstrap intervals using 10,000 resamples of the components
above, seed 20261109, recalculating citation denominators. Include raw counts:
one old citation is 1/96 (1.04 points). Forty cases per cohort have low power,
especially with few independent groups; no power calculation justifies this
budget. Small negative cohorts cannot establish population accuracy of 0.95 or
0.99. Development intervals are descriptive after selection.

**ADR 0010, promotion rule:** all floors, subject contracts and host gates pass;
no new abstention failures or unexplained validity losses; recall@8, MRR@8 and
recall@20 do not fall on either the old or sealed gate cohort under its approved
scoring contract. Predeclare sealed product micro-recall@8 as the sole
superiority endpoint: its paired 95% interval for finalist minus baseline must
lie wholly above zero. MRR@8 and recall@20 are non-regression guards, not
alternative significance tests; development gains cannot establish superiority.
Conflicting metrics or an interval spanning zero retain baseline pending fresh
evidence. A cost-only change may pass at exact gate parity, labeled as such.

## Reproduce before tuning

Use `banhmi_vn`, PostgreSQL 18.6/pgvector 0.8.7, 3,974 documents and 142,086
chunks. Do not re-index this baseline. banhmi's Current state reports recall
93.8%, MRR 73.3%, precision and abstention 100% on 92 cases, not a new result.
Older PLAN entries describe 130,707 chunks and different MRR; do not mix them.

| Component        | Historical configuration to reproduce                                                                                                                                                                                                                                               |
| ---------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Dense            | `gold.chunk_embedding`, model `qwen3-embedding-0.6b`, 1,024 dimensions; matching Qwen query instruction/pooling/precision from the dense spike                                                                                                                                      |
| Lexical          | `content + " " + context_prefix`; lowercase, NFD, remove combining marks, đ→d, nonletters/digits→spaces, whitespace syllables; FNV-1a 32-bit modulo 2^20 plus 1                                                                                                                     |
| BM25             | Chunk is the statistical document; k1=1.2, b=0.75; IDF `ln(1+(N-df+0.5)/(df+0.5))`; weight `IDF*tf*(k1+1)/(tf+k1*(1-b+b*dl/avgdl))`; sum collisions in document vector, query presence=1 per hashed dimension                                                                       |
| Pools and fusion | Dense 50, lexical 50; one-based ranks; `1/(60+rd)+w/(60+rl)`, missing arm contributes zero; w=0.5, routed w=1.0; fused ties by chunk ID                                                                                                                                             |
| ANN              | Fetch max(400,24\*dense depth), thus 1,200; ef_search=min(pool,1000), strict iterative scan, force_custom_plan; exact filtered fallback on shortfall                                                                                                                                |
| Result shaping   | Hydrate first 8\*limit fused chunks; khoản roll-up keeps best member, consolidation-family duplicate collapse, cap 3 per document with demoted backfill; section aggregation off                                                                                                    |
| Routing          | Known document identifiers scope both arms, up to 8 resolved documents; abbreviation dictionary expands both arms; unaccented dense restoration and lexical boost; fingerprint dictionaries and family map                                                                          |
| Validity/gate    | Default current/partial primary pass plus at most 3 non-current dense hits, one per document; strict current-only suppresses tail; known identifier overrides default currency filtering; scope/known gaps also gate abstention; seed cosine threshold 0.3, verify restored setting |
| Chunks           | Article-first, recurse through legal children above rough 512 tokens (runes/2), carry substantive parent lead-in, then line/word shards; drop label-only or normalized content below 20 runes; exact content dedup within document                                                  |
| Prefix           | Number, title, Chương, Mục, effective date else issue date; title/headings capped at 220 runes each; prefix is a retrieval hint, never evidence                                                                                                                                     |

Primary first inventories `pg_indexes`, vector counts/model tags, restored
non-secret retrieval settings and dictionary hashes. Reproduce legacy Qwen with
its original evaluator as an independent reference. Compare rewritten harness
case ranks and rounded aggregates to that reference and accepted numbers.
Investigate matcher, validity, settings, ties and corpus hashes if they differ;
never tune parameters to manufacture parity. Sparse SQL has no ID tie-break in
banhmi: preserve it for parity and report tie sensitivity.

Model ablations require exact-versus-exact retrieval over identical eligible
chunks, holding lexical ranking and response processing fixed. Qwen's migration
`deploy/migrations/gold/00001_init.sql:17` creates HNSW; vi_v2's
`spikes/embedder/embed_chunks.py:52` creates only a primary key. Restored
indexes remain unverified. Substituting tables alone could mix ANN and exact
search. Use `spike.chunk_vector_vi_v2` with ADR 0009 vectors. Legacy query
expansion conflicts with that ADR's raw query rule; keep this parity ablation
separate. For product trials use raw dense queries, current-only default,
whole-country retrieval and provisions; disable legacy implicit scoping and
restoration. Calibrate abstention on development only; 0.3 is not portable
between models. Then measure each model's ANN candidate recall against its own
exact top-k, final hybrid metric losses, latency, index size and build cost
separately. Use the same corpus and declared ANN settings/budget; log any
model-specific tuning on development. Freeze the serving configuration before
sealed scoring.

### SQL shapes for the primary's harness

These are fresh specifications, not copied banhmi code. Bind values, not SQL
identifiers. Choose the vector table from a fixed allowlist. For Qwen bind `$1`
to its vector, `$2` to the model tag, `$3=1200`, `$4=50`:

```sql
WITH candidates AS MATERIALIZED (
  SELECT chunk_id, embedding <=> $1::vector AS distance
  FROM gold.chunk_embedding
  WHERE model = $2
  ORDER BY embedding <=> $1::vector
  LIMIT $3
)
SELECT c.id, c.document_id, c.section_id, c.citation, x.distance
FROM candidates x JOIN gold.chunk c ON c.id = x.chunk_id
WHERE c.document_id = ANY($5::bigint[])
ORDER BY x.distance, c.id LIMIT $4;
```

For vi_v2 replace the table, omit the model predicate and renumber: vector `$1`,
fetch `$2=1200`, depth `$3=50`, eligible IDs `$4::bigint[]`. Qwen uses `$5` for
those IDs, never subject-node IDs. Leave no unused parameter; PostgreSQL
[infers types from references](https://www.postgresql.org/docs/current/sql-prepare.html).
This shape is for ANN; set its options on the same connection. Exact runs must
join/filter all eligible embeddings before sorting by distance and chunk ID,
with no candidate pre-limit. Disable index/bitmap scans locally and verify plans
exclude approximate scans for both models. ANN shortfalls rerun exact, but a
full pool does not prove recall:
[pgvector](https://github.com/pgvector/pgvector#indexing) documents
approximate-search loss. Measure ANN loss even when the pool is full.

For lexical bind `$1` to the sparse query, `$2=50`, `$3` to the same IDs:

```sql
SELECT id, document_id, section_id, citation,
       -(content_sparse <#> $1::sparsevec) AS score
FROM gold.chunk
WHERE content_sparse IS NOT NULL
  AND content_sparse <#> $1::sparsevec < 0
  AND document_id = ANY($3::bigint[])
ORDER BY content_sparse <#> $1::sparsevec
LIMIT $2;
```

Build eligible IDs from `silver.document.id` and `silver.validity_period`
(`document_id,section_id,status_class,eff_from,eff_to,observed_at,id, superseded_at`).
Both legacy default and explicit date/filter paths require `section_id IS NULL`
before selecting the latest unsuperseded row per document by observed_at/id
(`pkg/rag/retrieve/retrieve.go:650,873`). Product validity also uses document
rows. Freeze `as_of`; keep missing dates with a gap per design. For unfiltered
arm controls, pass all document IDs. Hydrate document numbers/title through
`gold.chunk.document_id` to `silver.document(id,doc_number,title)`; map
provisions through `section_id` to
`silver.document_section(id,document_id,parent_id,kind,ordinal,label,heading, citation_path,content)`.
No product taxonomy mapping is assumed in this dump.

## Ranked experiments

Cache exact arm rankings/scores to the maximum trial depths first. Run fusion,
roll-up/cap and routing boosts from that cache before lexical or chunk rebuilds;
dictionary expansion needs fresh lexical rankings. Compare each change to the
fixed vi_v2 product baseline, combine development winners, then ablate each. For
lexical trials stream `gold.chunk(id,content,context_prefix,document_id)`;
recompute corpus-wide df/avgdl for each recipe, never on golden documents only.
Use a proposed scratch relation `(chunk_id bigint, sparse sparsevec(1048576))`
per trial and substitute its joined sparse column in the lexical SQL. Keep
baseline tables untouched. Prefix/title variants use the document join above.
All quality effects below are hypotheses, not predicted percentage gains.

| Priority         | Hypothesis and exact development experiment                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            | Host cost and release fields                                                                                                                                                                                                                                                                                    |
| ---------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1. Fusion/depth  | Complementary evidence may be buried. With weights fixed, compare (dense,lexical) depths (50,50), (100,50), (50,100), (100,100), (200,100); retain smallest non-losing pool. Then test RRF k={20,60,100} × lexical weight={0.25,0.5,1}, dense weight=1. Keep identifier boost fixed until routing trial. Report each arm's unique gold coverage, overlap and union recall.                                                                                                                                                                                             | More candidates/ANN work and hydration RAM, no vector growth. Record depths, ANN controls, k/weights in proposed retrieval recipe. Existing 50/50/60 budget changes need approval.                                                                                                                              |
| 2. Roll-up/cap   | Sibling crowding may hide other duties. Compare clause versus article grouping, cap={3,5,limit}; limit means no effective cap. Keep the best member score, never sum siblings. Also test provision grouping within each arm before RRF, assigning fresh provision ranks. Audit child-citation containment and multi-document losses.                                                                                                                                                                                                                                   | Reuses cached rankings; small extra grouping RAM/CPU, no index growth. Proposed retrieval recipe: group key, stage, member reducer, cap/backfill. Product still returns provisions.                                                                                                                             |
| 3. Query routing | Exact legal references may need more lexical weight. Compare off versus w=1 for validated document-number patterns; next, stable-promote exact document+Điều/khoản/điểm matches within the fused pool. Resolve numbers using `silver.document.doc_number`, canonical separator/case/đ normalization, equality only; parse bounded citation tokens against section paths. Preserve whole-country candidates and currency rules. Then compare lexical-only dictionary expansion off/on, sorted unique expansions with total tokens capped at 256; dense query stays raw. | Boosts reuse cached rankings; expansion refreshes lexical rankings. Bounded parsing/document lookup; no extra embedding or index growth. Proposed retrieval recipe grammar/dictionary hash; `lexical_recipe` query encoding. Test unknown/ambiguous IDs, two-law amendments, bare numbers and hostile patterns. |
| 4. Diacritics    | Folded homographs may displace precise legal terms. Compare folded-only with two namespaced channels: NFC lowercase accented syllables and existing folded syllables. Keep independent df/avgdl per channel; sum BM25 scores with accented weight 1 and folded weight {0.25,0.5,1}. Namespace before hashing; query uses both.                                                                                                                                                                                                                                         | Up to roughly twice token entries, more sparse scan work/cache; dense unchanged. `lexical_recipe` normalization/channel weights; `lexical_stats` per-channel df/lengths and collision count.                                                                                                                    |
| 5. Compounds     | Adjacent syllables may distinguish legal concepts better [segmentation evidence](https://aclanthology.org/L18-1410/). Add namespaced adjacent bigrams to unigrams, weights {0.5,1}; never cross punctuation, newline or field boundaries. Alternative: retain unigrams and add longest matches from an owner-reviewed, versioned legal-term list, weight 1. Test separately, never choose terms from failed gate cases.                                                                                                                                                | Up to roughly twice entries for bigrams; term-list matcher adds bounded CPU/RAM, no served model. `lexical_recipe` boundary rules/list hash; recompute `lexical_stats`. Segmentation gains on legal retrieval remain unverified.                                                                                |
| 6. Fields/BM25   | Repeated titles may swamp short duties. Compare existing concatenation with separately normalized BM25 fields: body weight 1, prefix {0,0.5,1}; then body=1, title=2, remaining prefix=0.5, removing the title from that prefix. Each field trains its own df/avgdl; weighted scores sum. On winning fields test (k1,b)=(0.8,0.5),(1.2,0.5),(1.2,0.75),(1.6,0.75).                                                                                                                                                                                                     | Separate fields add sparse entries/cache; parameter-only changes keep dimensions/nnz but require full lexical rebuild. Pin field extraction/weights/k1/b in `lexical_recipe`; statistics per field. No dense rebuild.                                                                                           |
| 7. Chunks/prefix | Rough rune splits may separate duties or over-repeat context. From the full section tree, rebuild the same legal-child-first algorithm at actual vi_v2 tokenizer body limits {256,512,768}, reserving prefix space within 2,048 total tokens. Keep parent lead-ins and no overlap. At winning size compare existing prefix with number+title+all ancestor headings, no date, each heading capped at 220 runes. Separately retain substantive leaves below 20 runes; label-only leaves still excluded.                                                                  | Highest cost: changed chunk count, full BM25 and dense rebuild, HNSW rebuild and activation disk/RAM. Proposed chunk recipe plus `dense_recipe` input hash; lexical statistics also change. Measure source-span loss, truncation and citation coverage before ranking.                                          |

[RRF's study](https://cormack.uwaterloo.ca/cormacksigir09-rrf.pdf) motivates the
baseline. [Bruch et al.](https://arxiv.org/abs/2210.11934) found sensitivity and
score-fusion gains on their datasets, not Vietnamese optimality. Defer score
fusion (changes RRF) and learned segmenters (another query model/dependency). No
secondary-source claims are relied on.

For all trials record per-case misses at candidate, fusion, grouping, cap and
abstention stages. Measure sparse nnz p50/p95/max and hash collisions before
considering sparse HNSW: [pgvector](https://github.com/pgvector/pgvector#hnsw)
limits its sparse HNSW vectors to 1,000 nonzeros, versus 16,000 for storage.
Never silently prune terms to fit. Exact sparse scans are a control, not a
latency promise. Raw sparse bytes are `8*nnz+16`; vector(1024) bytes are 4,104
([storage reference](https://github.com/pgvector/pgvector#reference)). Thus 100
extra sparse entries across 142,086 chunks add about 114 MB before tuple, index
and retained-release overhead. Measure `pg_total_relation_size` and
`pg_indexes_size`, cache reads and build time. No monthly cost saving is
claimed. Primary measures Mac costs first; host acceptance remains separate.

## Model decision and release order

BM25 may rescue Qwen's dense misses while adding little to vi_v2. Compare common
recipes first, then give each model the same development grid and trial budget.
Harrier needs full-corpus embeddings, its own hard negatives, query-precision
validation and host costs; screening cannot win. Select one complete finalist on
development. Retain vi_v2 on ties or conflicting metrics; model changes need the
promotion rule, host gates, owner approval and ADR 0009 History. Never mix model
vectors in a release.

ADR 0010 fixes the abstention statistic (first fused provision's best-member
cosine, including lexical-only hits) and adds `chunk_recipe` and
`retrieval_recipe` to the release contract. Sweep thresholds at development
score midpoints, maximize positive answer rate with zero negative failures, then
freeze per dense recipe.

Release order: independent review and owner decisions; baseline parity;
development experiments; frozen finalist and sealed gate; approved contract
updates; index/build/eval plan; host acceptance; release. Rebuild into a new
release, never mutate the baseline. Preserve source text/provision identities;
map every omitted or split span and report lost short provisions and duplicate
citations. Do not reuse vectors after input changes. Roll back by release
pointer, respecting existing pin retention. Bind SQL parameters, bound parser
work, pin dictionary hashes, and keep query text out of logs. Pending evidence:
parity, taxonomy fixtures, new labels, chosen recipes and every host cost.
