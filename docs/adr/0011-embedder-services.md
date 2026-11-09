# ADR 0011: Go query embedder, Python document embedder

Status: accepted 2026-11-09; revised 2026-11-09 to serve queries from Go.

## Context

[ADR 0009](0009-dense-embedding-recipe.md) selects the model, tokenizer, pooling
and precision. Two paths embed text. The read path embeds one query per search
on the host, on CPU with ONNX Runtime FP32. The write path embeds every chunk of
a release on the MacBook, where PyTorch on the GPU (MPS) embeds the 98,506-chunk
corpus in about 50 minutes in FP32; Go has no binding for that GPU path. Both
paths need the same input limits and recipe checks, without model bindings
inside the Go retrieval package. banhmi served queries from a standalone Go
service built on the ONNX Runtime Go binding, which the owner maintains, and the
Rust tokenizers library.

## Decision

One HTTP contract, specified in the [embedder design](../design/embedder.md),
has two services with one runtime each:

- Query service, Go, `apps/queryembed`: ONNX Runtime CPU FP32 with
  `onnx/model.onnx`, tokenizer through `github.com/daulet/tokenizers`. It runs
  in its own container on the host and locally for evaluation, never inside the
  server process.
- Document service, Python with uv, `apps/docembed`: PyTorch MPS FP32, on the
  MacBook only, bound to loopback.

The host runs no Python. A release is evaluated with the query service that will
serve it, so the evaluated and served query recipe hashes match. Each service
reports its complete runtime recipe; callers reject mismatches before using
vectors. Reject input above the token cap; never truncate. No answer model or
reranker is added.

The Python query mode written first stays only as the parity reference. The Go
service replaces it after three checks on the same model bundle and ONNX Runtime
version: identical token ids for the golden queries and edge-case texts; cosine
at least 0.9999 for every input; and, for every evaluation case, identical
ordered hit identities and abstention decisions from two builds of the same
inputs that differ only in the query recipe. Each build keeps its strict recipe
check; no build is evaluated with the other service. The Python query mode is
then deleted.

## Consequences

The host image holds one Go binary, the ONNX Runtime shared library and the
model, with no Python dependency set. cgo appears only in the query service's
tokenizer and ONNX packages, behind a build tag, so other binaries and default
tests stay pure Go. Native libraries are pinned by version and SHA-256, like
model files. HTTP adds serialization and one local hop; the host gates measure
it. The separate process isolates model memory and admission. Query memory
remains the FP32 budget from ADR 0009.

Two languages implement the contract, so each side has its own contract tests,
and the opt-in real-model smoke checks both against one bundle. Both tokenize
with the Rust tokenizers library from the same `tokenizer.json`; the parity
check proves identical ids instead of assuming them.

Tests inject small runtimes and tokenizers. Real-model smoke and parity checks
remain separate, because fake vectors cannot establish retrieval quality.
Provision model files locally with hashes, no runtime downloads or remote code,
and never log input text. The document service has no public listener.

Rebuild document vectors if the recorded document recipe or input changes. A
query recipe change needs a new build and evaluation, not new document vectors.
Never mix vectors from runtimes other than the pinned document/query pair.
Retain prior artifacts for rollback.

## Alternatives rejected

- Python query service on the host, this record's first decision: a second
  language, dependency set and image in production without measured benefit;
  banhmi's query embedder already used the Go stack.
- Document embedding in Go, with ONNX Runtime on the MacBook CPU or its CoreML
  provider: not measured; the owner keeps the measured MPS path, the fastest
  known, for the write path.
- Document embedding in FP16 on MPS: 2.6 times faster, but queries on the host
  CPU stay FP32, so the two sides would differ in precision. The owner chose one
  precision; ADR 0009 History has the measurement.
- ONNX Runtime through cgo inside the Go server: couples native model memory and
  cancellation to the API process.
- Rust service: an additional language without measured benefit.

## History

- 2026-11-09: the primary selected Python, uv and two startup runtimes for the
  index and evaluation work; the designer specified the HTTP contract.
- 2026-11-09: the owner moved the query runtime to Go, following banhmi's
  standalone Go embedder. Python keeps only document embedding on the MacBook.
  Recipe version fields became language-neutral. Document embedding moved to
  FP32 so both services use one precision.
