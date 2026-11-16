# Corpus release

Decision: [ADR 0006](../adr/0006-single-host-oci-podman.md) (History
2026-11-14). The MacBook uploads an evaluated build; GitHub checks and signs it.
A host timer activates it. Code follows [pull deployment](pull-deployment.md).

## Upload

`scripts/corpus-upload.sh [--dry-run] <cc> <rel> <build-dir> <report.json>` runs
on the MacBook after `corpus eval` and uploads to bucket `trachuan-releases`,
prefix `<cc>/<rel>/`:

- `corpus.dump`: the build's verified dump.
- `MANIFEST.sha256`: the build artifact manifest
  ([pipeline](pipeline.md#items-artifacts-and-gaps)).
- `report.json`: the [evaluation](evaluation.md) report, with `--previous` set
  to the active report. For version 1, `--previous-release` supplies
  `release.json` or a signed manifest. Version 2 keeps the full retrieval
  recipe.
- `release.json`: the dump's `release` row as JSON
  ([corpus schema](corpus-schema.md)), written by `corpus release-row`.
- `build.json`: the build record, listed in `MANIFEST.sha256`; schema 8 and
  later only, the source of the manifest's `heading_evidence`.

`corpus release-row <cc> <rel> --out <file>` reads the release database and
workshop read-only, without Temporal. `release.json` keeps the release row's
column names, excluding `singleton`, and adds `extensions`, `run` and
`recipe_hash_inputs`. `run` records its id, expected and incomplete item counts,
unaccepted gap count, matching build artifact hash and `complete`. Complete
requires an evaluated or released run, every item done or a gap with a gap row,
every gap accepted, and a done build whose input and recipe hashes match the
release row. The workshop reads share one repeatable-read snapshot.

Report recipes are hashes. `recipe_hash_inputs` holds the exact JSON strings
hashed for `query`, `document`, `chunk`, `lexical` and `retrieval`: the
embedding and search recipes use their Go serialization; chunk uses PostgreSQL's
JSONB bytes, as evaluation does. Validation parses each string and requires
equality with its release-row recipe object, then checks its SHA-256 against the
report. The signed manifest carries the release row's recipe objects.

It uses AWS CLI v2 against the
[S3 compatibility endpoint](deployment.md#object-storage) with the
`trachuan-release-upload` customer secret key from the owner's credential store,
path-style addressing and `AWS_REQUEST_CHECKSUM_CALCULATION=when_required`,
since recent AWS CLI versions add checksum headers by default
([AWS SDK data integrity](https://docs.aws.amazon.com/sdkref/latest/guide/feature-dataintegrity.html)).
It refuses unless the run's expected items are done or accepted gaps,
`report.json` has `quality_pass: true`, no research override and a
`previous_diff`, every required `non_regression` gate has `gating:true` and
status `pass` at the top level and in each populated product cohort, and
`dump_sha256` equals the local dump. Previous identity:
`previous_diff.previous:{country,rel,report_sha256}`. The uploader verifies the
previous release's signed manifest with `oras` and `cosign`, fetches its report
through the same S3 configuration, and applies the checks of
[MRR@8 tolerance](evaluation-mrr-tolerance.md#validators). Top-level comparisons
must equal `old_product`; all cohorts must exist, including empty ones without
comparisons. A prefix that already holds objects is never overwritten; a rebuilt
corpus gets a new `rel`. The Python validator owns the comparison contract, and
a Go test checks it against evaluator output, including the non-gating flags.
Success dispatches `corpus-release.yml`; dry-run only checks local inputs.

The uploader holds an exclusive nonblocking `flock` on
`${HOME}/.cache/trachuan/corpus-upload.lock` through validation and dispatch.
The directory has mode `0700`; all checkouts share the lock and refuse another
local upload while it is held. Uploads use S3
[`PutObject`](https://docs.aws.amazon.com/cli/latest/reference/s3api/put-object.html)
or
[multipart uploads](https://docs.aws.amazon.com/cli/latest/reference/s3api/complete-multipart-upload.html)
with 64 MiB parts, retaining each returned ETag and the local byte count. After
all uploads, every object's HEAD size and ETag must still match that receipt
before dispatch. ETags are opaque identifiers, not content hashes. The uploader
logs its AWS CLI version. The workflow installs and requires AWS CLI 2.40.0 and
logs its version.

## Signing workflow

`.github/workflows/corpus-release.yml`: `workflow_dispatch` from `main`,
environment `corpus-release`, which holds the `trachuan-release-verify`
read-only customer secret key. That key is the only OCI credential in GitHub; it
reads one bucket of public legal text and cannot write. The job:

1. HEADs every object before downloading: dump at most 16 GiB, each JSON sidecar
   at most 16 MiB, and `MANIFEST.sha256` at most 1 MiB. Their total must be
   below the runner's current free disk minus 2 GiB. Each download requests a
   [byte range](https://docs.aws.amazon.com/cli/latest/reference/s3api/get-object.html)
   of exactly `bytes=0-<HEAD size minus 1>`, with `--if-match` set to HEAD's
   ETag, and must have that size. The GET response must repeat HEAD's ETag and
   report `ContentRange` as `bytes 0-<HEAD size minus 1>/<HEAD size>`. Response
   checks reject growth or replacement even when the endpoint ignores
   `--if-match` and the downloaded prefix is unchanged.
2. Requires SHA-256 of `MANIFEST.sha256` to equal the report's
   `build_artifact_sha256`, the manifest's `corpus.dump` line and the report's
   `dump_sha256` to equal the downloaded dump's SHA-256, and records its size.
3. Requires the report's `country` and `rel` to equal the inputs, `status`
   completed, `quality_pass: true`, no research override, a `previous_diff`
   naming the release it was compared with, the complete required
   `non_regression` gate set in each product cohort with cases and the top-level
   comparison, with `gating:true` and status `pass` (`not_run` fails), and its
   `recipes` to equal the recipes in `release.json`.
4. Repeats the previous-report checks of upload, writes canonical JSON and signs
   it with `cosign sign-blob --bundle`, keyless, like the image manifest.
5. Pushes `manifest.json` and `manifest.sigstore.json` to
   `ghcr.io/dannyota/trachuan-corpus-release`, tagged `<cc>-<rel>` and
   `<cc>-production`.

`release_eligible` in the report stays false; the signed corpus manifest is the
release authorization. A failed manifest is never retried by the host;
dispatching `corpus-release.yml` again for the same `rel` signs a new manifest
with a new sequence, which is how a failed activation is retried. Signing an
older `rel` again gives it a new sequence, which is how a corpus rollback is
requested. No MacBook signing key exists.

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
              "retrieval": {...}},
  "heading_evidence": {"projection": 1, "scan_sha256": "<64 hex>",
                       "exclusions_sha256": "<64 hex>"},
  "heading_review": {"projection": 1, "scan_sha256": "<64 hex>",
                     "exclusions_sha256": "<64 hex>", "reviewer": "<role>",
                     "date": "2026-11-16"}
}
```

`sequence` is `github.run_number` of `corpus-release.yml`. `previous` is
`previous_diff.previous.rel`. The OCI artifact type is
`application/vnd.trachuan.corpus-release.v1`. Publication moves
`<cc>-production` only above the current verified sequence. `extensions` comes
from `release.json`, where `corpus release-row` records the build database's
`pg_extension` names and versions; the host requires exactly the names `vector`
and `ltree` and logs the versions, and the smoke check in step 3 proves the
restored data works with the image's versions. The certificate identity is
`https://github.com/dannyota/trachuan/.github/workflows/corpus-release.yml@refs/heads/main`.
`heading_review` is present only when `sources/vn-heading-reviews.yaml` has an
entry for the release; signing copies it unchanged. `heading_evidence` is copied
by signing from the build record `build.json`, which the upload directory holds
beside the dump and whose SHA-256 must equal its line in `MANIFEST.sha256`; it
is required on schema 8 manifests and absent below 8. The build input hash
covers `sources/vn-heading-exclusions.yaml` like the corrections
([public document summaries](web-public-documents.md)).

## Host activation

The [updater](pull-deployment.md#updater) checks
`ghcr.io/dannyota/trachuan-corpus-release:<cc>-production` on every run, under
the same locks, so image and corpus changes never overlap. Until the host has
accepted a corpus manifest, a missing or anonymously unreadable package (GHCR
answers `denied`) is status `current` with "no corpus release published"; after
that it fails the run.

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
   ([recipe changes](corpus-release-changes.md#recipe-changes)), alarmed after
   24 hours. Refuse below the free memory and disk thresholds in
   [host budgets](budgets-host.md#host-memory-estimates).
4. `new`: download the dump through the instance principal with the OCI Python
   SDK into `/srv/trachuan/data/releases/<cc>-<rel>/`; require size and SHA-256
   to equal the manifest.
5. Run the activation steps below; on success delete the local dump and write
   the result.

### Activation steps

The host activator lives under `deploy/host/updater/` and works through
`podman exec` into `trachuan-postgres`. Privileged preparation runs as
`postgres` through `postgres-admin.sh`; everything else runs as `activator` (ADR
0003; [system](system.md#database-roles)), which has `CREATEDB`, owns the corpus
databases it creates, and may write only `corpus_release`, `release_journal` and
`retired_release` in `auth`. Before its first activator connection, the corpus
phase requires both `/usr/local/bin/provision-activator.sh` and
`/run/secrets/activator_db_password` to be readable in the running PostgreSQL
container; otherwise it waits with reason `activator not provisioned`. It runs
the idempotent provisioner as `postgres` before connecting and before each
activation: it creates the role with `CREATEDB` when missing, sets its password
from the Vault secret `activator_db_password`, and reapplies the table grants
even when the auth migrations ran before the role existed.

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
   against it with the server code that will serve it, under the evaluation
   profile: no candidate cache or postings and the 60-second deadline, since the
   smoke checks correctness, not serving latency. The restore and prewarm
   sessions set every PostgreSQL timeout to zero.
4. In one transaction: require the country's pointer row, compare-and-swap
   `corpus_release.active_db` from the journaled expected value, require exactly
   one updated row, insert the old database into `retired_release` with its dump
   SHA-256 and `retain_until` seven days ahead, delete the target from
   `retired_release` in `retained` mode, and append `activated`.
5. The server resolves the release once per request, so traffic moves without a
   restart; the postings for the new release load on first use under the
   server's existing gate ([sparse postings](sparse-postings.md)). The public
   summary pool of the new release opens and the old one retires within 5
   seconds ([serving public summaries](web-public-documents-serving.md)).
   `headings_approved` is set for the release only when the manifest's
   `heading_review` equals its `heading_evidence` field by field (projection,
   scan output hash, exclusions hash), both signed; otherwise summaries serve
   labels only.

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

## Schema and recipe changes

The order in which schema versions and recipe changes ship:
[corpus release changes](corpus-release-changes.md).

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
