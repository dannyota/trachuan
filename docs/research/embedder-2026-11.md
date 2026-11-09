# Dense embedding model for Vietnam (2026-11-09)

Throwaway spike `spikes/embedder/` compared embedding models for the Vietnam
corpus and checked how a quantized model serves on the planned host. The
decision is [ADR 0009](../adr/0009-dense-embedding-recipe.md). Not authority;
numbers below are dense-only measurements, not product retrieval quality.

## Setup

- Corpus: banhmi `banhmi-v049.dump` restored locally, 3,974 documents, 142,086
  chunks. Document text is `strip(prefix) + "\n" + strip(content)`.
- Questions: banhmi `deploy/eval/golden.json`, 92 cases; 87 expect 96 citations,
  5 expect abstention.
- Metric: exact cosine scan over chunks, top k chunks, banhmi's citation
  matching rule: document number or one of its `alt_doc_numbers`, then
  Điều/khoản/điểm tokens. banhmi's `relation_ok` credit is not applied (2 of 96
  citations use it). Recall is micro-averaged over citations, MRR averaged over
  cases. No validity filter, no BM25, no grouping by provision. With 87 scored
  cases, one citation is about 1.04 points.
- Software: PostgreSQL 18.6, pgvector 0.8.7 (banhmi's dump came from PostgreSQL
  17); torch 2.15.0, onnxruntime 1.31.1, transformers 5.22 and
  sentence-transformers 6.2 after the upgrade (4.57.6 and 5.7.0 before it).
- Hardware: MacBook M5 Max (18 cores, 36 GB) for embedding and probes; one AWS
  `t4g.large` (Graviton2, 2 vCPU, 8 GB) for the quantized Qwen soak.

## Recipe check (Qwen3-Embedding-0.6B)

ONNX FP32 export with last-token pooling, L2 normalization and banhmi's legal
query instruction reproduces banhmi's stored FP16 vectors: cosine median and
minimum 0.99999 on 200 chunks. PyTorch FP16 on MPS gives 1.00002 and 0.99975.

## Quantization parity (Qwen3, 1,000 chunks and 92 questions vs FP32)

| Variant                                    | Chunks median / min | Questions median / min | Gate 0.995 / 0.98 |
| ------------------------------------------ | ------------------- | ---------------------- | ----------------- |
| Dynamic INT8 (`quantize_dynamic`)          | 0.9643 / 0.9313     | 0.9674 / 0.9457        | fail              |
| Weight-only 8-bit, float compute (`q8w`)   | 0.9992 / 0.9987     | 0.9994 / 0.9987        | pass              |
| Weight-only 8-bit, int8 compute (`q8w_a4`) | 0.9980 / 0.9960     | 0.9982 / 0.9959        | pass              |
| Weight-only 4-bit (`q4w`, 200 chunks)      | 0.894 / 0.866       |                        | fail              |

Dense recall@8 with stored document vectors: FP32 queries 69/96 (MRR@8 0.492),
`q8w` and `q8w_a4` 68/96, dynamic INT8 66/96 with default threads and 62/96 when
rerun with 4 threads. Dynamic per-tensor activation quantization breaks this
decoder and is not even stable across thread counts; weight-only 8-bit is safe.

## Model comparison

Full corpus (142,086 chunks):

| Model                            | recall@8       | MRR@8  | recall@20 |
| -------------------------------- | -------------- | ------ | --------- |
| AITeamVN/Vietnamese_Embedding_v2 | 0.7917 (76/96) | 0.5885 | 0.8542    |
| Qwen/Qwen3-Embedding-0.6B        | 0.7188 (69/96) | 0.4922 | 0.8125    |

Screening corpus (28,632 chunks: 11,948 chunks of the 77 documents the answers
cite, including consolidated texts named as alternates; 3,283 more from the top
50 per question of Qwen3 and Vietnamese_Embedding_v2; 13,401 seeded random
chunks):

| Model                             | recall@8       | MRR@8  | recall@20 | Better / worse than Qwen3 (cases) |
| --------------------------------- | -------------- | ------ | --------- | --------------------------------- |
| AITeamVN/Vietnamese_Embedding_v2  | 0.7917 (76/96) | 0.5885 | 0.8542    | 14 / 7                            |
| microsoft/harrier-oss-v1-0.6b     | 0.7708 (74/96) | 0.6433 | 0.8750    | 10 / 5                            |
| Qwen/Qwen3-Embedding-0.6B         | 0.7188 (69/96) | 0.4922 | 0.8125    |                                   |
| mainguyen9/vietlegal-harrier-0.6b | 0.7188 (69/96) | 0.5583 | 0.7917    | 8 / 8                             |

