# llama.cpp against ONNX Runtime for query embedding (2026-11-10)

Measured on the OCI A1 host to decide whether llama.cpp should replace ONNX
Runtime for queries. Not authority; [ADR 0011](../adr/0011-embedder-services.md)
decides.

## Setup

OCI `VM.Standard.A1.Flex`, 4 OCPU (Neoverse-N1: `asimddp` and `asimdhp`, no
`i8mm` or `bf16`), 24 GB, Oracle Linux 9. Vietnamese_Embedding_v2 at revision
18b44161e041. ONNX Runtime 1.31.1 CPU with the service settings: 4 intra-op
threads, 1 inter-op, sequential, all graph optimizations, CPU arena off; INT8 is
`quantize_dynamic` QInt8 per-channel. llama.cpp b12244 (commit d5bbcd1), built
on the host with GCC 11.5 and `GGML_NATIVE=ON`; GGUF files converted from
`model.safetensors` with `convert_hf_to_gguf.py` as F32, F16 and Q8_0;
`llama-server --embedding --pooling cls -t 4 -tb 4 -c 512 -b 512 -ub 512`,
called over loopback HTTP.

Every variant received the same Hugging Face token ids. Short latency is the 92
golden questions one at a time after three warmups; the long input is 217
tokens, seven runs. Each variant ran twice, interleaved; the table shows the
second round. Recall uses the stored banhmi document vectors (MPS FP16) and the
96 golden citations, as in the [INT8 study](query-int8-2026-11.md). RSS is the
llama-server process, or the Python process for ONNX Runtime, which also holds
the tokenizer.

## Results

| Runtime, precision | Short p50 / p95 | Long p50 | recall@8 | MRR@8  | Cosine to ORT FP32, median / min | RSS     |
| ------------------ | --------------- | -------- | -------- | ------ | -------------------------------- | ------- |
| ONNX Runtime FP32  | 192 / 258 ms    | 1,234 ms | 76/96    | 0.5885 | 1 / 1                            | 1.64 GB |
| ONNX Runtime INT8  | 64 / 100 ms     | 373 ms   | 75/96    | 0.5886 | 0.9873 / 0.9756                  | 1.31 GB |
| llama.cpp F32      | 237 / 315 ms    | 1,970 ms | 76/96    | 0.5888 | 1.0000 / 1.0000                  | 2.33 GB |
| llama.cpp F16      | 594 / 789 ms    | 5,001 ms | 76/96    | 0.5888 | 1.0000 / 1.0000                  | 1.27 GB |
| llama.cpp Q8_0     | 82 / 107 ms     | 542 ms   | 74/96    | 0.5809 | 0.9994 / 0.9985                  | 1.07 GB |

Round one agreed within 7% except ONNX Runtime INT8 (80 ms short), whose first
run also quantized the model in the same process. The llama.cpp tokenizer
produced the Hugging Face ids for 92 of 92 questions.

## Reading

- ONNX Runtime INT8 is the fastest variant. llama.cpp Q8_0, the fastest
  llama.cpp variant, takes 28% longer on short queries and 45% longer on the
  long input.
- llama.cpp Q8_0 tracks FP32 more closely (cosine 0.9994 against 0.987) but
  finds one citation fewer and has lower MRR; quality does not favor it.
- llama.cpp F16 is 2.5 times slower than its F32 on this CPU; the earlier ONNX
  Runtime FP16 result (5 times slower) was not specific to one runtime.
- The ONNX Runtime F32 export and the llama.cpp F32 conversion give the same
  vectors, so the conversion is correct.

## Not measured

llama.cpp with `GGML_CPU_KLEIDIAI`, other compilers or flash-attention settings;
concurrent load; AmpereOne (A2 or A4) CPUs with `i8mm` and `bf16`.
