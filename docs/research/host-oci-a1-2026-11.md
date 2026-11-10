# Query embedder on an OCI A1 host (2026-11-09)

First host measurement of the [ADR 0009](../adr/0009-dense-embedding-recipe.md)
query recipe. Not authority; the host plan and its acceptance gates decide.

## Setup

OCI `VM.Standard.A1.Flex`, 4 OCPU (Ampere Altra, Neoverse-N1 cores) and 24 GB,
Singapore, Oracle Linux 9.8. The embedder service ran from `apps/embedder` with
the pinned bundle downloaded from Hugging Face on the host; its recipe hash
equalled the committed query recipe (`f05c6020…`). Queries were the 92 authored
golden questions; the long input joined questions to 217 tokens.

## Results

Through the service (ONNX FP32, 2 intra-op threads, as in
[budgets](../design/budgets.md)):

| Measure                                        | p50      | p95    | p99    |
| ---------------------------------------------- | -------- | ------ | ------ |
| 92 questions, one at a time                    | 256 ms   | 326 ms | 402 ms |
| 2 requests per second for 120 s, 240 of 240 OK | 258 ms   | 334 ms | 376 ms |
| Long input (217 tokens)                        | 1,944 ms |        |        |

Resident memory stayed at 1.66 GB.

Direct ONNX Runtime timing (first 40 questions; long input 217 tokens):

| Model and threads                 | Short p50 / p95 | Long p50 | Mean cosine to FP32 |
| --------------------------------- | --------------- | -------- | ------------------- |
| FP32, 2 threads                   | 394 / 442 ms    | 2,736 ms | 1.0000              |
| FP32, 4 threads                   | 267 / 416 ms    | 1,727 ms | 1.0000              |
| Dynamic INT8 (default), 2 threads | 98 / 113 ms     | 695 ms   | 0.9796              |
| Dynamic INT8 (default), 4 threads | 57 / 99 ms      | 488 ms   | 0.9796              |

## Reading

- Typical questions meet the search gate with FP32 on this host; long inputs
  near the 256-token cap exceed the 2 s embedder deadline with 2 threads.
- Four threads cut long-input latency by about a third, at the cost of CPU the
  database also needs; INT8 is four to six times faster but its vectors drift
  (mean cosine 0.98 against FP32, below the 0.995 parity gate in ADR 0009), so
  it needs a retrieval evaluation before use.
- The host plan should choose among FP32 with 4 threads and a longer embedder
  deadline for long inputs, a 128-token query cap (ADR 0009 fallback), or an
  evaluated INT8 export, measuring search end to end with the database loaded.
