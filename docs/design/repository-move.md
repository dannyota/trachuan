# Repository move to dntmakers

Decision (owner, 2026-11-19): `github.com/dannyota/trachuan` moves to the
organization `github.com/dntmakers/trachuan`. This note owns how image releases,
corpus releases and the host keep verifying across the move. The release
contract itself is in [pull deployment](pull-deployment.md) and
[corpus signing](corpus-signing.md).

## What the transfer changes

| Item                              | After the transfer                                                                                                                         |
| --------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------ |
| Issues, pull requests, wiki       | Move ([transfer docs][transfer])                                                                                                           |
| Secrets, webhooks, deploy keys    | Stay with the repository ([transfer docs][transfer])                                                                                       |
| Variables, environments, rulesets | Not stated; checked and re-applied with `deploy/github/settings.py` after the transfer                                                     |
| Git and web URLs                  | Redirect to the new name until a repository or fork takes the old name                                                                     |
| Packages under `ghcr.io/dannyota` | Stay owned by `dannyota`; the moved repository's `GITHUB_TOKEN` can no longer write them. Anonymous pulls keep working                     |
| Signer identity                   | Fulcio names the workflow by owner: `https://github.com/dntmakers/trachuan/.github/workflows/<file>@<ref>`. Old signatures keep `dannyota` |
| Provenance attestations           | Name the source repository by owner, as signatures do                                                                                      |
| Go module `danny.vn/trachuan`     | Unaffected; the `replace` of `github.com/dannyota/onnxruntime/go` names another repository and stays                                       |

[transfer]:
  https://docs.github.com/en/repositories/creating-and-managing-repositories/transferring-a-repository

## Decisions

1. **No code names the owner.** Workflows publish to
   `ghcr.io/${{ github.repository_owner }}` and read the previous release from
   there. `deploy/github` takes the repository from `GITHUB_REPOSITORY` for the
   manifests' `repository`, image names, annotations, signing identities,
   `settings.py` and `wait_ci.py`; outside Actions it takes it from
   `--repository`, which is required. When `deploy/github` verifies an earlier
   artifact (the previous release, a previous corpus manifest), it accepts the
   repositories of `deploy/host/deploy-policy.json`, as the host does, because
   that artifact may carry the other owner's signature. Test fixtures may keep
   `dannyota` as sample data.
2. **The host policy accepts a list of repositories.**
   - `repositories` replaces `repository`: one or two `owner/name` entries.
   - `images` is keyed by repository, each with the four image names of that
     owner.
   - Each `*_identity` regular expression names both owners as alternatives
     while the list holds two.
   - The verifier reads `repository` from the signed manifest, requires it in
     `repositories`, and passes that exact value to cosign's
     `--certificate-github-workflow-repository` and to the provenance check. The
     manifest's images must be that repository's images. The signature binds the
     manifest bytes, so a manifest cannot claim the other owner.
   - Corpus and frameworks manifests follow the same rule.
   - `render-units.sh` reads the allowed image names from the installed policy
     instead of constants.
3. **The host reads one channel per kind, switched once by a copy.**
   `release_repository`, `rollback_repository`, `corpus_repository` and, when
   the policy has it, `frameworks.repository` stay single values. The first
   release from `dntmakers` points them at `ghcr.io/dntmakers`. Two copies with
   the owner's token bridge the switch. Before that release, every tag of the
   old channels is copied to the new registry, so its workflow finds the
   previous release and the host finds the current corpus releases. After its
   image workflow, its release artifact is copied back to the old release
   channel, where the running host still looks. `oras cp` keeps the bytes, so
   every copied artifact keeps its signature and its manifest hash, and the host
   reads it as already seen.
4. **A cleanup release drops `dannyota`.** It runs once every channel's
   production artifact is signed by `dntmakers`. The host verifies the
   production corpus manifest on every poll, before it decides the manifest was
   already seen (`deploy/host/updater/deploy_corpus.py`, step `manifest`), so a
   copied `dannyota` corpus manifest must be replaced first.

Rejected:

- **A repository-ID pin, as aboutme chose.** aboutme deploys by push and keeps
  both owners for good. Here the host pulls from a named channel, so the switch
  and its copies are needed anyway. The ID would also need a certificate
  extension parser on the host and in `deploy/github`. With the cleanup release,
  the floor already keeps rollback above the move.
- **A host that polls both channels.** It is permanent complexity for a one-time
  switch.

## Sequence

Each release has its own tag, green CI on its exact commit, the catalog audit
and the usual checks after deploy ([deploy runbook](../runbooks/deploy.md)).

1. **Release A, from `dannyota`.**
   - Ships decisions 1 and 2, with `repositories` set to `dannyota/trachuan` and
     `dntmakers/trachuan`.
   - Channels are unchanged. It must verify and activate like any release.
2. **Organization (owner).** Set Actions to the repository's policy: allowed
   actions, SHA pinning and a read-only default token. Let members create public
   packages.
3. **Transfer (owner).** Then the leader:
   - runs `settings.py` against `dntmakers/trachuan`;
   - checks the secrets `RELEASES_READ_KEY_*` and `FRAMEWORKS_READ_KEY_*`, the
     variable `OCI_S3_ENDPOINT`, the `production` environment with its branch
     policies, and the rulesets;
   - points local checkouts' `origin` at the new name.

   From the transfer until B is live:
   - tag nothing but B;
   - re-run no workflow run started before the transfer;
   - create nothing named `dannyota/trachuan`.

4. **Copy the channels in.**
   - With a token of the owner's that can write the organization's packages, run
     `oras cp --recursive` for every tag of `trachuan-release`,
     `trachuan-corpus-release` and, when it exists, `trachuan-rollback`, from
     `ghcr.io/dannyota` to `ghcr.io/dntmakers`.
   - Make each new package public.
   - Check with an anonymous pull that each copied `production`, `vn-production`
     and `frameworks-production` tag resolves to the same digest as the old one.
5. **Release B, from `dntmakers`.**
   - Policy channels point at `ghcr.io/dntmakers`, and `release_floor` is A's
     sequence.
   - Pause the host's deploy timer before tagging.
   - The image workflow reads the previous release from the copied channel and
     publishes B there. Its four image packages must pull anonymously.
   - Copy B's release artifact to `ghcr.io/dannyota/trachuan-release` and move
     `production` there.
   - Resume the timer. The host, under A's policy, verifies and activates B. Its
     next poll reads the new channels and finds everything already seen.
6. **Corpus releases from `dntmakers`.** The next `vn` release and a
   `frameworks` release or re-sign replace the copied production manifests.
7. **Release C.**
   - `repositories` holds only `dntmakers/trachuan`, and the identities name
     only `dntmakers`.
   - `release_floor` is B's sequence.
   - Ship it only after step 6, for the reason in decision 4.

Packages under `ghcr.io/dannyota` stay as they are. Nothing reads them after C.

## Failure handling

- **A fails to verify or activate.** It is an ordinary failed release, and the
  transfer waits.
- **The transfer drops a secret, variable, environment or ruleset.** Re-create
  it before B: the image and corpus workflows read the release bucket with those
  secrets.
- **B's copy back to the old channel is missed.** The host stays on A and
  reports nothing new. The copy can run at any time while the timer is paused.
- **A copied tag resolves to a different digest.** Stop before B. The host would
  read it as a new or changed release.
- **Rollback.** Before B, rollback works as today. Between B and C, the floor
  keeps it at or above A, whose policy accepts both owners.
