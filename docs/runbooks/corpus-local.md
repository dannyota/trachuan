# Local corpus operations

## Start the local services

Local development runs natively, without containers: the MacBook and the host
are both arm64. Install `brew install postgresql@18 pgvector temporal` once.
`make services-up` starts PostgreSQL 18 on `127.0.0.1:25433` with its data in
`~/.local/share/trachuan-pg18` (initialized on first start with the `trachuan`
superuser), creates the `pipeline` and `corpus_builder` roles, the `workshop`
database and the `corpus_template` database with `vector` and `ltree`, and
starts the Temporal development server on `127.0.0.1:27233` (UI
`http://127.0.0.1:28233`) with its state in a SQLite file under
`~/.local/share/trachuan-temporal` and the `corpus` namespace at 30-day
retention. Every step is idempotent. `.env` must hold `POSTGRES_PASSWORD`,
`PIPELINE_PASSWORD` and `BUILDER_PASSWORD` (see `.env.example`).
`make services-down` stops both and keeps their data. Point `CORPUS_PG_DUMP` and
`CORPUS_PG_RESTORE` at `/opt/homebrew/opt/postgresql@18/bin`.

## Run a release

Checkpoints need `pg_dump` 18 or newer (`brew install postgresql@18`).

The OCR cross-check renders PDF pages with `pdftoppm` from poppler
(`brew install poppler`; 26.11.0 at the time of writing). Only
`corpus ocr check` needs it; the worker does not. Set `CORPUS_OCR_STORE` to the
OCR store directory for the `corpus ocr` commands and for a worker that should
read checked OCR text.

```sh
set -a; . ./.env; set +a
export CORPUS_DATABASE_URL="postgres://pipeline:${PIPELINE_PASSWORD}@127.0.0.1:25433/workshop"
export CORPUS_CHECKPOINT_DIR=work/checkpoints
export CORPUS_PG_DUMP=/opt/homebrew/opt/postgresql@18/bin/pg_dump
export CORPUS_PG_RESTORE=/opt/homebrew/opt/postgresql@18/bin/pg_restore
export CORPUS_BUILDER_DATABASE_URL="postgres://corpus_builder:${BUILDER_PASSWORD}@127.0.0.1:25433/postgres"
export CORPUS_DATABASE_DISK_PATH=work
export CORPUS_PHAPDIEN_PACKAGE=work/phapdien/BoPhapDienDienTu.zip
go run ./apps/corpus/cmd/corpus migrate
go run ./apps/corpus/cmd/corpus worker &
go run ./apps/corpus/cmd/corpus start vn 20261109 --until relate
go run ./apps/corpus/cmd/corpus status vn 20261109
```

Fetch the Pháp điển package with `scripts/phapdien-fetch.sh` once; the worker
reads `taxonomy/` and refuses to start when the package is missing or its hash
differs from `taxonomy/mapping.yaml`.

Apply workshop migrations with `go run ./apps/corpus/cmd/corpus migrate` after
pulling new code and before restarting the worker; a missing table fails stage
commits with `relation "chunk" does not exist`. Start `make queryembed-run` and
`make docembed-run` in separate terminals before extending a paused run.
`make queryembed-run` builds the query service with the Go version pinned to its
recipe (`GOTOOLCHAIN`), whatever `go` is on PATH; a service started another way
with a different Go version reports a different recipe hash and build and search
refuse it. Both use the model bundle in `EMBEDDER_MODEL_DIR`. Provision it with
`uv run python scripts/make_bundle.py <dir> --model AITeamVN/Vietnamese_Embedding_v2 --revision 18b44161e041bf1d3a333ab5144b5b7b93f914d2`
in `apps/docembed`; it writes the INT8 query model `onnx/model_int8.onnx` from
`onnx/model.onnx` (about 12 seconds and 9.5 GB peak memory) before the manifest.
Set `CORPUS_EVAL_FIXTURES` to an adjudicated fixture directory before starting a
run when the embedded inventory does not match it. For a run paused by an older
worker before the index stages were registered, set the fixture directory before
extending the run. Restart the worker with the same settings and extend the run:

```sh
go run ./apps/corpus/cmd/corpus start vn 20261109
go run ./apps/corpus/cmd/corpus status vn 20261109
go run ./apps/corpus/cmd/corpus eval vn 20261109 --out work/eval
```

Add `--previous <report.json>` to compare a previous report or
`--require-floors` to return exit status 2 when a floor fails or is unavailable.

`corpus cancel`, `corpus gaps`, `corpus accept-gaps --kind <kind> --note <text>`
and `corpus export-history` take the same country and release. Run every command
from the repository root: `start` reads `sources/<cc>.yaml` there and artifacts
default to `work/artifacts`.

