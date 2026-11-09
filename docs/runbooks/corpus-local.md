# Local corpus operations

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
