# Corpus signing

The upload that precedes this and host activation are in
[corpus release](corpus-release.md).

## Signing workflow

`.github/workflows/corpus-release.yml`: `workflow_dispatch` from `main` with
inputs `country`, `rel`, `report_sha256` and `release_sha256`, environment
`corpus-release`, which holds the `trachuan-release-verify` read-only customer
secret key. That key is the law workflow's only OCI credential; it reads one
bucket of public legal text and cannot write
([frameworks](frameworks-release.md#bucket-and-keys) has its own).

The uploader dispatches with the SHA-256 of the report and of `release.json`
that it validated and uploaded. The job fetches those two first and refuses
either with another hash before it fetches anything else. `release.json` binds
`MANIFEST.sha256` (and through it the dump and `build.json`) and, for a release
with a decision, the report and the three evidence files; the report is pinned
itself, so a holder of the upload key cannot swap any object between upload and
signing. Signing a released `rel` again, for a retry or a rollback, passes
`report.sha256` from that `rel`'s signed manifest and no `release_sha256`, which
the job accepts only when `<cc>-<rel>` is already signed. Such a re-sign keeps
every signed field, and refuses a field the signed manifest lacks, so a forged
`release.json` cannot change it. The pins rest on who may dispatch: someone with
repository write access and the upload key chooses them, and only the job's own
checks remain. `scripts/corpus-upload.sh` is the only upload entry: it holds the
local lock and the empty-prefix check. Because a re-sign refuses any field its
signed manifest lacks, a new manifest field must be derived only for releases
signed after it ships, or be added to the fields a re-sign may change; otherwise
no earlier release can be signed again. The job:

1. HEADs every object before downloading: dump at most 16 GiB, each JSON file at
   most 16 MiB, the two [JSON Lines evidence files](release-evidence.md#upload)
   at most 1 GiB, and `MANIFEST.sha256` at most 1 MiB. Their total must be below
   the runner's current free disk minus 2 GiB. Each download requests a
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
   [selected report checks](release-evidence.md#upload-and-signing) and the
   [gate](release-evidence.md#gate), writes canonical JSON and signs it with
   `cosign sign-blob --bundle`, keyless, like the image manifest.
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
and `ltree` and logs the versions, and the smoke check in
[activation step 3](corpus-release.md#activation-steps) proves the restored data
works with the image's versions. The certificate identity is
`https://github.com/dannyota/trachuan/.github/workflows/corpus-release.yml@refs/heads/main`.
`heading_review` is present only when `sources/vn-heading-reviews.yaml` has an
entry for the release; signing copies it unchanged. `heading_evidence` is copied
by signing from the build record `build.json`, which the upload directory holds
beside the dump and whose SHA-256 must equal its line in `MANIFEST.sha256`; it
is required on schema 8 manifests and absent below 8. The build input hash
covers `sources/vn-heading-exclusions.yaml` like the corrections
([public document summaries](web-public-documents.md)).

A release with a [decision](release-evidence.md#selected-report) also carries
`evidence: {findings_sha256, identity_sha256, changes_sha256}`.
