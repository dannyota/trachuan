# Control frameworks: release

How a frameworks release goes from the MacBook to the host. It follows the law
path of [corpus release](corpus-release.md) and
[pull deployment](pull-deployment.md) step for step; this file names only what
differs for the kind. The identity rules (manifest fields, channel, first
release, pointer table) are in
[frameworks](frameworks.md#release-and-deployment) and the places licensed
material may be are in
[access](frameworks-access.md#licensed-material-and-where-it-may-be). Nothing
below prints, logs or uploads anything but ids, sizes, hashes, counts and
statuses.

## Bucket

The dump and its sidecars go to their own bucket, `trachuan-frameworks-releases`
(`NoPublicAccess`, the settings of `trachuan-releases`, `prevent_destroy`),
prefix `frameworks/<rel>/`. `trachuan-releases` keeps holding public legal text
only, so no later decision about law dumps can reach a licensed one. Three
principals reach the bucket, as for law: the upload group may create, inspect
and read objects; the release verify group and the host's dynamic group may read
them. No other statement names it.

## Upload

`scripts/corpus-upload.sh [--dry-run] frameworks <rel>` takes the run's files
from `FRAMEWORKS_WORK_DIR/<rel>/`, with the law uploader's lock, AWS CLI
settings, size receipts and no-overwrite rule:

| Object            | Source                                                                  |
| ----------------- | ----------------------------------------------------------------------- |
| `frameworks.dump` | `build/frameworks.dump`                                                 |
| `build.json`      | `build/manifest.json`, the build step manifest                          |
| `evaluate.json`   | `evaluate/manifest.json`, the evaluate step manifest                    |
| `report.json`     | `evaluate/report.json`                                                  |
| `release.json`    | `corpus frameworks release-row <rel> --out <file>`, from the local copy |

`corpus frameworks release-row` reads `frameworks_<rel>` read-only and writes
its `release` row (columns except `singleton`) with the database's extension
names and versions as `extensions`. It reads no workshop state.

The uploader refuses unless (the checks of
[build](frameworks-build.md#gaps-and-signing)): `quality_pass` is true; the
dump's SHA-256 and the build step manifest's SHA-256 equal the report's; the
report's release id and its `snapshot` through `embed` recipe hashes equal the
release row's; its `build` and `evaluate` recipe hashes equal those in the two
step manifests; and its previous report is the active frameworks release's. The
active release is the `frameworks-production` manifest, verified with `oras` and
`cosign` as for law; its report is fetched from the bucket and its SHA-256 must
equal the manifest's. With no frameworks manifest published, the report's
`previous` must be null; once one is, a null `previous` is refused.

## Signing workflow

`.github/workflows/frameworks-release.yml`: `workflow_dispatch` from `main` with
input `rel`, environment `corpus-release` and its read-only key, which now reads
both release buckets. A workflow of its own gives the kind its own certificate
identity and its own `run_number` sequence. The job:

1. HEADs and downloads the five objects with the law job's bounds and range
   checks into `$RUNNER_TEMP`.
2. Repeats every upload check, including the previous report through the active
   `frameworks-production` manifest.
3. Writes the canonical manifest with the field list of
   [frameworks](frameworks.md#release-and-deployment): `kind`
   `trachuan.corpus-release`, `version` 1, `corpus_kind` `frameworks`,
   `database` `frameworks_<rel>`, `sequence` its run number, `previous` the
   report's previous release or null, `dump`
   (`frameworks/<rel>/frameworks.dump`, SHA-256, size), `report` (SHA-256,
   `quality_pass`), `build_artifact_sha256` (the build step manifest's SHA-256),
   `schema_version`, `extensions` and `recipes` from `release.json` (parameters
   only), and the numbers of framework versions and controls from the release
   row.
4. Signs it keyless with `cosign sign-blob --bundle` and pushes `manifest.json`
   and `manifest.sigstore.json` to `ghcr.io/dannyota/trachuan-corpus-release`,
   tagged `frameworks-<rel>` and `frameworks-production`, moving the production
   tag only above the current verified frameworks sequence.
5. Deletes `$RUNNER_TEMP`'s copies in a step that always runs. It uploads no
   workflow artifact and caches nothing.

The certificate identity is
`https://github.com/dannyota/trachuan/.github/workflows/frameworks-release.yml@refs/heads/main`.
A failed manifest is never retried by the host; dispatching again signs a new
sequence, as for law.

## Host

The updater runs a `frameworks` phase after the law `corpus` phase, under the
same locks. Each phase's result is its own; a waiting or failed law phase does
not skip the frameworks phase, nor the reverse. The policy gains a `frameworks`
block (repository, identity, namespace, bucket) beside `corpus`, and
`state.json` a `frameworks` key with its own accepted sequence, retained and
failed lists, so one kind's history never supersedes the other's.

The phase follows [host activation](corpus-release.md#host-activation) with
these differences:

- **Pull and verify.** Tag `frameworks-production`, the frameworks identity, and
  a verifier with the frameworks field list that refuses any other
  `corpus_kind`. Until a frameworks manifest is accepted, a missing package is
  status `current` with "no frameworks release published".
- **Mode.** `retained` and `new` as for law, over `retired_release` rows of kind
  `frameworks`. `new` requires `previous` to equal the active frameworks
  release; with no `frameworks_release` row it requires `previous` null instead,
  which is the first-release bootstrap and happens once.
- **Compatibility.** `server release-check` reports, per kind, the schema
  versions and recipe versions the image serves; a frameworks manifest outside
  them is status `waiting`, as for law.
- **Staging.** `/srv/trachuan/data/releases/frameworks-<rel>/`, mode `0700`,
  deleted when activation ends in any state.
- **Restore.** The law steps: the same TOC allowlist (`vector`, `ltree`),
  restore options and `grant-corpus.sh`, which accepts `frameworks_<rel>` names
  and grants the corpus reader as it does for law.
- **Smoke.** `server release-check --database frameworks_<rel>` opens the
  frameworks engine on the target with a key made in memory for that run, and
  runs one search and one read as a caller who may read every version. It prints
  status and counts only.
- **Pointer.** Step 4 swaps `frameworks_release.active_db` from the journaled
  value, or inserts the country's row when there is none (bootstrap), and writes
  the old database to `retired_release` with kind `frameworks`. The law phase
  never writes `frameworks_release`; the frameworks phase never writes
  `corpus_release`.
- **Result.** `/srv/trachuan/state/deploy/release-frameworks.json`; the metrics
  timer publishes `release_failed` and `release_waiting_age_seconds` with a
  `kind` dimension, and the alarms fire for either kind.

The daily prune handles retired databases of both kinds. Recovery, protection
and the seven-day window are the law rules, per kind.

## Schema

One auth migration:

- `release_journal` and `retired_release` gain `corpus_kind` (`law` or
  `frameworks`, existing rows `law`); the database-name check pairs the kind
  with its prefix, and keys that were per country become per country and kind.
- `frameworks_release` gains the checks of `corpus_release`: a two-letter
  country and an `active_db` that is a `frameworks_<rel>` name.
- `activator` may `SELECT`, `INSERT` and `UPDATE` `frameworks_release`;
  `provision-activator.sh` reapplies that grant.

## Key and flag

The server's frameworks key (cursor and `access_set_version` HMAC,
[results](frameworks-results.md)) is the Vault secret `server_frameworks_key`.
The owner generates it locally, as for `account_keys`, and never prints it:
`openssl rand -base64 32 | tr '+/' '-_' | tr -d '='`. The host mounts it at
`/run/secrets/server_frameworks_key` (owner `65532`, mode `0400`) and
`server.env` sets `SERVER_FRAMEWORKS_KEY_FILE` to that path and
`SERVER_FRAMEWORKS=false`. The server reads the key only with the flag on, so
the image ships with the key in place and the feature off. Rotating the key
invalidates frameworks cursors and access set versions; clients search again.

## Order

1. The owner creates `server_frameworks_key` in Vault and applies OpenTofu: the
   bucket, its three read and write statements, and the secret name in the
   host's Vault read statement.
2. One image ships the migration, `release-check` per kind, the updater's
   frameworks phase and the key mount, with the flag off.
3. The next image raises the [release floor](pull-deployment.md#release-floor)
   to it, so no older updater can run once frameworks state exists.
4. The first frameworks release is uploaded and signed; the host activates it.
5. Host memory is measured with both corpora active
   ([host budgets](budgets-host.md)).
6. The owner's entitlement is created and, on the owner's decision, an image
   turns the flag on; acceptance runs with an entitled and a non-entitled
   account.

## Proofs

The law proofs of [corpus release](corpus-release.md#proofs) for the kind, plus:
the bootstrap accepted once with no pointer row and refused once a row exists; a
law manifest refused by the frameworks verifier and the reverse; a frameworks
activation failure that leaves law activation, state and pointer untouched, and
the reverse; the frameworks phase never issuing a statement on `corpus_release`;
the signing job's workspace empty after success and failure; and no log line of
the uploader, the job or the updater holding anything but ids, sizes, hashes,
counts and statuses.
