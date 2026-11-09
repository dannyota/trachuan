# ADR 0009: Dense embedding recipe for Vietnam

Status: accepted 2026-11-09.

## Context

The design left the dense model open until a spike measured candidates on the
Vietnam corpus. The spike ([research note](../research/embedder-2026-11.md))
restored banhmi's corpus (142,086 chunks), scored dense-only recall on banhmi's
92 golden cases, screened the newest multilingual models, and measured CPU
serving precision. banhmi used Qwen3-Embedding-0.6B.

## Decision

Release 1 uses this dense recipe; the release manifest records every field.

| Field              | Value                                                                              |
| ------------------ | ---------------------------------------------------------------------------------- |
| Model              | `AITeamVN/Vietnamese_Embedding_v2`, revision `18b44161e041`                        |
| Architecture       | XLM-RoBERTa encoder fine-tuned from BGE-M3, 568M parameters                        |
| Dimensions         | 1,024, `vector(1024)`                                                              |
| Pooling            | CLS token, then L2 normalization; similarity is cosine                             |
| Query input        | the question as written, no instruction prefix, at most 256 tokens                 |
| Document input     | `strip(context_prefix) + "\n" + strip(content)`, no prefix, at most 2,048 tokens   |
| Document embedding | batch on the MacBook GPU (PyTorch MPS, FP16); about 20 minutes for 98,506 chunks   |
| Query serving      | ONNX Runtime CPU, dynamic INT8 per-channel from the repository's `onnx/model.onnx` |
| Tokenizer          | the model repository's tokenizer at the same revision                              |

Queries longer than 256 tokens are rejected before embedding; the model was
trained with 256-token queries.

## Consequences

On the golden set, dense recall@8 rises from 69/96 (Qwen3) to 76/96 and MRR@8
from 0.492 to 0.589, with no chunk truncated; the model is better on 14 cases
and worse on 7. These are dense-only, chunk-level figures without a validity
filter, over 87 scored cases (one citation is about 1.04 points), so they rank
models; they are not product quality. Hybrid recall with BM25 is measured before
release.

The embedder budget is 2.5 GB because the FP32 weights are 2.27 GB. Re-embedding
a release of 98,506 chunks takes about 20 minutes on the MacBook GPU in FP16.
The model comes from a small team, so the golden set is the only quality
evidence we rely on.

Query latency is measured only on the MacBook (ONNX FP32, about 14 ms at p50);
host latency is unmeasured and FP32 on Graviton2 may be several times slower.
The host acceptance gates in [budgets](../design/budgets.md) decide. If the host
misses the search p95 gate or the 2 s embedder deadline at the 256-token query
cap, lower the query cap to 128 tokens and record the change in this ADR's
History. A query precision change such as the dynamic INT8 ONNX export
(measured: one citation lost, about 1 GB) needs an owner-approved revision of
this record and ADR 0011.

## Alternatives rejected

- Qwen3-Embedding-0.6B: 7 citations lower at recall@8; about 5 times slower to
  embed documents on the MacBook GPU.
- microsoft/harrier-oss-v1-0.6b: 74/96 and MRR@8 0.643 on the screening corpus,
  which are upper bounds because its own hard negatives outside that corpus were
  never scored. On the MacBook it is twice as slow per query, about 5 times
  slower to embed, and uses 3.0 GB. Kept as the fallback model; it shares the
  Qwen3 architecture, but the weight-only 8-bit ONNX path was validated on
  Qwen3's weights only and must be re-validated for Harrier.
- mainguyen9/vietlegal-harrier-0.6b: 69/96 on the screening corpus; truncates at
  512 tokens.
- Dynamic INT8 queries now: parity median 0.989 below the 0.995 gate and one
  citation lost; kept as the host fallback above. Weight-only 8-bit: two
  citations lost and about 7 times slower on the MacBook CPU.
- EmbeddingGemma-2, pplx-embed, Nemotron-3-Embed-1B, granite R2, voyage-4-nano:
  not measured by the owner's choice; jina v5 (non-commercial) and LFM2.5 (no
  Vietnamese) excluded.

## History

- 2026-11-09: accepted after the embedder spike.
- 2026-11-09: final review; added caveats, Mac-only labels and the host
  fallback, after rescoring with banhmi's alternate document numbers (counts
  unchanged, MRR@8 slightly higher).
- 2026-11-09: the primary selected the Python service in
  [ADR 0011](0011-embedder-services.md). It reports both runtime recipes and
  rejects over-cap input without truncation; this record still owns the model,
  tokenizer, pooling, dimensions and precision choices.
- 2026-11-09: one precision on both sides. Document embedding moved from MPS
  FP16 to MPS FP32, because CPU queries stay FP32. On an 800-chunk sample at
  batch 16, FP32 runs at 33.2 chunks per second against 85.6 for FP16, and the
  two agree to cosine 0.99981 at minimum. [ADR 0011](0011-embedder-services.md)
  moved the query runtime to Go; query serving stays ONNX Runtime CPU FP32.
- 2026-11-09: FP16 queries on the host CPU rejected by measurement. On the OCI
  A1 host (Neoverse-N1, ONNX Runtime 1.31.1, golden questions), an FP16
  conversion of `onnx/model.onnx` took 1,722 ms at p50 for short queries with 2
  threads against 329 ms for FP32, and 15.2 s against 2.5 s for a 217-token
  query; vectors agreed to cosine 0.99978 at minimum. One precision on both
  sides therefore means FP32.
- 2026-11-09: the owner chose FP16 documents and FP32 queries. FP16 queries are
  5 times slower on the host CPU, FP16 documents are 2.6 times faster than FP32
  on MPS, and FP16 and FP32 document vectors agree to cosine 0.99981 at minimum.
  The recall figures in this record were measured with this pair. The first
  baseline release, vn/20261109, was embedded with FP32 documents before this
  choice.
- 2026-11-10: the owner chose INT8 queries for speed. Query serving uses dynamic
  INT8 with per-channel weights (`quantize_dynamic`, QInt8, per_channel) made
  from `onnx/model.onnx`. On the A1 host it scored 75/96 recall@8 and MRR@8
  0.5886 against 76/96 and 0.5885 for FP32, 3.2 times faster (65 ms short, 387
  ms long at p50) in 0.83 GB; per-tensor INT8 lost four citations. Cosine to
  FP32 is 0.987 at median, below the 0.995 parity gate, so release evaluation on
  the trachuan corpus is the acceptance evidence. Documents stay FP16 on MPS.
  [Research note](../research/query-int8-2026-11.md).
