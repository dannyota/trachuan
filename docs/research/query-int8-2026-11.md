# INT8 query embedding for the four finalist models (2026-11-10)

Measured on the OCI A1 host to choose a fast query precision. Not authority;
[ADR 0009](../adr/0009-dense-embedding-recipe.md) decides.

## Setup

OCI `VM.Standard.A1.Flex` (Neoverse-N1), ONNX Runtime 1.31.1 CPU, 4 intra-op
threads, 1 inter-op, sequential. Models at the revisions pinned in the embedder
spike; Qwen-family models exported with optimum (opset 17), each FP32 export
reproducing the spike's recall exactly. Query vectors were computed on the host,
because ARM integer kernels differ from the Mac (Vietnamese_Embedding_v2 dynamic
INT8: median cosine 0.989 on the Mac, 0.98 per-tensor on the host). Recall uses
the stored banhmi document vectors (MPS FP16) and the spike's 92 golden
questions with 96 citations. Short latency is over the 92 questions one at a
time; the long input is a 900-character text (217 tokens for
Vietnamese_Embedding_v2, 251-254 for Qwen-family models with their prompt).

Variants: dynamic INT8 (`quantize_dynamic`, QInt8, per-channel), and 8-bit
weight-only with integer compute (`MatMulNBits`, bits 8, block 128, accuracy
level 4, "q8w_a4").

## Results

| Model, variant                    | recall@8 | MRR@8  | Cosine to FP32, median / min | Short p50 / p95 | Long p50 | RSS     |
| --------------------------------- | -------- | ------ | ---------------------------- | --------------- | -------- | ------- |
| Vietnamese_Embedding_v2 FP32      | 76/96    | 0.5885 | 1 / 1                        | 207 / 347 ms    | 1,214 ms | 1.60 GB |
| Vietnamese_Embedding_v2 INT8 p-ch | 75/96    | 0.5886 | 0.9873 / 0.9756              | 65 / 86 ms      | 387 ms   | 0.83 GB |
| Vietnamese_Embedding_v2 q8w_a4    | 74/96    | 0.5872 | 0.9983 / 0.9972              | 76 / 104 ms     | 488 ms   | 1.05 GB |
| Vietnamese_Embedding_v2 INT8 p-t  | 72/96    | 0.5640 | 0.9786 / 0.9675              | 67 / 81 ms      | 415 ms   | 1.18 GB |
| Qwen3-Embedding-0.6B FP32         | 69/96    | 0.4922 | 1 / 1                        | 433 / 538 ms    | 1,859 ms | 1.92 GB |
| Qwen3-Embedding-0.6B INT8 p-ch    | 51/96    | 0.3844 | 0.8174 / 0.5140              | 146 / 205 ms    | 1,057 ms | 0.63 GB |
| Qwen3-Embedding-0.6B q8w_a4       | 69/96    | 0.4867 | 0.9982 / 0.9960              | 262 / 322 ms    | 1,114 ms | 0.91 GB |

p-ch is per-channel, p-t per-tensor. Harrier models have document vectors only
for the 28,632-chunk screening corpus, so their recall is an upper bound:

| Model, variant (screening corpus) | recall@8 | MRR@8  | Cosine median / min | Short p50 | Long p50 |
| --------------------------------- | -------- | ------ | ------------------- | --------- | -------- |
| harrier-oss-v1-0.6b FP32          | 74/96    | 0.6433 | 1 / 1               | 419 ms    | 2,021 ms |
| harrier-oss-v1-0.6b INT8 p-ch     | 74/96    | 0.6205 | 0.9438 / 0.8004     | 156 ms    | 921 ms   |
| harrier-oss-v1-0.6b q8w_a4        | 74/96    | 0.6473 | 0.9995 / 0.9990     | 215 ms    | 1,185 ms |
| vietlegal-harrier-0.6b FP32       | 69/96    | 0.5583 | 1 / 1               | 424 ms    | 1,922 ms |
| vietlegal-harrier-0.6b INT8 p-ch  | 62/96    | 0.4997 | 0.9003 / 0.6013     | 152 ms    | 765 ms   |
| vietlegal-harrier-0.6b q8w_a4     | 70/96    | 0.5580 | 0.9992 / 0.9981     | 193 ms    | 840 ms   |

On the screening corpus Vietnamese_Embedding_v2 INT8 per-channel scores the same
75/96 and 0.5886 as on the full corpus.

## Reading

- Vietnamese_Embedding_v2 with per-channel dynamic INT8 loses one citation and
  no MRR, runs about 3.2 times faster than FP32 and halves resident memory. Its
  cosine to FP32 is below the 0.995 parity gate, so recall, not parity, is the
  evidence.
- Per-tensor dynamic INT8, the default, loses four citations; do not use it.
- Dynamic INT8 breaks the Qwen-family models on ARM; only q8w_a4 holds their
  quality, and it is 3 to 4 times slower than Vietnamese_Embedding_v2 INT8.
- Changing the document model would also force a full re-embed, about 20 minutes
  for Vietnamese_Embedding_v2 on MPS FP16 and five or more times longer for
  Qwen-family models.

## Go service on the A1 host (2026-11-11)

The Go query service (`apps/queryembed` at e25b097, ONNX Runtime 1.31.1, INT8
per-channel, 4 intra-op threads, arena off, admission 2 active and 8 queued) on
the same `VM.Standard.A1.Flex` (4 Neoverse-N1 CPUs, Oracle Linux 9.8, kernel
6.12, Go 1.27.3), measured over loopback HTTP with nothing else running. The
host build passes the `ort` tests and reports the pinned recipe `63ac35af…`;
startup to ready takes 4.6 s.

| Load                                      | Requests                | p50    | p95    | p99    | Max    |
| ----------------------------------------- | ----------------------- | ------ | ------ | ------ | ------ |
| 92 golden questions, one at a time        | 92, all 200             | 83 ms  | 131 ms | 155 ms | 155 ms |
| 217-token text, seven runs                | 7, all 200              | 485 ms |        |        | 607 ms |
| 2 clients without pause, 300 s            | 3,669 (12.2/s), all 200 | 160 ms | 233 ms | 273 ms | 346 ms |
| 10 clients without pause, 60 s (200 only) | 736 × 200, 1,237 × 429  | 389 ms | 472 ms | 504 ms | 537 ms |

The burst produced no 504 and no other status: admission rejected the excess at
once. Through HTTP the service is about 20 ms slower than direct ONNX Runtime
(64 ms short, 373 ms long). RSS was 2.35 GB right after startup and peaked at
2.51 GB (VmHWM) after the sequential run, fell to 1.81 GB a minute later and
then stayed between 1.21 and 1.28 GB through the stress and the burst, under the
2.0 GB gate. The early peak is the verified 570 MB model buffer kept until the
Go runtime returned it. With `debug.FreeOSMemory` after warmup (d2e74cb) RSS is
1.07 GB at ready and for the next minute and 1.17 GB after a 60-second stress
with two clients (799 requests, all 200); model load still peaks at about 2.4 GB
(VmHWM 2,424,064 kB) for under 5 seconds before ready. Raw files are kept
outside the repository.

## Not measured

Full-corpus recall for the Harrier models, Go service latency with PostgreSQL
under load at the same time, static or calibrated INT8, and evaluation on the
trachuan corpus through the eval stage.
