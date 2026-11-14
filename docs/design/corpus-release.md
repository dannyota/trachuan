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
- `report.json`: the evaluation report ([evaluation](evaluation.md)).
- `release.json`: the dump's `release` row as JSON
  ([corpus schema](corpus-schema.md)), written by `corpus release-row`.

It uses AWS CLI v2 against the
[S3 compatibility endpoint](deployment.md#object-storage) with the
`trachuan-release-upload` customer secret key from the owner's credential store,
path-style addressing and `AWS_REQUEST_CHECKSUM_CALCULATION=when_required`,
since recent AWS CLI versions add checksum headers by default. It refuses before
upload unless `report.json` has `quality_pass: true` and no research override,
and its `dump_sha256` equals the local dump. Then it dispatches
`corpus-release.yml` with `country` and `rel`.

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
   completed, `quality_pass: true`, no research override, and its `recipes` to
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
  "sequence": 57,
  "dump": {"object": "vn/20261114_3/corpus.dump", "sha256": "<64 hex>",
           "size": 1234567890},
  "report": {"sha256": "<64 hex>", "quality_pass": true},
  "build_artifact_sha256": "<64 hex>",
  "schema_version": 6,
  "recipes": {"dense": {...}, "chunk": {...}, "lexical": {...},
              "retrieval": {...}}
}
```

`sequence` is `github.run_number` of `corpus-release.yml`. The certificate
identity is
`https://github.com/dannyota/trachuan/.github/workflows/corpus-release.yml@refs/heads/main`.

## Host activation

`trachuan-release.timer` starts the root `trachuan-release.service` every 10
minutes. It holds the deploy lock from [pull deployment](pull-deployment.md) for
the whole run, so image and corpus changes never overlap.

1. Fetch `<cc>-production`; verify with `cosign verify-blob --bundle`, the
   identity from policy and the transparency log proof. A manifest whose hash is
   active or retained is a no-op; otherwise require a sequence above the stored
   highest corpus sequence.
2. Check compatibility: `podman run --rm <server image> server release-check`
   prints the schema versions and recipes the deployed server serves and the
   running query embedder reports. Refuse on any difference from the manifest,
   and below the free memory and disk thresholds in
   [host budgets](budgets-host.md#host-memory-estimates). A recipe difference is
   status `waiting` (see [recipe changes](#recipe-changes)), not a failure, for
   24 hours.
3. Choose the mode: `retained` when `database` is listed in `retired_release`,
   else `new`.
4. `new`: download the dump through the instance principal with the OCI Python
   SDK into `/srv/trachuan/data/releases/<cc>-<rel>/`; require size and SHA-256
   to equal the manifest.
5. Run the activation steps below; on success delete the local dump, store the
   manifest hash and sequence, and write the result.

Activation steps, in the host activator under `deploy/host/release/`, using
`podman exec` into `trachuan-postgres`. They connect as the `activator` role,
and the restore as the `restorer` role, both limited as in
[system](system.md#database-roles) and ADR 0003; their passwords are the Vault
secrets `activator_db_password` and `restorer_db_password`, and the PostgreSQL
initialization creates both roles:

0. Take the `activation` advisory lock in the `auth` database; append
   `release_journal` (`started`, mode, target database, manifest hash). Every
   later step appends its own row.
1. `new` only: list the TOC and refuse functions, extensions other than
   pgvector, and event triggers; create the database;
   `pg_restore --no-owner --no-privileges --exit-on-error` with
   `maintenance_work_mem=2GB` and two maintenance workers; run
   `grant-corpus.sh`.
2. Require the restored `release` row to equal the manifest's `rel`,
   `schema_version` and recipes; otherwise drop the target (`new`) or stop
   (`retained`).
3. Prewarm the target's tables, indexes and TOAST relations with `pg_prewarm`,
   then `server release-check --database <target>` runs one search and one read
   against it with the deployed server code.
4. In one transaction: compare-and-swap `corpus_release.active_db` from the
   expected current value, insert the old database into `retired_release` with
   `retain_until` seven days ahead, delete the target from `retired_release` in
   `retained` mode, and append `activated`.
5. The server resolves the release once per request, so traffic moves without a
   restart; the postings for the new release load on first use under the
   server's existing gate ([sparse postings](sparse-postings.md)).

`release_journal` and `retired_release` are auth migrations. After a crash the
next run reads the journal: in `new` mode the target must still be neither
active nor retired, in `retained` mode it must still be retired and not active;
then it continues from the last step. When that guard fails it closes the
journal as aborted and drops the target only in `new` mode. A daily `prune`
under the same lock drops a database only when it is not active, not the target
of an open journal, past `retain_until`, and without connections.

## Recipe changes

A release whose recipes differ from the running server waits. The
[image manifest](pull-deployment.md#image-manifest) for the matching code sets
`corpus_release` to that corpus manifest's hash. The updater then verifies the
corpus manifest and runs steps 0 to 3 while the old release still serves, stops
Caddy, deploys the images, runs step 4, and starts Caddy after readiness. A
failure before step 4 leaves the old images and release active.

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
resume and abort. Integration test with a 2099 test release on the local
PostgreSQL: new activation, retained reactivation, prune.
