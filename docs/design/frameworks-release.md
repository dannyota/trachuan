# Control frameworks: release

How a frameworks release goes from the MacBook to the host. It follows the law
path of [corpus release](corpus-release.md) and
[pull deployment](pull-deployment.md) step for step; this file names only what
differs for the kind. The identity rules (manifest, channel, first release,
pointer table) are in [frameworks](frameworks.md#release-and-deployment) and the
places licensed material may be are in
[access](frameworks-access.md#licensed-material-and-where-it-may-be). Nothing
below prints, logs or uploads anything but ids, sizes, hashes, counts and
statuses. The frameworks pointer, journal and retired rows use the country key
`vn`, as the server's resolver does.

## Bucket and keys

The dump and its sidecars go to their own bucket, `trachuan-frameworks-releases`
(`NoPublicAccess`, the settings of `trachuan-releases`, `prevent_destroy`),
prefix `frameworks/<rel>/`. `trachuan-releases` keeps holding public legal text
only, so no later decision about law dumps can reach a licensed one.

Three principals reach it. The upload group `trachuan-release-upload` may
create, overwrite, inspect and read objects, as for law: multipart uploads need
both create and overwrite
([OCI multipart uploads](https://docs.oracle.com/en-us/iaas/Content/Object/Tasks/usingmultipartuploads.htm)),
and the uploader never overwrites an existing prefix. A new user and group,
`trachuan-frameworks-verify`, with a customer secret key, may read it and
nothing else; the law verify key cannot read it, so a leaked law key exposes no
licensed dump. The host's dynamic group may read it. No other statement names
the bucket.

## Upload

`scripts/corpus-upload.sh [--dry-run] frameworks <rel>` takes the run's files
from `FRAMEWORKS_WORK_DIR/<rel>/` with the law uploader's lock, AWS CLI
settings, size receipts and no-overwrite rule:

| Object            | Source                                                                   |
| ----------------- | ------------------------------------------------------------------------ |
| `frameworks.dump` | `build/frameworks.dump`                                                  |
| `build.json`      | `build/build.json`, whose SHA-256 the report binds                       |
| `evaluate.json`   | `evaluate/manifest.json`, the evaluate step manifest                     |
| `report.json`     | `evaluate/report.json`                                                   |
| `release.json`    | `release/release.json`, written by `corpus frameworks release-row <rel>` |

`corpus frameworks release-row <rel>` reads `frameworks_<rel>` read-only and
writes `FRAMEWORKS_WORK_DIR/<rel>/release/release.json`: the `release` row
(columns except `singleton`) and the database's extension names and versions as
`extensions`. It reads no workshop state.

Every temporary file the uploader makes (multipart parts, the fetched previous
report, verification output) goes under `FRAMEWORKS_WORK_DIR/<rel>/.upload/`,
mode `0700`, set as `TMPDIR` for every child process and deleted on exit; the
uploader refuses a work directory inside a synced folder, as build does.

The uploader checks a new release (the checks of
[build](frameworks-build.md#gaps-and-signing)): `quality_pass` is true; the
dump's SHA-256 equals the report's `dump_sha256` and `build.json`'s; the SHA-256
of `build.json` equals the report's `build_manifest_sha256`; the report's `rel`
and its `snapshot`, `parse`, `relate`, `chunk` and `embed` recipe hashes equal
the release row's; its `build` recipe hash equals that of `build.json`'s
`recipe` and its `evaluate` hash that of `evaluate.json`'s. Its previous report
must be the active one: the `frameworks-production` manifest, verified with
`oras` and `cosign` as for law, whose report, fetched from the bucket, has the
SHA-256 the report names. With no frameworks manifest published, `previous` must
be null.

## Signing workflow

`.github/workflows/frameworks-release.yml`: `workflow_dispatch` from `main` with
input `rel`, environment `frameworks-release` (deployments from `main` only),
which holds `FRAMEWORKS_READ_KEY_ID` and `FRAMEWORKS_READ_KEY_SECRET`. A
workflow of its own gives the kind its own certificate identity and its own
`run_number` sequence. The job:

1. HEADs and downloads the five objects into `$RUNNER_TEMP` with the law job's
   size bounds and range checks.
2. Repeats the upload checks except the previous one, which here is weaker so
   that a retry and a rollback can be signed: `previous` is null, or names a
   frameworks release whose signed manifest (`frameworks-<rel>`) verifies and
   whose report SHA-256 equals the one named. The host decides whether the
   release may become active (mode, below).
3. Writes the canonical manifest:

   ```json
   {
     "kind": "trachuan.corpus-release", "version": 1,
     "corpus_kind": "frameworks",
     "repository": "dannyota/trachuan", "commit": "<workflow commit>",
     "rel": "20261116", "database": "frameworks_20261116",
     "sequence": 3, "previous": null,
     "dump": {"object": "frameworks/20261116/frameworks.dump",
              "sha256": "<64 hex>", "size": 123456789},
     "report": {"sha256": "<64 hex>", "quality_pass": true},
     "build_artifact_sha256": "<SHA-256 of build.json>",
     "schema_version": 1,
     "extensions": {"vector": "<version>", "ltree": "<version>"},
     "recipes": {"dense": {...}, "chunk": {...}, "lexical": {...},
                 "retrieval": {...}},
     "counts": {"versions": 3, "controls": 1612}
   }
   ```

   `previous` is the report's previous `rel` or null; `recipes` are the release
   row's recipe objects, unchanged; `counts` are the row's version and control
   counts. The field set is closed.

4. Signs it keyless with `cosign sign-blob --bundle` and pushes `manifest.json`
   and `manifest.sigstore.json` to `ghcr.io/dannyota/trachuan-corpus-release`,
   tagged `frameworks-<rel>` and `frameworks-production`, moving the production
   tag only above the current verified frameworks sequence.
5. Deletes `$RUNNER_TEMP`'s copies in a step that always runs. It uploads no
   workflow artifact and caches nothing.

The certificate identity is
`https://github.com/dannyota/trachuan/.github/workflows/frameworks-release.yml@refs/heads/main`.
Dispatching again for the same `rel` signs a new sequence (a retry); dispatching
for an older retained `rel` is a rollback, as for law.

## Host

The updater runs a `frameworks` phase after the law `corpus` phase, under the
same locks, only when the policy has a `frameworks` block (repository, identity,
namespace, bucket, `min_release_floor`). Each phase's result is its own: a
waiting or failed law phase does not skip the frameworks phase, nor the reverse.
`state.json` gains a `frameworks` key with its own accepted sequence, retained
and failed lists. The phase waits, with reason `release floor`, until the stored
[release floor](pull-deployment.md#release-floor) is at least
`min_release_floor`, so no updater that ignores frameworks state can run once it
exists.

The phase follows [host activation](corpus-release.md#host-activation) with
these differences:

- **Pull and verify.** Tag `frameworks-production`, the frameworks identity, and
  a verifier with the field list above that refuses any other `corpus_kind`.
  Until a frameworks manifest is accepted, a missing package is status `current`
  with "no frameworks release published".
- **Mode.** `retained` and `new` as for law, over `retired_release` rows of kind
  `frameworks`. `new` requires `previous` to equal the active frameworks
  release; with no `frameworks_release` row it requires `previous` null, the
  first-release bootstrap. Once the row exists, a null `previous` is refused for
  `new`.
- **Compatibility.** `server release-check --kind frameworks` prints
  `{"schema_versions": [...], "retrieval_recipe_versions": [...], "lexical_recipe_versions": [...], "query_recipe_hash": "..."}`;
  the manifest's schema version and recipe versions must be listed and its dense
  recipe's query hash must equal the running embedder's, else status `waiting`.
  Without `--kind` the output stays the law output, byte for byte.
- **Staging.** `/srv/trachuan/data/releases/frameworks-<rel>/`, mode `0700`,
  deleted when activation ends in any state.
- **Logs.** No session in a frameworks database may write an error, statement or
  parameter to the PostgreSQL log. Right after creating the database, as
  `postgres`, set `log_min_messages` and `log_min_error_statement` to `panic`,
  `log_min_duration_statement` to `-1` and `log_statement` to `none` on the
  database and, with `ALTER ROLE ... IN DATABASE`, for `activator` and
  `corpus_reader` in it, which takes precedence over any role-wide setting. The
  same step refuses when `pg_parameter_acl` grants `SET` on any of them to a
  role other than a superuser; without that grant no other role can change them
  in a session. Every session that reads a frameworks database (restore,
  prewarm, smoke, and each connection the server's frameworks pool opens) reads
  the four effective values first and stops, or is discarded, unless they are
  those. Superusers are the trusted boundary: the host's `postgres` role is the
  only one, and its own sessions set the values before touching the database.
  The updater and the server log only safe causes (SQLSTATE, step, object
  names).
- **Restore.** The TOC list keeps the law allowlist and also omits every view,
  function and trigger entry, refusing the dump when such an entry is not one
  the image's schema version defines, and refusing any other executable object
  (rule, policy, aggregate, operator, cast, event trigger). The dump supplies
  tables, data, sequences, constraints and indexes only. After the data,
  `server frameworks-schema --version <n>` prints the image's own views,
  functions and triggers for that schema version, applied as `activator`; a
  check constraint or default that needs a function the dump omitted fails the
  restore. Then `grant-corpus.sh`, which accepts `frameworks_<rel>` names,
  grants the corpus reader as for law.
- **Smoke.**
  `server release-check --kind frameworks --database frameworks_<rel>` opens the
  frameworks engine with a key made in memory for that run and runs one search
  and one read as a caller who may read every version, printing status and
  counts only.
- **Pointer.** Step 4 swaps `frameworks_release.active_db` from the journaled
  value, or inserts the row when there is none (bootstrap), and writes the old
  database to `retired_release` with kind `frameworks`. The law phase never
  writes `frameworks_release`; the frameworks phase never writes
  `corpus_release`.
- **Result.** `/srv/trachuan/state/deploy/release-frameworks.json` and the log
  `/var/log/trachuan/release-frameworks.log`; the release-floor wait is step
  `floor`; the metrics timer publishes `release_failed` and
  `release_waiting_age_seconds` with a `kind` dimension, and the alarms fire for
  either kind.

The daily prune handles retired databases of both kinds, parsing each name by
its kind's prefix. Recovery, protection and the seven-day window are the law
rules, per kind; an open journal is resumed only by the phase of its kind.

## Schema

One auth migration that an updater unaware of frameworks keeps working with,
since that updater may finish its own run after installing the image:

- `release_journal` and `retired_release` gain `corpus_kind` (`law` or
  `frameworks`) with default `law`, so existing rows and inserts that omit it
  stay law. A check pairs the kind with the database-name prefix. Unique keys
  and conflict targets stay as they are: names are distinct across kinds by
  prefix.
- `frameworks_release` gains the checks of `corpus_release`: country `vn` and an
  `active_db` that is a `frameworks_<rel>` name.
- `activator` may `SELECT`, `INSERT` and `UPDATE` `frameworks_release`;
  `provision-activator.sh` reapplies that grant.

The new updater's law phase reads only `law` rows; its frameworks phase only
`frameworks` rows.

## Key and flag

The server's frameworks key (cursor and `access_set_version` HMAC,
[results](frameworks-results.md)) is the Vault secret `server_frameworks_key`.
The owner generates it locally, as for `account_keys`, and never prints it:
`openssl rand -base64 32 | tr '+/' '-_' | tr -d '='`. The host mounts it at
`/run/secrets/server_frameworks_key` (owner `65532`, mode `0400`), and
`server.env` sets `SERVER_FRAMEWORKS_KEY_FILE` to that path and
`SERVER_FRAMEWORKS=false`. The server reads the key only with the flag on.
Rotating it invalidates frameworks cursors and access set versions; clients
search again.

A Podman secret exists only after an updater that lists it has reconciled, and a
container gets its mounts when it is created, so the first image to fetch the
secret is not the one that mounts it (order, below). Before activating an image,
the updater requires every secret that image's Quadlet files name to exist as a
Podman secret after its forced reconciliation. A missing one stops the
activation before any container changes: the result is `failed` at step
`secrets` with `secret missing: <name>`, alarmed like a pending secret
replacement; the serving image stays and the next run tries again. An updater
older than A rejects the unknown `server_frameworks_key` in its stored secret
state, so a rollback from A to an older image leaves that updater's secrets
phase failing each run, alarmed, until the host rolls forward; image activation
is unaffected, and once B has raised the floor that rollback is refused.

## Order

1. The owner creates `server_frameworks_key` in Vault and applies OpenTofu: the
   bucket; the frameworks verify user, group and its read statement; the upload
   group's and the host's statements for the bucket; and the secret name in the
   host's Vault read statement. The verify user's key is created and stored in
   the `frameworks-release` environment only after the signing workflow's change
   declares that environment, deployments from `main` only, in
   `deploy/github/settings.json` and it is applied, so no hand-made environment
   ever holds it.
2. Image A ships the migration, `release-check --kind`, `frameworks-schema`,
   `grant-corpus.sh` names, the updater's frameworks phase, and
   `server_frameworks_key` in the policy's secret list and the fetcher's
   allowlist, and the secret-presence check above. Its policy has no
   `frameworks` block, so the phase does not run; its updater keeps a
   `frameworks` state key it finds, untouched, so a rollback from B to A loses
   nothing. The updater that installs A reconciles nothing new; A's own updater
   fetches the secret.
3. Image B raises the release floor to A's sequence, adds the `frameworks` block
   with `min_release_floor` A's sequence, and adds the key mount and the
   `server.env` lines with the flag off. A's updater installs it only once the
   secret exists; until then each run fails with `secret missing`.
4. The first frameworks release is uploaded and signed; the host activates it.
5. Host memory is measured with both corpora active: the runbook starts a
   one-off server container from the serving image with the flag on, on loopback
   only and outside ingress, warms both kinds, records its resident memory
   against [host budgets](budgets-host.md) and stops it.
6. The owner's entitlement is created and, on the owner's decision, an image
   turns the flag on; acceptance runs with an entitled and a non-entitled
   account.

## Proofs

The law proofs of [corpus release](corpus-release.md#proofs) for the kind, plus:
the bootstrap accepted once with no pointer row and refused once a row exists; a
retry and a rollback signed and accepted in the right mode; a law manifest
refused by the frameworks verifier and the reverse; a frameworks activation
failure that leaves law activation, state and pointer untouched, and the
reverse; the phase waiting below `min_release_floor`; an updater from before the
migration completing a law activation against it; a dump carrying a function,
trigger or view outside the schema version refused before restore; the restored
executable objects equal to the image's; the effective log settings seen by each
role in a new session despite a conflicting role-wide setting, a session
override and a parameter grant refused, a server connection with other values
discarded, and an induced error with a marker absent from the PostgreSQL log; an
image whose Quadlet names a missing secret left waiting with the serving image
unchanged; the uploader's and the job's temporary directories empty after
success and failure; and no log line of the uploader, the job or the updater
holding anything but ids, sizes, hashes, counts and statuses.
