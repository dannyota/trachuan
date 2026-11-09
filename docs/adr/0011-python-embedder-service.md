# ADR 0011: Python embedding service with two runtimes

Status: accepted 2026-11-09; the contract passed independent design review on
2026-11-09.

## Context

[ADR 0009](0009-dense-embedding-recipe.md) selects the model, tokenizer, pooling
and precision. Document embedding runs on the MacBook GPU; serving uses CPU
ONNX. Both need the same explicit input limits and recipe checks, without
putting model bindings inside the Go retrieval package.

## Decision

Use one uv-managed Python 3 HTTP service in `apps/embedder`, with runtime chosen
at startup: `query` loads ONNX Runtime CPU FP32 and `onnx/model.onnx`;
`document` loads PyTorch MPS FP16 through an optional dependency group, on the
MacBook only. The [service contract](../design/embedder.md) owns endpoints,
admission and failure behavior. ADR 0009 continues to own the dense recipe.

Report the complete runtime recipe. Callers reject mismatches before using
vectors. Reject input above the token cap; never truncate. The query runtime
becomes the host image in the host plan. No answer model or reranker is added.

## Consequences

Python uses the same ecosystem as the measured spike and supports both runtimes
without a Go native bridge. HTTP adds serialization and one local hop; benchmark
that cost with the host gates. The separate process isolates model memory and
admission, at the cost of another locked dependency set and health lifecycle.
Query memory remains the FP32 budget from ADR 0009; this choice claims no
monthly saving or measured host latency.

Tests inject small runtimes and tokenizers. Real-model smoke and parity checks
remain separate, because fake vectors cannot establish retrieval quality.
Provision model files locally with hashes, no runtime downloads or remote code,
and never log input text. The document runtime has no public listener.

Adopt the service before embedding new artifacts. No existing production
endpoint needs migration; spike clients remain disposable. Rebuild vectors if
the recorded recipe or input changes. Never mix vectors from runtimes other than
the pinned document/query pair. Retain prior artifacts for rollback.

## Alternatives rejected

- ONNX Runtime through cgo inside the Go server: removes HTTP but couples native
  model memory and cancellation to the API process and still needs a separate
  MPS document path.
- Rust service: keeps isolation but introduces an additional language and
  model/tokenizer integration without measured benefit over the Python spike.

## History

- 2026-11-09: the primary selected Python, uv and two startup runtimes for the
  index and evaluation work; the designer specified the HTTP contract.
