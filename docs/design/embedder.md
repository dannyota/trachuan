# Embedding service and stage

One Python HTTP service has two startup modes, as decided in
[ADR 0011](../adr/0011-python-embedder-service.md). The model recipe stays in
[ADR 0009](../adr/0009-dense-embedding-recipe.md); the service does not choose
models or change input text. Limits live in [budgets](budgets.md#indexing).

## Package and runtime

`apps/embedder/pyproject.toml` and `uv.lock` pin Python 3.12 and dependencies.
Use FastAPI, Uvicorn, NumPy, transformers and ONNX Runtime in the default
dependency set; optional group `document` adds PyTorch. Files under
`src/traquydinh_embedder/`: `app.py`, `admission.py`, `recipe.py`,
`tokenizer.py`, `runtime.py`, `onnx_runtime.py`, `torch_runtime.py`.
`__main__.py` only wires startup. `tests/` uses injected tokenizer, runtime and
clock objects.

Start with `uv run python -m traquydinh_embedder --runtime query --model-dir

<dir>` or `uv run --group document python -m traquydinh_embedder --runtime
document --model-dir <dir>`. Query loads `onnx/model.onnx` and its external data
with CPUExecutionProvider; document loads PyTorch weights on MPS in FP16.
Refuse unavailable MPS; never silently move document work to CPU. Convert CLS
output to float32, L2-normalize in float32, and return float32 vectors in both
modes. Fail on non-finite output or zero norm. Query uses one process with the
thread limits in budgets; document uses one model process and serialized GPU
inference. Neither mode downloads files at startup or enables remote code.

Provisioning supplies a local model bundle with model id, revision, file paths,
sizes and SHA-256 hashes. Verify every tokenizer, config, weight and external
ONNX data file before readiness. Resolve the ADR's revision prefix to a full
commit during provisioning and record both; refuse a prefix that resolves
ambiguously. No arbitrary file paths or model IDs arrive over HTTP.

## Recipe report

`GET /v1/recipe` returns `{protocol_version:1, recipe_hash, recipe}`. `recipe`
contains `model`, `revision`, `resolved_revision`, `tokenizer_revision`,
`tokenizer_sha256`, `weights_sha256`, `dimensions`, `pooling:"cls"`,
`normalization:"l2-float32"`, `input:"raw"`, `runtime:"query"|"document"`,
`backend:"onnxruntime-cpu"|"torch-mps"`, `precision:"fp32"|"fp16"`, `token_cap`,
`special_tokens:true`, `truncation:false`, and pinned Python, tokenizer-library
and runtime versions. Hash the fixed-field UTF-8 JSON without whitespace as
SHA-256. `weights_sha256` hashes the sorted file manifest.

Callers compare all fields with the expected runtime side of `dense_recipe`.
Document and query hashes intentionally differ; the common model, tokenizer,
pooling, normalization and dimensions must agree. `dense_recipe` holds both
reports. A response echoes `recipe_hash`; callers reject any change mid-run.
Recipe and model file checks happen once at startup and at client construction,
not by downloading data during a request.

## HTTP contract

All bodies are UTF-8 JSON; unknown fields, duplicate object keys, invalid UTF-8,
non-finite numbers and a wrong content type are errors. No streaming or OpenAI
compatibility endpoint. Every request supplies `recipe_hash` and `texts`, an
ordered nonempty array of strings. Query mode accepts exactly one text; document
mode accepts a bounded batch. Empty/whitespace-only text is invalid. Do not
trim, restore diacritics or remove instruction-looking text in service. The
caller constructs the exact ADR input.

`POST /v1/tokenize` returns `{recipe_hash,counts:[integer]}` in input order.
Counts include special tokens and exclude padding. Tokenization has truncation
disabled, even above the model cap; this endpoint lets the document client
classify oversize rows without running the model. Byte/body limits still apply.

`POST /v1/embed` returns:

```json
{
  "recipe_hash": "<sha256>",
  "dimensions": 1024,
  "data": [{ "index": 0, "token_count": 42, "embedding": [0.01, 0.02] }]
}
```

The example abbreviates the vector. Every successful result has the declared
dimension, a contiguous index and the actual unpadded token count. Count all
texts first. Reject the whole batch if any exceeds the cap; never truncate or
return partial vectors. Pad only to the longest accepted input in this batch.

Errors are `{error:{code,message,index:null|integer,token_count:null|integer}}`.
`message` is fixed text without submitted content. Codes and HTTP statuses:

| Status | Code                     | Meaning                                             |
| ------ | ------------------------ | --------------------------------------------------- |
| 400    | `invalid_request`        | JSON, fields, text or batch invalid                 |
| 409    | `recipe_mismatch`        | Expected hash differs                               |
| 413    | `body_too_large`         | Streaming byte cap exceeded                         |
| 415    | `unsupported_media_type` | Not JSON                                            |
| 422    | `input_too_long`         | Embed input exceeds token cap; index/count included |
| 429    | `busy`                   | Active/queue bounds or wait exhausted               |
| 503    | `not_ready`              | Loading, failed self-check or draining              |
| 504    | `deadline_exceeded`      | Request lifetime exceeded                           |
| 500    | `inference_failed`       | Runtime or invalid vector output                    |

`GET /healthz` returns 200 `{status:"alive"}` while the HTTP process runs.
`GET /readyz` returns 200 `{status:"ready",recipe_hash}` only after verification
and a fixed synthetic warmup, otherwise 503. Health routes have no inference
admission. Recipe may be read during load with 503 until verified.

Admission covers tokenization, inference and serialization, not just model
calls. Enforce body size while reading, before parsing. Query has the existing
active, queue, wait and total deadline from budgets; document has its own
limits. A timed-out/disconnected request drops its response but retains the
active slot until native inference actually finishes. A queue slot is removed on
cancellation. Do not start replacement work while the old computation runs.
There is no background unbounded executor queue and no cross-request batching.

Bind document mode to `127.0.0.1`; query mode binds only the container network.
The host plan owns network policy and the image. Disable access/body logging;
metrics contain code, runtime, counts and latency only. Health and recipe never
expose environment variables, paths or credentials.

## Embed stage

`apps/corpus/internal/stage/embed/` contains `embed.go`, `client.go`,
`artifact.go` and `rows.go`. Plan one item per done chunk artifact, same
document key. Input hash is that verified chunk artifact's manifest hash. Recipe
is `stage.RecipeHash("embed","dense/1", canonical_document_recipe_json)`; fields
include the complete document report above, ADR input concatenation, batch size
and artifact format version. Runtime and precision are mandatory.

In chunk ordinal order, construct
`strip(context_prefix) + "\n" + strip(content)`; strip means Unicode whitespace
at the edges. Ask `/v1/tokenize` in batches, omit rows above the document cap
with `embedding_input_too_long` gaps, then `/v1/embed` remaining rows in ordinal
order. Batch size is fixed by budgets as a maximum; pack fewer texts when the
encoded JSON body would exceed its byte cap. A single input above the byte cap
becomes `embedding_input_bytes_exceeded` with a null token count, since it was
never tokenized. No adaptive OOM fallback changes the recipe. Persist actual
token counts for all tokenized rows, including rejected ones. An empty chunk
artifact produces an empty embedding artifact without model calls, preserving
its coverage gaps.

Files: `vectors.f32`, `index.json`, `report.json`, `MANIFEST.sha256`.
`vectors.f32` is contiguous little-endian IEEE float32, row-major, no header.
`index.json` has `version:1`, `document_key`, `chunk_sha256`, `recipe_hash`,
`recipe`, `dimensions`, `rows:[{row,chunk_ordinal,token_count,input_sha256}]`,
`rejected:[{chunk_ordinal,token_count,kind}]`. Row numbers start at zero;
ordinals are increasing but may have rejected gaps. `input_sha256` hashes the
exact UTF-8 input. `report.json` has the gaps and counts, not copies of text.

Every chunk ordinal occurs exactly once in rows or rejected; no unknown ordinal
is allowed. Check byte size equals rows times dimensions times four, finite
components, and L2 norm within the tolerance in budgets. Manifest verification
precedes decoding. `Rows` is a no-op; `Resolve` regenerates the gaps. Valid rows
survive when another chunk is too long; the item is done with explicit gaps.

Recipe mismatch is an infrastructure failure of the batch, never an accepted
data gap. Timeout, 429, 503 and transport failure retry through the existing
stage policy with no nested retry loop. HTTP 400/409/415, malformed output or
non-finite vectors fail with a non-retryable infrastructure error. A 422 after
tokenize implies inconsistent service behavior and also fails. OOM/500 fails the
attempt; repeated failure stops at the existing attempt limit.

## Checks without model weights

Inject a deterministic tokenizer and runtime; unit tests never load or download
the model. A tiny locally generated ONNX tensor graph tests the adapter shape,
CLS selection, normalization and external error paths without production
weights. HTTP tests cover exact caps, batch atomicity, recipe mismatch,
cancellation, queue ownership and no truncation. Go uses a fake HTTP service for
artifact order, gap/retry classification and vector validation. A separate
opt-in Mac smoke with the pinned real bundle checks counts and query/document
parity; the primary records its output. Fake results are not quality evidence.

First-party references for the primary's version check:
[ONNX Python API](https://onnxruntime.ai/docs/api/python/api_summary.html),
[PyTorch MPS](https://docs.pytorch.org/docs/stable/notes/mps.html),
[tokenizer API](https://huggingface.co/docs/transformers/main_classes/tokenizer),
[uv groups](https://docs.astral.sh/uv/concepts/projects/dependencies/#dependency-groups).
Network access was forbidden for this design; these APIs remain unverified
against the proposed lockfile. Local spike evidence is linked from ADR 0009.
