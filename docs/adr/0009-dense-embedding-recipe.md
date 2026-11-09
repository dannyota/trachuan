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

| Field                | Value                                                                 |
| -------------------- | --------------------------------------------------------------------- |
| Model                | `AITeamVN/Vietnamese_Embedding_v2`, revision `18b44161e041`           |
| Architecture         | XLM-RoBERTa encoder fine-tuned from BGE-M3, 568M parameters           |
| Dimensions           | 1,024, `vector(1024)`                                                 |
| Pooling              | CLS token, then L2 normalization; similarity is cosine                |
| Query input          | the question as written, no instruction prefix, at most 256 tokens    |
| Document input       | `strip(context_prefix) + "\n" + strip(content)`, no prefix, at most 2,048 tokens |
| Document embedding   | batch on the MacBook GPU (PyTorch MPS, FP16); about 18 minutes per full corpus |
| Query serving        | ONNX Runtime CPU, FP32, the repository's `onnx/model.onnx`            |
| Tokenizer            | the model repository's tokenizer at the same revision                |

Queries longer than 256 tokens are rejected before embedding; the model was
trained with 256-token queries.

## Consequences

Dense recall@8 on the golden set rises from 69/96 (Qwen3) to 76/96, MRR@8
from 0.457 to 0.580, with no chunk truncated. Queries take about 14 ms on the
MacBook CPU; Graviton latency is measured in Plan 4's acceptance gates. The
embedder budget is 2.5 GB because FP32 weights are 2.27 GB. Re-embedding a
release takes minutes, not hours. The model comes from a small team, so the
golden set is the only quality evidence we rely on.

## Alternatives rejected

- Qwen3-Embedding-0.6B: 7 citations lower at recall@8, 5 times slower to embed.
- microsoft/harrier-oss-v1-0.6b: close on quality (74/96, higher MRR@8 0.637
  on the screening corpus) but twice as slow per query, 5 times slower to
  embed, and 3.0 GB in memory. Kept as the fallback; it shares the Qwen3
  architecture, so the validated weight-only 8-bit ONNX path applies.
- mainguyen9/vietlegal-harrier-0.6b: 69/96, truncates at 512 tokens.
- Dynamic INT8 queries: parity median 0.989 below the 0.995 gate, one citation
  lost. Weight-only 8-bit: two citations lost, seven times slower on CPU.
- EmbeddingGemma-2, pplx-embed, Nemotron-3-Embed-1B, granite R2, voyage-4-nano:
  not measured by the owner's choice; jina v5 (non-commercial) and LFM2.5 (no
  Vietnamese) excluded.

## History

- 2026-11-09: accepted after the embedder spike.
