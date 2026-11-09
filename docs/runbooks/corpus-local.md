# Local corpus operations

## Start the local services

`make services-up` starts PostgreSQL, creates the `pipeline` and `temporal`
roles and their databases, creates the `corpus_builder` role and its
`corpus_template` database with `vector` and `ltree`, creates or upgrades the
Temporal schemas, starts the Temporal server (`127.0.0.1:7233`) and its UI
(`http://127.0.0.1:8233`), and creates the `corpus` namespace with 30-day
retention. Every step is idempotent. `.env` must hold `POSTGRES_PASSWORD`,
`PIPELINE_PASSWORD`, `TEMPORAL_DB_PASSWORD`, and `BUILDER_PASSWORD` (see
`.env.example`). `make db-down` stops everything and keeps the volumes.

Image tags, all published for arm64 and checked on 2026-11-09:
`temporalio/server:1.32.2`, `temporalio/admin-tools:1.32.2`,
`temporalio/ui:2.58.1`. The server reads its configuration from environment
variables and needs the dynamic config file mounted from
`deploy/local/temporal-dynamicconfig.yaml`.

## Run a release

Checkpoints need `pg_dump` 18 or newer (`brew install postgresql@18`).

```sh
set -a; . ./.env; set +a
export CORPUS_DATABASE_URL="postgres://pipeline:${PIPELINE_PASSWORD}@127.0.0.1:5432/workshop"
export CORPUS_CHECKPOINT_DIR=work/checkpoints
export CORPUS_PG_DUMP=/opt/homebrew/opt/postgresql@18/bin/pg_dump
export CORPUS_PG_RESTORE=/opt/homebrew/opt/postgresql@18/bin/pg_restore
export CORPUS_BUILDER_DATABASE_URL="postgres://corpus_builder:${BUILDER_PASSWORD}@127.0.0.1:5432/postgres"
export CORPUS_DATABASE_DISK_PATH=work
go run ./apps/corpus/cmd/corpus migrate
go run ./apps/corpus/cmd/corpus worker &
go run ./apps/corpus/cmd/corpus start vn 20261109 --until relate
go run ./apps/corpus/cmd/corpus status vn 20261109
```

Apply workshop migrations with `go run ./apps/corpus/cmd/corpus migrate` after
pulling new code and before restarting the worker; a missing table fails stage
commits with `relation "chunk" does not exist`. Start `make embedder-query` and
`make embedder-document` in separate terminals before extending a paused run.
Both use the model bundle in `EMBEDDER_MODEL_DIR`. Set `CORPUS_EVAL_FIXTURES` to
an adjudicated fixture directory before starting a run when the embedded
inventory does not match it. For a run paused by an older worker before the
index stages were registered, set the fixture directory before extending the
run. Restart the worker with the same settings and extend the run:

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
