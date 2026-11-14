# Pull deployment

Decision: [ADR 0006](../adr/0006-single-host-oci-podman.md) (History
2026-11-14). A release tag on GitHub deploys code, images and the host bundle to
the production host with no SSH session. Corpus releases follow
[corpus release](corpus-release.md); secrets come from Vault
([deployment](deployment.md#secrets)).

## Repository settings

- Ruleset `main`: pull request required, required status check `ci`, no force
  push, no deletion.
- Ruleset `release-tags-fixed`: tags `v*` cannot be updated or deleted; no
  bypass.
- Ruleset `release-tags-create`: creating `v*` tags is restricted; bypass for
  the repository admin role. Rulesets make
  [`github.ref_protected`](https://docs.github.com/en/actions/reference/workflows-and-actions/contexts#github-context)
  true for release tags.
- Environment `production` with deployment tags `v*` and no required reviewer. A
  reviewer can be added later without changing code. Environments are available
  for public repositories on GitHub Free
  ([environments](https://docs.github.com/en/actions/how-tos/deploy/configure-and-manage-deployments/manage-environments)).
- Environment `corpus-release`, deployment branch `main`, holding the release
  bucket read key ([corpus release](corpus-release.md#signing-workflow)).

Workflow files change only through pull requests, which the `main` ruleset
already requires. `deploy/github/settings.json` holds these settings and
`deploy/github/apply-settings.sh` applies them with `gh api`; CI checks the
file's shape.

## Release workflow

`images.yml` gains a `release` job, `needs: images`, that runs only for a tag
push. A dispatch from `main` still builds and attests images for testing and
never produces a release. The job:

1. Requires `refs/tags/v<major>.<minor>.<patch>`, `github.ref_protected`, and
   the tag commit reachable from `main`.
2. Waits up to 60 minutes for the `ci` workflow run of the exact commit on
   `main` and requires conclusion `success`.
3. Builds the host bundle: `git archive --format=tar <commit> deploy/host`,
   compressed with `gzip -n`, so equal commits give equal bytes.
4. Writes the manifest below as canonical JSON: sorted keys, no insignificant
   whitespace, UTF-8, no wall-clock values.
5. Signs the exact bytes with
   [`cosign sign-blob --bundle`](https://docs.sigstore.dev/cosign/signing/signing_with_blobs/),
   keyless through GitHub OIDC, with a pinned cosign release.
6. Pushes `manifest.json`, `manifest.sigstore.json` and the bundle tarball as
   one OCI artifact to `ghcr.io/dannyota/trachuan-release`, tagged with the
   version and `production`, using a pinned `oras`. Tags are discovery hints;
   the host checks content.

`images.yml` attests each image with `push-to-registry: true`, so the SLSA
provenance is an OCI referrer next to the image and the host verifies it
anonymously. The certificate identity of both the release signature and the
build attestations is
`https://github.com/dannyota/trachuan/.github/workflows/images.yml@refs/tags/v<version>`.
Permissions per job are the minimum: `id-token: write`, `packages: write`,
`attestations: write`, `contents: read`, `actions: read`.

### Image manifest

```json
{
  "kind": "trachuan.image-release",
  "version": 1,
  "repository": "dannyota/trachuan",
  "commit": "<40 hex>",
  "ref": "refs/tags/v0.2.0",
  "release": "v0.2.0",
  "sequence": 1234,
  "target": "production",
  "images": {
    "server": "ghcr.io/dannyota/trachuan-server@sha256:<64 hex>",
    "web": "...",
    "queryembed": "...",
    "postgres": "..."
  },
  "upstream": { "caddy": "docker.io/library/caddy@sha256:<64 hex>" },
  "host_bundle": { "sha256": "<64 hex>", "size": 123456 },
  "corpus_release": null,
  "rollback_target": null
}
```

`sequence` is `github.run_number` of `images.yml`: increasing, with gaps from
dispatch builds. A re-run keeps the number; if it yields different bytes, the
host rejects it as sequence reuse and a new patch tag is needed. `upstream`
comes from `deploy/host/images.env` at the commit; upstream images are pinned by
digest only, since they carry no build attestation from this repository.
`corpus_release`, when set, names a corpus release and its manifest hash for a
[recipe change](corpus-release.md#recipe-changes). `rollback_target` is always
null from this workflow.

### Rollback workflow

`rollback.yml`, `workflow_dispatch` from `main`, environment `production`,
inputs `current` and `target` (manifest SHA-256). It fetches both published
manifests, requires `target` to have a lower sequence and
`rollback_target: null`, and signs
`{"kind":"trachuan.rollback","version":1,"current":...,"target":...,"sequence":<run_number>}`
the same way, published as `ghcr.io/dannyota/trachuan-rollback:<sequence>`.

## Host policy and state

`/etc/trachuan/deploy-policy.json`, root 0644, installed by the bundle:
repository, the two workflow identities as anchored regular expressions, OIDC
issuer `https://token.actions.githubusercontent.com`, artifact repositories,
image names, upstream repositories, mode (`verify` or `activate`), and the
Sigstore trust root pinned by the cosign release. Values come from this policy
only, never from a manifest.

State in `/var/lib/trachuan-deploy/`, root 0700: active manifest hash, highest
image sequence, highest rollback sequence, consumed rollback authorizations,
retained manifests and bundles (the last five), installed bundle hash, and the
journal. Application containers and the `trachuan` user cannot write policy or
state. Lost state is not reset to zero: the updater stops with `deploy_failed`
until [bootstrap](#bootstrap) runs again.

## Updater

`trachuan-deploy.timer` starts the root `trachuan-deploy.service` every 5
minutes. One run holds `/run/trachuan-deploy.lock`. Before restarting PostgreSQL
it also takes the backup lock (`/srv/trachuan/state/backup/backup.lock`) without
waiting; when a backup holds it, the run ends without changes and the next run
retries.

1. Fetch: resolve `ghcr.io/dannyota/trachuan-release:production` and the
   rollback repository's newest tag; download into a fresh staging directory.
2. Verify the manifest: `cosign verify-blob --bundle` with
   `--certificate-oidc-issuer` and `--certificate-identity-regexp` from policy;
   the transparency log proof is required and no skip flag is ever passed. Check
   `kind`, `version`, `repository` and `target`, and bind the manifest's `ref`
   and `commit` to the certificate with `--certificate-github-workflow-ref` and
   `--certificate-github-workflow-sha`.
3. Decide: a manifest whose hash is active or retained is a no-op. Accept a new
   manifest only when its sequence is above the stored highest; reject an equal
   or lower sequence with different bytes.
4. Verify images: pull each by digest, then verify its build attestation from
   the registry with the build identity from policy and require the SLSA v1
   subject digest, source repository, commit and ref to equal the manifest. The
   verifier is `cosign verify-attestation --new-bundle-format`
   ([bundle support](https://blog.sigstore.dev/cosign-verify-bundles/)); the
   first implementation step proves it reads GitHub's referrer bundles
   anonymously, and otherwise uses a pinned
   `gh attestation verify --bundle-from-oci`. Upstream images: digest equality
   only. Reject tags, unlisted repositories and missing claims.
5. Verify the host bundle: size and SHA-256 equal the manifest.
6. Activate (mode `activate` only):
   - Journal `started` with the manifest hash and the auth schema version
     (`max(version_id)` in `goose_db_version`).
   - When the bundle hash differs from the installed one: unpack into
     `/srv/trachuan/bundles/<sha256>/` and run its `install.sh` as root.
   - Write `/srv/trachuan/host/images.env` from the manifest; install the
     bundle's `server.env` and `web.env`; render units; `daemon-reload`.
   - Stop Caddy, web, server and queryembed. Restart PostgreSQL only when its
     image or rendered unit changed; then wait for it, run `pgbackrest check`
     and prewarm the active corpus database
     ([release restore](../runbooks/release-restore.md#prewarm-a-corpus-database)).
   - Start queryembed and migrate, then server and web; require `/readyz`; start
     Caddy; require `https://trachuan.vn/readyz` through Caddy.
   - Journal `activated`; store the new hash and sequence.
7. Failure after step 6 begins: when the auth schema version is unchanged and a
   previous manifest is retained, activate it the same way (its bundle's
   `install.sh` runs again when the bundle differs) and journal `rolled_back`.
   Otherwise leave Caddy stopped and journal `stopped`. Either way record
   `deploy_failed`; the failed manifest is never retried, and a higher sequence
   is still accepted. A fetch or verification failure changes nothing on the
   host.

A signed rollback authorization is applied when its `current` equals the active
hash, its `target` is retained, its sequence is above the stored rollback
sequence, and the target's auth schema version equals the current one. It is
consumed even when activation fails.

Image activation that needs a model the host does not have fails at queryembed
readiness. Model provisioning stays a manual owner step.

## Bootstrap

The leader's last planned SSH session installs the bundle containing the
updater, writes the policy in mode `verify`, and runs
`trachuan-deploy bootstrap <manifest-sha256>`, which pins that manifest as
active and its sequence as the floor after full verification. Switching to
`activate` is a policy change shipped in a release.

## Results and alarms

Each run writes `/srv/trachuan/state/deploy/result.json` (0640, readable by
`trachuan-metrics`): status, manifest hash, release, sequence, step, message and
timestamps; never secret values. The metrics timer publishes `deploy_failed` (1
when the newest result is `failed`, `rolled_back` or `stopped`) and
`deploy_last_success_age_seconds` to the `trachuan_host` namespace. Alarms
through topic `trachuan-alarms` email the owner on `deploy_failed` and on a
missing metric. Log lines go to `/var/log/trachuan/deploy.log`, shipped by
[OCI Logging](deployment.md#observability).

## Remote execution

After cutover the deploy SSH key is removed from `opc` and `trachuan`; the
owner's key and the OCI serial console remain for break-glass. The Compute
Instance Run Command plugin is disabled because no job uses it.

## Proofs

Unit tests with recorded fixtures cover each rejection: a dispatch build from
`main`, an unprotected or non-release ref, a wrong commit, a wrong or missing
image digest, an unlisted image, an older sequence, sequence reuse, a replayed
or consumed rollback, a bundle hash mismatch, and lost state; and the no-op for
a seen manifest. On the host in mode `verify`, the owner's proof script feeds a
dispatch-build manifest, a wrong digest, a replayed rollback and an older
sequence, and each is rejected in `deploy.log` without a change.
