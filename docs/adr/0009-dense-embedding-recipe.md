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

Queries use per-channel dynamic INT8 on the OCI A1 host; documents stay FP16 on
MPS. The [host precision study](../research/query-int8-2026-11.md) measures the
accepted pair against FP32 queries: about 3.2 times faster, lower resident
memory and one fewer matched citation, with effectively unchanged MRR. The
2026-11-10 History entry records the accepted measurements. These dense-only,
chunk-level results use banhmi's corpus without validity filtering; they do not
establish trachuan product quality. Cosine below the FP32 parity gate is an
accepted precision trade-off, not a reason to waive release retrieval floors.

The [earlier A1 measurement](../research/host-oci-a1-2026-11.md) measured FP32
through the Python service and showed thread-sensitive long-query latency. The
later INT8 measurements are direct ONNX Runtime timings, not Go service latency
or concurrent search acceptance. [Budgets](../design/budgets.md) owns the memory
allocation, thread limits, deadlines and host gates. Measure the Go INT8 service
with PostgreSQL loaded before release. If the host misses those gates at the
query cap, the existing fallback is a 128-token cap, recorded in History. INT8
is already approved; any further precision change revises this record and ADR
0011 with owner approval.

Changing only the query recipe requires a new build and evaluation, not new
document vectors. Keep earlier artifacts for rollback. FP16 document cost and
input limits remain in the Decision table. No additional hosted service is
needed. [ONNX Runtime quantization][quantization] documents the mechanism; the
local studies, not that documentation, establish this model's trade-offs.

[quantization]:
  https://onnxruntime.ai/docs/performance/model-optimizations/quantization.html

## Alternatives rejected

- Qwen3-Embedding-0.6B: lower recall and about 5 times slower to embed documents
  on the MacBook GPU in the original spike. The A1 study shows further recall
  loss with dynamic INT8; its weight-only path preserves FP32 recall but is
  slower than the selected model's per-channel INT8 queries.
- microsoft/harrier-oss-v1-0.6b: 74/96 and MRR@8 0.643 on the screening corpus,
  which are upper bounds because its own hard negatives outside that corpus were
  never scored. On the MacBook it is twice as slow per query, about 5 times
  slower to embed, and uses 3.0 GB. It remains a model candidate, not the query
  precision fallback. The A1 study measures its INT8 and weight-only paths, but
  full-corpus recall remains unmeasured; changing models needs ADR 0010 gates.
- mainguyen9/vietlegal-harrier-0.6b: 69/96 on the screening corpus; truncates at
  512 tokens.
- FP32 queries: one more citation in the A1 study, but slower and larger than
  the accepted per-channel INT8 export. FP16 CPU queries were slower still, as
  History records; FP16 remains the document precision only.
- Per-tensor dynamic INT8: four citations lost against FP32 on A1. Weight-only
  8-bit (`q8w_a4`): two lost and slower than per-channel INT8 on the same host.
  The study's Results table owns the per-variant measurements.
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
- 2026-11-10: aligned Consequences and Alternatives with the accepted INT8
  query/FP16 document pair and A1 measurements; removed obsolete FP32 budget,
  Graviton latency and pending-INT8 language. The decision is unchanged.