Qwen3 and Vietnamese_Embedding_v2 score the same on the screening and full
corpora because their own top-50 negatives are in the screening set. For the
same reason the Harrier and VietLegal-Harrier figures are upper bounds: their
own hard negatives outside the set were never scored. VietLegal-Harrier
truncated 275 chunks at its 512-token limit and trails base Harrier.

Vietnamese_Embedding_v2 wins on information security and banking topics (50/2024
online banking safety, 116/2025 cybersecurity, 17/2024 eKYC, 340/2025 penalties,
356/2025 personal data abroad) and loses on 20/2023 electronic transactions,
08/2024 interbank payments and 09/2020 business continuity.

## Speed and memory

| Measurement                                                      | Vietnamese_Embedding_v2 | Harrier-0.6B                           |
| ---------------------------------------------------------------- | ----------------------- | -------------------------------------- |
| One golden question, Mac CPU, PyTorch FP32, 4 threads, p50 / p95 | 38 / 50 ms              | 79 / 84 ms                             |
| Process RSS, Mac CPU, PyTorch FP32 (decimal GB)                  | 2.15 GB                 | 3.02 GB                                |
| Model load                                                       | 5.4 s                   | 20.9 s                                 |
| Document embedding, MPS FP16                                     | 135-178 chunks/s        | 28-33 chunks/s                         |
| Full corpus re-embed                                             | 1,066 s (measured)      | about 80 min (estimated from the rate) |

Graviton reference for the Qwen3 architecture (Harrier shares it), `q8w_a4` on
`t4g.large`: cold start 5.1 s; one short question p50 286 ms, p95 345 ms; 2
requests per second for 600 s with real questions: 1,200 of 1,200, p50 302 ms,
p95 391 ms, p99 433 ms, RSS 774 MB; about 256-token inputs one at a time p50
1,539 ms. Vietnamese_Embedding_v2 was not run on Graviton.

## Serving precision for Vietnamese_Embedding_v2

The model repository ships an ONNX FP32 export (`onnx/model.onnx`). Mac CPU, 4
threads, each variant in a fresh process, 92 golden questions one at a time;
parity is against PyTorch FP32 on 200 chunks and the 92 questions (not the
1,000-probe gate used for Qwen3); RSS is peak resident memory in GiB; recall
uses the stored full-corpus document vectors (MPS FP16):

| Query model (ONNX) | Parity chunks / questions, median | Peak RSS | p50 / p95    | recall@8 | MRR@8  |
| ------------------ | --------------------------------- | -------- | ------------ | -------- | ------ |
| FP32               | 1.0 / 1.0                         | 1.84 GiB | 14 / 22 ms   | 76/96    | 0.5885 |
| Weight-only 8-bit  | 0.9994 / 0.9996                   | 1.05 GiB | 102 / 124 ms | 74/96    | 0.5868 |
| Dynamic INT8       | 0.9894 / 0.9926 (below 0.995)     | 1.01 GiB | 13 / 17 ms   | 75/96    | 0.5806 |

FP32 in ONNX loses nothing, is about as fast as dynamic INT8 on the Mac (14 ms
against 13 ms at p50), and uses less memory than PyTorch (2.15 GB). Peak RSS was
measured after 92 questions; pages of the 250k-token vocabulary table load as
tokens appear, so long-running RSS can approach the full 2.27 GB of weights.
Quantization saves about 0.8 GB at the cost of one or two citations. All of
these timings are Mac timings; Graviton latency for this model is not measured.

## Not measured

- EmbeddingGemma-2, pplx-embed-v1-0.6b, Nemotron-3-Embed-1B, granite-embedding
  multilingual R2 and voyage-4-nano: dropped by the owner to shorten the
  screening round. Published multilingual scores put EmbeddingGemma-2 (61.4 MTEB
  v2) and granite R2 (65.2 retrieval) near Qwen3.
- jina-embeddings-v5 (CC-BY-NC) and LFM2.5-Embedding (no Vietnamese) were
  excluded before testing. BGE-M3 and Vietnamese_Embedding v1 were excluded on
  the owner's earlier results.
- Hybrid recall with BM25, and Vietnamese_Embedding_v2 latency on Graviton.