Review `sources/vn-corrections.yaml` before a build; a missing file means no
corrections. The build pins the parsed entries at planning. Review
`validity-review.json` in the build artifact for unresolved validity and
proposed corrections before accepting the release.

## Source settings and pauses

The secondary source defaults to
`CORPUS_CHINHPHU_URL=https://vanban.chinhphu.vn` and
`CORPUS_CHINHPHU_FILE_URL=https://datafiles.chinhphu.vn`. Its request rate and
concurrency default to 1, set by `CORPUS_CHINHPHU_RPS` and
`CORPUS_CHINHPHU_IN_FLIGHT`. `CORPUS_CHINHPHU_LOOPBACK_HTTP=true` permits
127.0.0.1 HTTP endpoints for tests and proofs only.

`corpus status <cc> <rel>` lists paused sources and their reasons. A pause
survives worker restarts and blocks the run. After resolving the cause and
ensuring no workflow is open, clear the pause on an unreleased run:

```sh
corpus source-resume <cc> <rel> --source <name> --note <reason>
corpus start <cc> <rel>
```

Resume records the note and prints the previous pause reason. It does not start
work. An already active source needs no change.

## Restore from a checkpoint

Stop the corpus worker. Choose a completed checkpoint directory under
`$CORPUS_CHECKPOINT_DIR/<cc>-<rel>/<generation>-<stage>/`, identified by its
`checkpoint.json`. Temporal persistence is not included in the checkpoint.

Set `CHECKPOINT` to the selected checkpoint directory. Create a fresh `workshop`
database using the local PostgreSQL administration connection, then restore the
selected dump into it:

```sh
createdb workshop
pg_restore --dbname workshop "$CHECKPOINT/workshop.dump"
```

Use the local PostgreSQL connection settings and credentials for both commands.

Copy the checkpoint's shared `artifacts/` tree back under `CORPUS_ARTIFACTS`,
preserving the stage and artifact directory names:

```sh
mkdir -p "$CORPUS_ARTIFACTS"
cp -R "$CORPUS_CHECKPOINT_DIR/artifacts/." "$CORPUS_ARTIFACTS/"
```

Start the worker, recover the restored run, and resume it:

```sh
corpus recover <cc> <rel> [--terminate]
corpus start <cc> <rel>
```

Use `--terminate` when an open Temporal workflow for that run must be
terminated.

## Query service

Run `make queryembed-native` to install the pinned native libraries, then run
`make queryembed-run` with the bundle at `EMBEDDER_MODEL_DIR` (default
`work/models/vi_v2-bundle`). The service listens on `127.0.0.1:8091`.

The binary accepts `--model-dir DIR`, `--ort-lib FILE`, and `--listen ADDR`. The
first two flags are required; `--listen` defaults to `127.0.0.1:8091`.

## Fresh OCR

Install trusted Poppler tools as described in the OCR cross-check section. Set
`CORPUS_OCR_STORE`, `GOOGLE_APPLICATION_CREDENTIALS` (an ADC credentials file
outside the repository) and `GOOGLE_CLOUD_PROJECT` (the billing project). Use
the workshop database and artifact variables above. Credentials come only from
the named readable regular file. Vision and token requests ignore proxy
variables and refuse redirects. Token acquisition stops on cancellation or after
30 seconds.

```sh
corpus ocr vision <cc> <rel> --max-pages 100 --dry-run
corpus ocr vision <cc> <rel> --max-pages 100 --limit-docs 5
```

Dry run counts verified PDFs without rendering or credentials. The live command
sends one grayscale JPEG page at 200 dpi and quality 90 per request to Google
Cloud Vision, with language hint `vi`. Only PDFs in verified fetch bundles of
the run are sent. The request limit includes retries. Rendering uses at most 256
MB of scratch space; each page is at most 7 MB. At most eight page requests run
at once; joined text is at most 50 MB. Use `--pdf SHA256` for a comparison
beside imported text. Fresh text still needs `corpus ocr check` before extract
can use it; imported text keeps priority when both entries exist.

## OCR cross-check pass

Pass the Claude key in each command's environment, never export it for the
session. Start the version 3 pass with ten documents:

```sh
ANTHROPIC_API_KEY='<key for this command>' corpus ocr check vn 20990101 \
  --model claude-opus-5-5 --effort high --check-version 3 \
  --limit-docs 10 --max-usd 10
```

Use the intended run's country and release. Version 3 sends JPEG pages at 200
dpi with eight documents in flight. Review the cost and verdict lines before
continuing with a higher cumulative `--max-usd` under the same checker id.
`--dry-run` renders and counts tokens but sends no generation request and writes
nothing. Versions 3 and 4 refuse `--batch`. If the owner chooses the 150 dpi
fallback, use `--check-version 4` for the entire pass; each version keeps its
own results and cumulative spend.

```sh
corpus ocr review vn 20990101 --model claude-opus-5-5 --effort high \
  --check-version 3
```
