# Corpus release

Decision: [ADR 0006](../adr/0006-single-host-oci-podman.md) (History
2026-11-14). The MacBook uploads an evaluated build; GitHub checks and signs it.
A host timer activates it. Code follows [pull deployment](pull-deployment.md).

The **production release** is the one the verified `<cc>-production` manifest
names. A lasting rollback is requested by signing the older `rel` within its
retention window, not by a manual swap, so the tag follows it.

## Upload

`scripts/corpus-upload.sh [--dry-run] --repository <owner/name> <cc> <rel> <build-dir> <report.json>`
runs on the MacBook after the run is `evaluated` and uploads to bucket
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
- The three [evidence files](release-evidence.md#upload), for a run with a
  decision.

`corpus release-row <cc> <rel> --out <file>` reads the release database,
workshop and the run's `eval` artifact read-only, without Temporal.
`release.json` keeps the release row's column names, excluding `singleton`, and
adds `extensions`, `run` and `recipe_hash_inputs`. `run` records its id,
expected and incomplete item counts, unaccepted gap count, matching build
artifact hash, the two report hashes of the
[selected report](release-evidence.md#selected-report), the three
[evidence file](release-evidence.md#upload) hashes and `complete`. Complete
requires an evaluated or released run, every item done or a gap with a gap row,
every gap accepted, a done build whose input and recipe hashes match the release
row, and, for a run with a [decision](release-evidence.md#selected-report), a
done `evidence` item; release-row refuses such a run without one, so the five
fields are present together or not at all. The workshop reads share one
repeatable-read snapshot.

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

## Signing

The signing workflow and the corpus manifest are in
[corpus signing](corpus-signing.md).

## Host activation

The [updater](pull-deployment.md#updater) checks
`ghcr.io/dntmakers/trachuan-corpus-release:<cc>-production` on every run, under
the same locks, so image and corpus changes never overlap. Until the host has
accepted a corpus manifest, a missing or anonymously unreadable package (GHCR
answers `denied`) or a missing production tag is status `current` with "no
corpus release published"; after that it fails the run.

1. Verify with `cosign verify-blob --bundle`, the identity from policy and the
   transparency log proof, then apply the
   [acceptance rule](pull-deployment.md#acceptance) for corpus sequences.
2. Choose the mode from the [activation record](activation-record.md#mode):
   `retained` when the host retired `database` with the manifest's dump SHA-256;
   refuse when the name is listed with another digest; else `new`. `new` also
   requires `previous` to equal the active release, so every newly restored
   release was compared with the release it replaces; `retained` is a rollback
   within the seven-day window and is exempt.
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
waited more than 24 hours, and within minutes on an
[integrity alert](activation-record.md#alerts). Logs go to
`/var/log/trachuan/release.log`.

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

The server's tables, on PostgreSQL: a trigger, view or policy planted by
`auth_owner` on each release table never runs as `activator`, in either state of
the provisioner flag; new activation, retained reactivation, prune and one crash
recovery work from the host record; a table failure waits without spending the
manifest; and the lock excludes a concurrent run. Unit tests against an
in-memory host and tables: each row of the recovery table, with the record lost,
unreadable or disagreeing with the journal; forged or altered journal rows that
try to resume, finish, abort, redirect or reach a command; a forged pointer or
table never dropping the host's active or retired database; every alert code
reaching the result, the metric and the watch; the first run seeding the file
from a real state shape and leaving a retained `postgres`-owned database alone;
and a rollback and roll forward across the change. No test runs a real deploy or
rollback across it.
