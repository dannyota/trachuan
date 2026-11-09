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

| Field              | Value                                                                            |
| ------------------ | -------------------------------------------------------------------------------- |
| Model              | `AITeamVN/Vietnamese_Embedding_v2`, revision `18b44161e041`                      |
| Architecture       | XLM-RoBERTa encoder fine-tuned from BGE-M3, 568M parameters                      |
| Dimensions         | 1,024, `vector(1024)`                                                            |
| Pooling            | CLS token, then L2 normalization; similarity is cosine                           |
| Query input        | the question as written, no instruction prefix, at most 256 tokens               |
| Document input     | `strip(context_prefix) + "\n" + strip(content)`, no prefix, at most 2,048 tokens |
| Document embedding | batch on the MacBook GPU (PyTorch MPS, FP16); about 18 minutes per full corpus   |
| Query serving      | ONNX Runtime CPU, FP32, the repository's `onnx/model.onnx`                       |
| Tokenizer          | the model repository's tokenizer at the same revision                            |

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
a release takes about 18 minutes on the MacBook GPU. The model comes from a
small team, so the golden set is the only quality evidence we rely on.

Query latency is measured only on the MacBook (ONNX FP32, about 14 ms at p50);
host latency is unmeasured and FP32 on Graviton2 may be several times slower.
The host acceptance gates in [budgets](../design/budgets.md) decide. If the host
misses the search p95 gate or the 2 s embedder deadline at the 256-token query
cap, switch the query model to the dynamic INT8 ONNX export (measured: one
citation lost, about 1 GB) and, if still slow, lower the query cap to 128
tokens; record the change in this ADR's History.

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
  [ADR 0011](0011-python-embedder-service.md). It reports both runtime recipes
  and rejects over-cap input without truncation; this record still owns the
  model, tokenizer, pooling, dimensions and precision choices.
