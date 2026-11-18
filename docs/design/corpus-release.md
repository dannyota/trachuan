# Corpus release

Decision: [ADR 0006](../adr/0006-single-host-oci-podman.md) (History
2026-11-14). The MacBook uploads an evaluated build; GitHub checks and signs it.
A host timer activates it. Code follows [pull deployment](pull-deployment.md).

The **production release** is the one the verified `<cc>-production` manifest
names. A lasting rollback is requested by signing the older `rel` within its
retention window, not by a manual swap, so the tag follows it.

## Upload

`scripts/corpus-upload.sh [--dry-run] <cc> <rel> <build-dir> <report.json>` runs
on the MacBook after the run is `evaluated` and uploads to bucket
`trachuan-releases`, prefix `<cc>/<rel>/`:

- `corpus.dump`: the build's verified dump.
- `MANIFEST.sha256`: the build artifact manifest
  ([pipeline](pipeline.md#items-artifacts-and-gaps)).
- `report.json`: the [selected report](release-evidence.md#selected-report) of
  the run, compared with the production release's report. Version 2 keeps the
  full retrieval recipe.
- `release.json`: the dump's `release` row as JSON
  ([corpus schema](corpus-schema.md)), written by `corpus release-row`.
- `build.json`: the build record, listed in `MANIFEST.sha256`; schema 8 and
  later only, the source of the manifest's `heading_evidence`.

`corpus release-row <cc> <rel> --out <file>` reads the release database,
workshop and the run's `eval` artifact read-only, without Temporal.
`release.json` keeps the release row's column names, excluding `singleton`, and
adds `extensions`, `run` and `recipe_hash_inputs`. `run` records its id,
expected and incomplete item counts, unaccepted gap count, matching build
artifact hash, the two report hashes of the
[selected report](release-evidence.md#selected-report) and `complete`. Complete
requires an evaluated or released run, every item done or a gap with a gap row,
every gap accepted, a done build whose input and recipe hashes match the release
row, and, for a run with a [decision](release-evidence.md#selected-report), a
done `evidence` item. The workshop reads share one repeatable-read snapshot.

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
a Go test checks it against evaluator output, including the non-gating flags. It
also applies the
[selected report checks](release-evidence.md#upload-and-signing). Success
dispatches `corpus-release.yml`; dry-run only checks local inputs.

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
read-only customer secret key. That key is the law workflow's only OCI
credential; it reads one bucket of public legal text and cannot write
([frameworks](frameworks-release.md#bucket-and-keys) has its own). The job:

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
4. Repeats the previous-report checks of upload and applies the
   [selected report checks](release-evidence.md#upload-and-signing), writes
   canonical JSON and signs it with `cosign sign-blob --bundle`, keyless, like
   the image manifest.
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
  "schema_version": 8,
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
answers `denied`) or a missing production tag is status `current` with "no
corpus release published"; after that it fails the run.

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

The steps, what a dump may do on the host, and recovery are in
[corpus activation](corpus-release-activation.md).

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
outside the allowlist, an abort or prune that targets the active or a retained
database, and each
[selected report check](release-evidence.md#upload-and-signing). Integration
test with a 2099 test release on the local PostgreSQL: new activation as
`activator`, retained reactivation, prune, and a recipe-change pair with its
pointer rollback.

Hostile dumps, for both kinds, on PostgreSQL: SQL planted as `activator` (a
`public.pg_prewarm` overload, a database `search_path`, an extra function, view,
schema, extension, type or role setting, a pre-created target database) is
refused by the check or removed by the create step and never runs in a stronger
session; no `postgres` session follows the first dump byte; another superuser is
refused; `activator`'s role-wide settings are reset; and an honest dump restores
to the same tables, data and reader grants as before.
