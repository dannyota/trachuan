# Embedding services and stage

Two services implement one HTTP contract, as decided in
[ADR 0011](../adr/0011-embedder-services.md): the Go query service serves the
read path on the host, and the Python document service embeds chunks for the
write path on the MacBook GPU. The model recipe stays in
[ADR 0009](../adr/0009-dense-embedding-recipe.md); neither service chooses
models or changes input text. Limits live in [budgets](budgets.md#indexing).

## Query service (Go)

`apps/queryembed/` is part of the root Go module. `cmd/queryembed/main.go` only
wires startup. Packages under `apps/queryembed/internal/`: `httpapi` (routes,
strict JSON, errors), `admission`, `bundle` (manifest and hash checks),
`tokenizer` and `onnx` (session, CLS selection, normalization). Recipe and
protocol types come from root `internal/embedding`.

`tokenizer` uses `github.com/daulet/tokenizers`, the Rust tokenizers library
linked statically through cgo as `libtokenizers.a`. `onnx` uses the ONNX Runtime
Go binding, module `github.com/microsoft/onnxruntime/go`, taken from the owner's
fork `github.com/dannyota/onnxruntime/go` at a pinned tag through a `replace`
until upstream tags releases. The binding loads the ONNX Runtime shared library
at startup from a configured path. Both packages build only with tag `ort`;
without it they compile to stubs whose constructors fail, so `go build ./...`,
`go test ./...` and lint need no native libraries. Changing the ONNX Runtime or
tokenizers version changes the query recipe.

`apps/queryembed/native.lock` pins the release assets for `darwin-arm64` and
`linux-arm64`: the ONNX Runtime shared library and `libtokenizers.a`, each with
version, URL and SHA-256. `make queryembed-native` fetches and verifies them
into `work/native/<os>-<arch>/`; the image build uses the same pins. Nothing is
downloaded at runtime.

Start with `queryembed --model-dir DIR --ort-lib FILE --listen ADDR`. Startup
order:

1. Verify the model bundle.
2. Load `tokenizer.json` with truncation and padding disabled. Prove it by
   counting a fixed synthetic input longer than the cap; the count must exceed
   the cap.
3. Open `onnx/model.onnx` with the CPU provider, the thread settings from
   budgets and the CPU memory arena disabled (banhmi measured 200 to 400 MB less
   resident memory at query rates). Refuse a model whose inputs are not exactly
   int64 `input_ids` and `attention_mask`, or whose first output is not float32
   `[batch, sequence, dimensions]`.
4. Run the fixed warmup, then report ready.

A query request carries one text, so there is no padding. Take position 0 of the
first output (CLS), L2-normalize in float32, and fail on non-finite output or
zero norm. Tokenizer calls are serialized behind one mutex because the cgo
wrapper documents no thread safety; the binding documents `Session.Run` as safe
for concurrent use. When a request deadline expires, answer 504 and set the
run's terminate flag through its run options; the active slot stays held until
`Run` returns.

Go's `encoding/json` accepts duplicate object keys and replaces invalid UTF-8;
the decoder rejects both explicitly. On the host the service binds only the
container network; locally it binds `127.0.0.1`.

## Document service (Python)

`apps/docembed/pyproject.toml` and `uv.lock` pin Python 3.14 and FastAPI,
Uvicorn, NumPy, transformers and PyTorch. Files under `src/trachuan_docembed/`:
`app.py`, `admission.py`, `bundle.py`, `recipe.py`, `tokenizer.py`,
`torch_runtime.py`; `__main__.py` only wires startup. `tests/` uses injected
tokenizer, runtime and clock objects. Start with
`uv run python -m trachuan_docembed --model-dir DIR`. It runs on the MacBook
only and binds `127.0.0.1`.

It loads the PyTorch weights on MPS in FP32. Refuse unavailable MPS; never
silently move work to CPU. Take the CLS output, L2-normalize in float32 and
return float32 vectors; fail on non-finite output or zero norm. One model
process runs serialized GPU inference. It downloads nothing at startup and never
enables remote code.

## Model bundle

Provisioning supplies a local model bundle with model id, revision, file paths,
sizes and SHA-256 hashes. Each service verifies every tokenizer, config, weight
and external ONNX data file it loads before readiness. Resolve the ADR's
revision prefix to a full commit during provisioning and record both; refuse a
prefix that resolves ambiguously. No arbitrary file paths or model IDs arrive
over HTTP.

## Recipe report

`GET /v1/recipe` returns `{protocol_version:1, recipe_hash, recipe}`. `recipe`
contains, in this order, `model`, `revision`, `resolved_revision`,
`tokenizer_revision`, `tokenizer_sha256`, `weights_sha256`, `dimensions`,
`pooling:"cls"`, `normalization:"l2-float32"`, `input:"raw"`,
`runtime:"query"|"document"`, `backend:"onnxruntime-cpu"|"torch-mps"`,
`precision:"fp32"`, `token_cap`, `special_tokens:true`, `truncation:false`,
`language`, `tokenizer_library` and `runtime_library`. The last three record
what the service loaded as `<name> <version>`: for example `go 1.27.3`,
`daulet/tokenizers 1.27.0` and `onnxruntime 1.31.1` for the query service;
`python 3.14.x`, `transformers 5.22.2` and `torch 2.15.0` for the document
service. Hash the fixed-field UTF-8 JSON without whitespace as SHA-256.
`weights_sha256` hashes the sorted file manifest.

Callers compare all fields with the expected runtime side of `dense_recipe`.
Document and query hashes intentionally differ; the common model, tokenizer,
pooling, normalization, precision and dimensions must agree. `dense_recipe`
holds both reports. A response echoes `recipe_hash`; callers reject any change
mid-run. Recipe and model file checks happen once at startup and at client
construction, not by downloading data during a request.

## HTTP contract

All bodies are UTF-8 JSON; unknown fields, duplicate object keys, invalid UTF-8,
non-finite numbers and a wrong content type are errors. No streaming or OpenAI
compatibility endpoint. Every request supplies `recipe_hash` and `texts`, an
ordered nonempty array of strings. The query service accepts exactly one text;
the document service accepts a bounded batch. Empty/whitespace-only text is
invalid. Do not trim, restore diacritics or remove instruction-looking text in
service. The caller constructs the exact ADR input.

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
calls. Enforce body size while reading, before parsing. The query service has
the active, queue, wait and total deadline from budgets; the document service
has its own limits. A timed-out/disconnected request drops its response but
retains the active slot until native inference actually finishes. A queue slot
is removed on cancellation. Do not start replacement work while the old
computation runs. There is no background unbounded executor queue and no
cross-request batching.

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

Unit tests never load or download production weights. Go tests inject a fake
tokenizer and runtime and cover the HTTP contract: exact caps, recipe mismatch,
strict JSON, cancellation, queue ownership and no truncation. Tests tagged `ort`
load a tiny ONNX graph and a tiny tokenizer file, committed under `testdata/`
with the script that generated them, through the real native libraries; they
check input names, CLS selection, normalization and error paths. CI runs them in
a job that fetches the pinned native assets. Python tests inject tokenizer,
runtime and clock objects and cover the same contract for document batches. The
corpus pipeline uses a fake HTTP service for artifact order, gap and retry
classification and vector validation.

An opt-in Mac smoke with the pinned real bundle runs both services and checks
token counts and query/document parity; the primary records its output. Fake
results are not quality evidence.

First-party references:
[ONNX Runtime C API](https://onnxruntime.ai/docs/api/c/),
[ONNX Runtime Go binding](https://github.com/microsoft/onnxruntime/tree/main/go),
[daulet/tokenizers](https://github.com/daulet/tokenizers),
[PyTorch MPS](https://docs.pytorch.org/docs/stable/notes/mps.html),
[tokenizer API](https://huggingface.co/docs/transformers/main_classes/tokenizer).
