# Corpus release

Decision: [ADR 0006](../adr/0006-single-host-oci-podman.md) (History
2026-11-14). An evaluated corpus build reaches production with no SSH session:
the MacBook uploads it, a GitHub workflow checks and signs it, and a host timer
restores and activates it. Code and images follow
[pull deployment](pull-deployment.md).

## Upload

`scripts/corpus-upload.sh <cc> <rel>` runs on the MacBook after `corpus eval`
and uploads to bucket `trachuan-releases`, prefix `<cc>/<rel>/`:

- `corpus.dump`: the build's verified dump.
- `MANIFEST.sha256`: the build artifact manifest
  ([pipeline](pipeline.md#items-artifacts-and-gaps)).
- `report.json`: the evaluation report ([evaluation](evaluation.md)), run with
  `--previous` set to the report of the release active in production.
- `release.json`: the dump's `release` row as JSON
  ([corpus schema](corpus-schema.md)), written by `corpus release-row`.

It uses AWS CLI v2 against the
[S3 compatibility endpoint](deployment.md#object-storage) with the
`trachuan-release-upload` customer secret key from the owner's credential store,
path-style addressing and `AWS_REQUEST_CHECKSUM_CALCULATION=when_required`,
since recent AWS CLI versions add checksum headers by default
([AWS SDK data integrity](https://docs.aws.amazon.com/sdkref/latest/guide/feature-dataintegrity.html)).
It refuses before upload unless the run's expected items are all done or
accepted gaps (the workshop database is the only place gap acceptance exists),
`report.json` has `quality_pass: true`, no research override and a
`previous_diff`, and its `dump_sha256` equals the local dump. A prefix that
already holds objects is never overwritten; a rebuilt corpus gets a new `rel`.
Then it dispatches `corpus-release.yml` with `country` and `rel`.

## Signing workflow

`.github/workflows/corpus-release.yml`: `workflow_dispatch` from `main`,
environment `corpus-release`, which holds the `trachuan-release-verify`
read-only customer secret key. That key is the only OCI credential in GitHub; it
reads one bucket of public legal text and cannot write. The job:

1. Downloads the four objects into the runner.
2. Requires SHA-256 of `MANIFEST.sha256` to equal the report's
   `build_artifact_sha256`, the manifest's `corpus.dump` line and the report's
   `dump_sha256` to equal the downloaded dump's SHA-256, and records its size.
3. Requires the report's `country` and `rel` to equal the inputs, `status`
   completed, `quality_pass: true`, no research override, a `previous_diff`
   naming the release it was compared with, every gating `non_regression` entry
   in every cohort with status `pass` (`not_run` fails), and its `recipes` to
   equal the recipes in `release.json`.
4. Writes the corpus manifest below as canonical JSON and signs it with
   `cosign sign-blob --bundle`, keyless, like the image manifest.
5. Pushes `manifest.json` and `manifest.sigstore.json` to
   `ghcr.io/dannyota/trachuan-corpus-release`, tagged `<cc>-<rel>` and
   `<cc>-production`.

`release_eligible` in the report stays false; the signed corpus manifest is the
release authorization. Signing an older `rel` again gives it a new sequence,
which is how a corpus rollback is requested. No MacBook signing key exists.

### Corpus manifest

```json
{
  "kind": "trachuan.corpus-release", "version": 1,
  "repository": "dannyota/trachuan", "commit": "<workflow commit>",
  "country": "vn", "rel": "20261114_3", "database": "corpus_vn_20261114_3",
  "sequence": 57, "previous": "20261112_2",
  "dump": {"object": "vn/20261114_3/corpus.dump", "sha256": "<64 hex>",
           "size": 1234567890},
  "report": {"sha256": "<64 hex>", "quality_pass": true},
  "build_artifact_sha256": "<64 hex>",
  "schema_version": 6,
  "extensions": {"vector": "<version>", "ltree": "<version>"},
  "recipes": {"dense": {...}, "chunk": {...}, "lexical": {...},
              "retrieval": {...}}
}
```

`sequence` is `github.run_number` of `corpus-release.yml`. `previous` is the
release in the report's `previous_diff`. `extensions` comes from `release.json`,
where `corpus release-row` records the build database's `pg_extension` names and
versions; the host requires exactly the names `vector` and `ltree` and logs the
versions, and the smoke check in step 3 proves the restored data works with the
image's versions. The certificate identity is
`https://github.com/dannyota/trachuan/.github/workflows/corpus-release.yml@refs/heads/main`.

## Host activation

The [updater](pull-deployment.md#updater) checks
`ghcr.io/dannyota/trachuan-corpus-release:<cc>-production` on every run, under
the same locks, so image and corpus changes never overlap.

1. Verify with `cosign verify-blob --bundle`, the identity from policy and the
   transparency log proof, then apply the
   [acceptance rule](pull-deployment.md#acceptance) for corpus sequences.
2. Choose the mode: `retained` when `database` is listed in `retired_release`
   and its recorded dump SHA-256 equals the manifest's; refuse when the name is
   listed with another digest; else `new`. `new` also requires `previous` to
   equal the active release, so every newly restored release was compared with
   the release it replaces; `retained` is a rollback within the seven-day window
   and is exempt.
3. Check compatibility: `podman run --rm <server image> server release-check`
   prints the schema versions and recipes the deployed server serves and the
   running query embedder reports. A recipe difference is status `waiting`
   ([recipe changes](#recipe-changes)), alarmed after 24 hours. Refuse below the
   free memory and disk thresholds in
   [host budgets](budgets-host.md#host-memory-estimates).
4. `new`: download the dump through the instance principal with the OCI Python
   SDK into `/srv/trachuan/data/releases/<cc>-<rel>/`; require size and SHA-256
   to equal the manifest.
5. Run the activation steps below; on success delete the local dump and write
   the result.

### Activation steps

The host activator lives under `deploy/host/release/` and works through
`podman exec` into `trachuan-postgres`. Privileged preparation runs as
`postgres` through `postgres-admin.sh`; everything else runs as `activator` (ADR
0003; [system](system.md#database-roles)), which has `CREATEDB`, owns the corpus
databases it creates, and may write only `corpus_release`, `release_journal` and
`retired_release` in `auth`. The updater runs the idempotent
`provision-activator.sh` as `postgres` before each activation: it creates the
role with `CREATEDB` when missing and sets its password from the Vault secret
`activator_db_password`, so an existing cluster gets the role without
reinitialization. The table grants belong to the auth migrations, which run
after it and create `release_journal` and `retired_release`.

0. Take the `activation` advisory lock in `auth`; append `release_journal`
   (`started`, mode, target database, manifest hash, dump SHA-256, expected
   current `active_db`). Every later step appends its own row.
1. `new` only:
   - List the TOC. Refuse functions, event triggers, and extensions other than
     `vector` and `ltree` at the versions in the manifest, which must equal the
     versions the PostgreSQL image provides.
   - As `activator`: create the database.
   - As `postgres`: create `vector` and `ltree` in it.
   - As `activator`: `pg_restore --no-owner --no-privileges --exit-on-error`
     with a TOC list (`-L`) that omits every extension entry and every entry
     that belongs to an extension, including `COMMENT ON EXTENSION`, with
     `maintenance_work_mem=2GB` and two maintenance workers.
   - As `postgres`: run `grant-corpus.sh`.
2. Require the restored `release` row to equal the manifest's `rel`,
   `schema_version` and recipes; otherwise abort.
3. As `postgres`, prewarm the target's tables, indexes and TOAST relations with
   `pg_prewarm`, creating and dropping the extension as the
   [runbook](../runbooks/release-restore.md#prewarm-a-corpus-database) does.
   Then `server release-check --database <target>` runs one search and one read
   against it with the server code that will serve it.
4. In one transaction: compare-and-swap `corpus_release.active_db` from the
   journaled expected value, insert the old database into `retired_release` with
   its dump SHA-256 and `retain_until` seven days ahead, delete the target from
   `retired_release` in `retained` mode, and append `activated`.
5. The server resolves the release once per request, so traffic moves without a
   restart; the postings for the new release load on first use under the
   server's existing gate ([sparse postings](sparse-postings.md)).

`release_journal` and `retired_release` (with `dump_sha256`) are auth
migrations. The old active database gets its dump digest from its active corpus
manifest, or NULL when restored by hand; that row stays protected for seven days
and cannot be reactivated by the pipeline, so rollback within that window is the
owner's manual compare-and-swap of `corpus_release.active_db`. Two rules close
recovery and cleanup:

- A journal whose last row is `activated` or `reverted` is complete; the next
  run only finishes local bookkeeping.
- A database is protected when it is `active_db`, the target of an open
  activation journal, or in `retired_release` with `retain_until` in the future.
  No step, abort or prune ever drops a protected database, whatever the journal
  says.

After a crash, an open journal continues from its last step when the guard holds
(`new`: the target is neither active nor retired; `retained`: it is retired and
not active). Otherwise the journal is closed as aborted, and a `new` target is
dropped subject to the rule above. A daily `prune` under the same lock handles
each unprotected database in `retired_release` without connections: it journals
`pruning`, drops the database if it exists, deletes the row, and journals
`pruned`. An open `pruning` journal is finished the same way on the next run.

## Recipe changes

The pair activation path is not yet implemented; recipe mismatches wait and
image manifests with a non-null `corpus_release` are refused.

A release whose recipes differ from the running server waits until the
[image manifest](pull-deployment.md#image-manifest) for the matching code names
it in `corpus_release`. The updater activates the pair in one locked run:

1. Verify both manifests and the images. Run activation steps 0 to 2 for the
   corpus while the old pair serves.
2. Stop Caddy; activate the images up to started containers, without the
   readiness gate.
3. Run step 3 with the new server image, then step 4, then require loopback
   `/readyz`, start Caddy and run the edge probe; a failed probe stops Caddy.
4. On failure after step 4, one transaction reverses it: compare-and-swap the
   pointer back, delete the old release's `retired_release` row, insert the new
   one with its digest and `retain_until` seven days ahead, and journal
   `reverted`. Then restore the previous images under the
   [failure rule](pull-deployment.md#failure-and-recovery) and start Caddy once
   the previous pair passes loopback readiness, then run the edge probe; a
   failed probe stops Caddy and records the failure. An interrupted pair run is
   recovered by the same rule: its image journal is treated as failed, and a
   corpus journal ending in `activated` for that pair is reversed first.

## Results and alarms

The result goes to `/srv/trachuan/state/deploy/release.json` with the fields of
the deploy result plus `rel` and mode. The metrics timer publishes
`release_failed` and `release_waiting_age_seconds`; alarms on topic
`trachuan-alarms` email the owner when `release_failed` is 1 or a release has
waited more than 24 hours. Logs go to `/var/log/trachuan/release.log`.

## Proofs

Unit tests: a wrong workflow identity, an unprotected signer (a dispatch from a
branch), a dump hash or size mismatch, a report for another dump, a report
without `quality_pass`, an older sequence, a recipe mismatch (waiting), a
restored `release` row that differs, and a crash after each journal step with
resume and abort, a retained database with a different dump digest, an extension
outside the allowlist, and an abort or prune that targets the active or a
retained database. Integration test with a 2099 test release on the local
PostgreSQL: new activation as `activator`, retained reactivation, prune, and a
recipe-change pair with its pointer rollback.
