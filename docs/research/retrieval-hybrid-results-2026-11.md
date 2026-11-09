# Hybrid retrieval baseline and model ablation (2026-11-09)

First measurements under
[ADR 0010](../adr/0010-retrieval-evaluation-for-agents.md) and the
[study](retrieval-hybrid-2026-11.md). Not authority. All runs use banhmi's own
evaluator as a tool (no banhmi code copied), its 92 legacy cases (87 scored, 96
citations) and legacy matcher, on the local restore of banhmi v049 (3,974
documents, 142,086 chunks), PostgreSQL 18.6, pgvector 0.8.7.

## Results

| Run                                                         | recall@8      | MRR@8 |
| ----------------------------------------------------------- | ------------- | ----- |
| banhmi accepted (PLAN.md, commit 1dd216b, production, PG17) | 93.8% (90/96) | 73.3% |
| Qwen3 hybrid, HNSW, local                                   | 84.4% (81/96) | 69.7% |
| Qwen3 hybrid, exact search, local                           | 87.5% (84/96) | 72.0% |
| Qwen3 dense only (banhmi pipeline), exact                   | 86.5% (83/96) | 69.2% |
| BM25 only (banhmi pipeline)                                 | 59.4%         | 45.8% |
| Vietnamese_Embedding_v2 hybrid, exact, all else unchanged   | 88.5% (85/96) | 70.6% |

Current-law precision and abstention were 100% in every run. Per case,
Vietnamese_Embedding_v2 beat Qwen3 on 21 cases and lost on 18 (losses include
both 08/2024 interbank payment cases, 20/2023 data messages and one unaccented
query). Under ADR 0010 conflicting metrics keep Vietnamese_Embedding_v2; this is
a diagnosis on the legacy cohort, not a superiority claim.

## Parity

The accepted 93.8% was not reproduced. The same numbers come from banhmi HEAD
and from the accepting commit 1dd216b, so code drift is ruled out; sparse
vectors cover every chunk and both arms work; HNSW on this restore loses 3
citations against exact search. Six citations remain unexplained; candidates are
the production database state at evaluation time compared with the v049 dump,
and PostgreSQL 17 against 18. Comparisons in this project use the local exact
baseline of 84 of 96.

## How to rerun

Vietnamese_Embedding_v2 vectors replace the Qwen3 vectors in a copy of the
corpus database (`banhmi_vn_vi`); `spikes/embedder/openai_server.py` serves
query embeddings (ONNX FP32, CLS pooling, L2, prefix stripped) through banhmi's
OpenAI-compatible embedder setting. Exact search is forced with
`enable_indexscan = off` and `enable_bitmapscan = off` on the database.
