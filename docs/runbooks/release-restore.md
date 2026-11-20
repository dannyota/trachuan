# Restore and select a corpus release

On the [production host](host.md), after first start.

## Release a corpus

On the MacBook, configure AWS CLI v2 profile `trachuan-release-upload` with the
OCI IAM user `trachuan-release-upload` customer secret key. Run
`aws configure --profile trachuan-release-upload` yourself; the credential
fields are `aws_access_key_id` and `aws_secret_access_key`. Region is
`ap-singapore-1`. The uploader uses that named shared-credentials profile and
supplies a temporary configuration with path-style S3 addressing. It never reads
or prints the keys. Set `OCI_S3_ENDPOINT` to the releases bucket's HTTPS S3
compatibility endpoint. Python 3.11 or later, `flock`, `curl`, `oras`, signed-in
`gh` and `cosign` v3.1.4 are also required. The uploader runs
`cosign verify-blob` to verify the production and previous releases' signed
manifests. On macOS, install `flock` with `brew install flock` if absent
([Homebrew](https://formulae.brew.sh/formula/flock)). The uploader prints its
AWS CLI version. The workflow installs and requires AWS CLI 2.40.0 and prints
its version.

A release compares with the production release from the moment its run is
created. Before the first `corpus start` with `--previous-report`, restart the
corpus worker on code that has the flag, so no older worker picks up the run.
Pass the uploaded `report.json` of the production release, exactly as its signed
`<cc>-production` manifest binds it. Never pass a stage evaluation artifact from
the artifact store. `corpus start` freezes that choice as the
[selected report](../design/release-evidence.md#selected-report) decision; a
different previous release needs a new run and `rel`.

When the run is `evaluated`, accept its reviewed gaps with `corpus accept-gaps`.
The report to upload is `report.json` of the run's `eval` artifact. A
`corpus eval` report cannot publish. The upload requires completed quality
checks, passing required gating comparisons in every product cohort with cases,
and no research override. All cohorts must be present; empty cohorts require no
comparisons.

Prepare a separate upload directory so the immutable build artifact stays
unchanged. Copy its dump, original manifest and build record without
regenerating the manifest. With `CORPUS_DATABASE_URL`,
`CORPUS_BUILDER_DATABASE_URL` and an absolute `CORPUS_ARTIFACTS` configured:

```sh
CC=vn
REL=20261120_1
REPOSITORY=$(gh repo view --json nameWithOwner --jq .nameWithOwner)
BUILD=/path/to/verified-build-artifact
UPLOAD=/path/to/new-upload-directory
PREVIOUS=/path/to/production-upload-directory/report.json
corpus start "$CC" "$REL" --previous-report "$PREVIOUS"
```

After the run is `evaluated`:

```sh
artifact() {
  psql "$CORPUS_DATABASE_URL" -At -v cc="$CC" -v rel="$REL" -v stage="$1" <<'SQL'
SELECT a.path FROM run r
JOIN run_item i ON i.run_id = r.id
JOIN artifact a ON a.id = i.artifact_id
WHERE r.country = :'cc' AND r.rel = :'rel'
  AND i.stage = :'stage' AND i.state = 'done';
SQL
}
REPORT=$CORPUS_ARTIFACTS/$(artifact eval)/report.json
EVIDENCE=$CORPUS_ARTIFACTS/$(artifact evidence)
mkdir "$UPLOAD"
cp "$BUILD/corpus.dump" "$BUILD/MANIFEST.sha256" "$BUILD/build.json" "$UPLOAD/"
cp "$EVIDENCE/findings.json" "$EVIDENCE/identity.jsonl" "$EVIDENCE/changes.jsonl" "$UPLOAD/"
corpus release-row "$CC" "$REL" --out "$UPLOAD/release.json"
bash scripts/corpus-upload.sh --dry-run --repository "$REPOSITORY" "$CC" "$REL" "$UPLOAD" "$REPORT"
bash scripts/corpus-upload.sh --repository "$REPOSITORY" "$CC" "$REL" "$UPLOAD" "$REPORT"
```

The older path is for `vn/20261118_2` only, whose run predates this rule: it has
no decision, so evaluate with
`corpus eval <cc> <rel> --out <directory> --previous <report.json>`, using the
production release's uploaded report, and upload that report. Its `release.json`
has no selected report hashes; upload refuses that shape for every other
release.

A release with a decision uploads its three
[evidence files](../design/release-evidence.md#upload) beside the dump.
`corpus release-row` writes their SHA-256 into `release.json` from the run's
verified `evidence` artifact and refuses a decided run without one. Upload and
signing apply the [gate](../design/release-evidence.md#gate): they refuse a
missing file, a hash or binding that differs from the dump, build, report or the
previous release's signed manifest, a detector that is not complete, and an
`open` finding of a `blocking` or unlisted detector. They print the count of
`open` findings of `review` and `info` detectors, which do not block.

The upload refuses unless production is settled: the report's previous `rel` and
SHA-256 must equal `rel` and `report.sha256` of the verified `<cc>-production`
manifest, and `release` of `https://trachuan.vn/api/v1/public/status` must be
that `rel`. A release still activating, waiting on recipes, failed or rolled
back by hand blocks it. Only `vn` can upload until that endpoint takes a
country. If production has moved on, the release needs a new run and `rel`.

Dry-run validates local files and prints the upload and dispatch plan without
calling AWS or GitHub. A real upload holds
`${HOME}/.cache/trachuan/corpus-upload.lock`, shared by all checkouts for the
same owner. The uploader creates the cache directory with mode `0700`, checks
the prefix is empty, uploads every object (`build.json` too from schema 8), then
rechecks every size and ETag against its upload receipt. It keeps the uploaded
report as `$UPLOAD/report.json` before dispatching `corpus-release.yml` from
`main` with the SHA-256 of the uploaded report and `release.json` as
`report_sha256` and `release_sha256`; signing refuses any other. Use that file
as `--previous-report` for the next release after this release becomes active.
If dispatch alone fails, do not upload again: run the uploader again with
`--dry-run` and the same arguments, whose last line is the dispatch command with
both hashes, and run that line. An older checkout dispatches without the hashes,
and the job refuses it; dispatch by hand the same way.

A partial upload reserves that prefix; use a new release id for a rebuilt
corpus. The signed manifest authorizes release; the report's `release_eligible`
remains false. Host activation follows
[corpus release](../design/corpus-release.md#host-activation).

Signing repeats the production comparison before it signs or pushes anything, so
a refusal leaves no tag and no signature. A release whose `<cc>-<rel>` manifest
already exists with a sequence at or below production's is exempt, which keeps
retry and rollback working. Dispatching the same release again retries a failed
activation. Signing an existing `<cc>-<rel>` again keeps every field of its
signed manifest except `commit`, `sequence` and `heading_review`, `evidence`
included, and refuses before signing when the bucket's objects would change one
or add a field the signed manifest lacks. A release without selected report
hashes below `vn/20261118_2` signs only when its manifest already exists.

When an earlier run pushed the release but failed to move `<cc>-production`, the
comparison applies to the retry only while production's sequence is below the
pushed one. Once another release holds production with a higher sequence, the
pushed release is exempt too: signing it moves the tag, the host refuses it
because its `previous` is not the active release, and uploads stay blocked.
Recover by signing the active release again. Every re-sign passes
`report.sha256` from that release's signed manifest and no `release_sha256`,
which the job accepts only for a release already signed; `resign-check` verifies
the manifest and prints the exact command as its last line, with or without the
upload directory. A re-sign that fails at "Require a signed release for an empty
release pin" names a release that was never signed, or a registry that could not
be read anonymously or verified (an `oras` or `cosign` error there); sign a new
release first from its upload, with its pins:

```sh
python3 deploy/github/corpus_production.py resign-check vn <active rel> [<upload-dir>] --repository "$REPOSITORY"
```

Then run the printed `gh workflow run corpus-release.yml ...` line.

### First release with a decision

Before uploading the first release whose `release.json` carries the decision
fields, confirm that production runs the updater release that accepts the
manifest's `evidence` field, and that the
[release floor](../design/release-floor.md) is at or above that image. An older
updater refuses the manifest before it binds the sequence; the same manifest
activates once the newer updater is installed.

### First release with reviewed knowledge

Before uploading the first release whose `sources/<cc>-knowledge.yaml` holds a
`provision_effect` or `relation` entry, confirm that production runs v0.1.19 or
later, which admits relation method `reviewed`, and that the
[release floor](../design/release-floor.md) is at or above that image, so no
rollback can reach an image that drops the warning.

### Roll back a corpus release

Request a lasting rollback by signing the older release again, within its
retention window: run `resign-check` for it, as below, then the
`gh workflow run` line it prints, which carries that release's signed
`report.sha256`.

The new signature gives that release a new sequence, `<cc>-production` follows
it, and the host reactivates the retained database. A manual swap of
`corpus_release.active_db` leaves the tag behind, and the next upload refuses
until the two agree.

Check a rollback target ahead of need, from its upload directory. The command
needs `oras` and `cosign`, signs and pushes nothing, prints each signed field
that would change and exits 3 when any would, and ends with the dispatch command
for that release. A `signed absent` line means a field the signed manifest lacks
would be added, so that release cannot be signed again. Without the upload
directory it only verifies the manifest and prints the command:

```sh
python3 deploy/github/corpus_production.py resign-check vn <rel> [<upload-dir>] --repository <owner/name>
```

`no signed field would change` does not prove the rollback will sign. Signing
also runs `check_remote`, which fetches the previous release's report and
recomputes the comparison with today's rules. It needs the S3 key, so
`resign-check` does not run it.

### Refresh the home sample

The home page shows one stored search result whose stamp states its release
date. After the host activates a new corpus release (`corpus_status` on
`https://trachuan.vn` names it), refresh the sample from the MacBook with a
personal token, typed at a hidden prompt rather than on the command line:

```sh
read -rs TRACHUAN_TOKEN && export TRACHUAN_TOKEN
TRACHUAN_URL=https://trachuan.vn node apps/web/scripts/make-sample.mjs
unset TRACHUAN_TOKEN
```

The script rewrites `apps/web/app/data/sample-result.json` and
`sample-result.meta.json`. Review the diff and open a web pull request with both
files; the sample reaches the site with the next tag.
