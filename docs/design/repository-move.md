# Repository move to dntmakers

Decision (owner, 2026-11-19): `github.com/dannyota/trachuan` moves to the
organization `github.com/dntmakers/trachuan`. This note owns how image releases,
corpus releases and the host keep verifying across the move. The release
contract itself is in [pull deployment](pull-deployment.md) and
[corpus signing](corpus-signing.md).

## What the transfer changes

| Item                              | After the transfer                                                                                                                                                |
| --------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Issues, pull requests, wiki       | Move ([transfer docs][transfer])                                                                                                                                  |
| Secrets, webhooks, deploy keys    | Stay with the repository ([transfer docs][transfer])                                                                                                              |
| Variables, environments, rulesets | Not stated; checked and re-applied with `deploy/github/settings.py` after the transfer                                                                            |
| Git and web URLs                  | Redirect to the new name until a repository or fork takes the old name                                                                                            |
| Packages under `ghcr.io/dannyota` | Stay owned by `dannyota`; the moved repository's `GITHUB_TOKEN` can no longer write them ([package permissions][perm]). Anonymous pulls keep working              |
| New packages                      | Created by the moved repository's `GITHUB_TOKEN`: linked to it and as visible as it is (public). Created with a personal token: linked to nothing until connected |
| Signer identity                   | Fulcio names the workflow by owner: `https://github.com/dntmakers/trachuan/.github/workflows/<file>@<ref>`. Old signatures keep `dannyota`                        |
| Workflow run numbers              | Not stated. Image and corpus sequences are run numbers, so they are checked                                                                                       |
| Go module `danny.vn/trachuan`     | Unaffected; the `replace` of `github.com/dannyota/onnxruntime/go` names another repository and stays                                                              |

[transfer]:
  https://docs.github.com/en/repositories/creating-and-managing-repositories/transferring-a-repository
[perm]:
  https://docs.github.com/en/packages/learn-github-packages/about-permissions-for-github-packages

Facts on 2026-11-19: the repository is public; `dntmakers` is on the Free plan;
the last run numbers are 59 for `images.yml` (sequence 59, v0.1.45) and 5 for
`corpus-release.yml`; `rollback.yml` and `frameworks-release.yml` have never
run, the `trachuan-rollback` package does not exist, and the host policy has no
`frameworks` block.

## Decisions

1. **No code names the owner.**
   - Workflows publish to `ghcr.io/${{ github.repository_owner }}` and read the
     previous release from there.
   - `deploy/github` takes the repository from `GITHUB_REPOSITORY` for the
     manifests' `repository`, image names, annotations, signing identities,
     `settings.py` and `wait_ci.py`. Outside Actions it takes it from a required
     `--repository`.
   - When `deploy/github` verifies an earlier artifact (the previous release, a
     previous corpus manifest), it accepts the repositories of
     `deploy/host/deploy-policy.json`, as the host does.
   - Host files read image names from the policy: `render-units.sh`,
     `check-mounts.py`, `images.env` and `proof.py`. Where they run without an
     installed policy (CI), they take a policy path. Test fixtures may keep
     `dannyota` as sample data.
2. **The host policy accepts a list of repositories.**
   - `repositories` replaces `repository`: one or two `owner/name` entries.
   - `images` and every `*_identity` are keyed by repository, each with that
     owner's names.
   - Release, corpus and frameworks manifests carry `repository`. The verifier
     requires it in `repositories` and passes that exact value, with that
     repository's identity, to cosign's
     `--certificate-github-workflow-repository` and to the provenance check. The
     manifest's images must be that repository's images. The signature binds the
     manifest bytes, so a manifest cannot claim the other owner.
   - A rollback manifest carries no `repository`. It is verified against each
     entry in turn, with that entry's rollback identity.
3. **The host reads one channel per kind, switched once by a copy.**
   - `release_repository`, `rollback_repository`, `corpus_repository` and, when
     the policy has it, `frameworks.repository` stay single values. The first
     release from `dntmakers` points them at `ghcr.io/dntmakers`.
   - Two copies bridge the switch. Before that release, every tag of the old
     release and corpus channels is copied to the new registry, so its workflow
     finds the previous release and the host finds the current corpus release.
     After its image workflow, its release artifact is copied back to the old
     release channel, where the running host still looks.
   - `oras cp` keeps the bytes. The signature bundle is a layer of the artifact,
     not a referrer, so every copied artifact keeps its signature and its
     manifest hash, and the host reads it as already seen.
4. **A cleanup release drops `dannyota`.** It runs once no artifact the host
   verifies on a poll is signed by `dannyota`:
   - the production release: from the first `dntmakers` release on;
   - the production corpus manifest, which the host verifies on every poll
     before it decides the manifest was already seen
     (`deploy/host/updater/deploy_corpus.py`, step `manifest`): after a `vn`
     release built and signed by `dntmakers` is activated on the host;
   - the highest rollback tag, which the host verifies on every poll before
     `production` (`deploy_run.py`): no tag exists, and no rollback is published
     from the transfer until the release after the cleanup.

Rejected:

- **A repository-ID pin, as aboutme chose.** aboutme deploys by push and keeps
  both owners for good. Here the host pulls from a named channel, so the switch
  and its copies are needed anyway. The ID would also need a certificate
  extension parser on the host and in `deploy/github`. With the cleanup release,
  the floor keeps rollback above the move.
- **A host that polls both channels.** It is permanent complexity for a one-time
  switch.
- **Re-signing pre-move corpus releases under `dntmakers`.** A re-sign may not
  change signed fields such as `repository` (`corpus_production.py`). A new run
  is simpler.

## Sequence

Each release has its own tag, green CI on its exact commit, the catalog audit
and the usual checks after deploy ([deploy runbook](../runbooks/deploy.md)).

1. **Release A, from `dannyota`.**
   - Ships decisions 1 and 2, with `repositories` set to `dannyota/trachuan` and
     `dntmakers/trachuan`. Channels are unchanged.
   - It must verify and activate like any release.
   - From A until the release after C, nobody dispatches `rollback.yml`.
2. **Organization (owner).** Set Actions to the repository's policy: allowed
   actions, SHA pinning and a read-only default token. Let members create public
   packages.
3. **Transfer (owner).** Then the leader:
   - runs `settings.py` against `dntmakers/trachuan`;
   - checks the rulesets, including that `v*` tags are protected, since
     `images.yml` refuses an unprotected tag, and the three environments
     (`production`, `corpus-release`, `frameworks-release`) with their branch
     policies;
   - checks the secrets `RELEASES_READ_KEY_*` and `FRAMEWORKS_READ_KEY_*` and
     the variable `OCI_S3_ENDPOINT`;
   - checks that the last run numbers of `images.yml` and `corpus-release.yml`
     are unchanged, and stops the move if not;
   - points local checkouts' `origin` at the new name.

   From the transfer until B is live:
   - tag nothing but B;
   - dispatch no corpus, frameworks or rollback workflow;
   - re-run no workflow run started before the transfer;
   - create nothing named `dannyota/trachuan`.

4. **Prove the new identity.**
   - Dispatch `images.yml` from `main`. It builds, signs and attests the images
     and creates the four `ghcr.io/dntmakers/trachuan-*` image packages through
     `GITHUB_TOKEN`; it publishes no release, since `images.yml` releases only
     on a tag push.
   - Check that each pulls anonymously.
   - Verify its image signature and provenance under A's `dntmakers` identity,
     as `proof.py` does.
5. **Copy the channels in.**
   - With the owner's token (`write:packages`), run `oras cp` for every tag of
     `trachuan-release` and `trachuan-corpus-release`, from `ghcr.io/dannyota`
     to `ghcr.io/dntmakers`.
   - The copies are linked to nothing. In each package's settings, connect it to
     `dntmakers/trachuan`, grant the repository Write under Actions access, and
     make it public.
   - Check with an anonymous pull that every copied tag resolves to the same
     digest as the old one.
6. **Release B, from `dntmakers`: the switch only.**
   - Policy channels point at `ghcr.io/dntmakers`, and `release_floor` is A's
     sequence.
   - Living docs that name the owner change with it: pull deployment, corpus
     signing, corpus release, frameworks release, frameworks and the deploy
     runbook.
   - Pause the host's deploy timer before tagging.
   - The image workflow reads the previous release from the copied channel and
     publishes B there.
   - Copy B's release artifact to `ghcr.io/dannyota/trachuan-release` with the
     owner's token, and move `production` there.
   - Resume the timer. The host, under A's policy, verifies and activates B. Its
     next poll reads the new channels and finds everything already seen.
7. **A `vn` corpus release from `dntmakers`.** A new run, built and signed by
   `dntmakers` and activated on the host.
8. **Release C.**
   - `repositories` holds only `dntmakers/trachuan`.
   - `release_floor` is B's sequence. `proof.py`'s fixture becomes a `dntmakers`
     release.
   - Ship it only after step 7, for the reasons in decision 4.
9. **The release after C** raises `release_floor` to C's sequence, so no
   rollback restores trust in `dannyota`. Rollback publishing resumes.

Packages under `ghcr.io/dannyota` stay as they are. Nothing reads them after C.

## Failure handling

- **A fails to verify or activate.** It is an ordinary failed release, and the
  transfer waits.
- **The transfer drops a secret, variable, environment or ruleset.** Re-create
  it before B: the image and corpus workflows read the release bucket with those
  secrets.
- **Run numbers restart.** Stop before B. The host would refuse B as an older
  sequence; sequences then need their own design.
- **The identity proof fails.** Stop before B, and keep A.
- **A copied tag resolves to a different digest.** Stop before B. The host would
  read it as a new or changed release.
- **B's copy back to the old channel is missed.** The host stays on A and
  reports nothing new. The copy can run at any time while the timer is paused.
- **B fails to activate.** B changes only the policy, so the host recovers to A
  by itself, still reading the old channel.
- **Rollback.** No rollback is published from A until the release after C. From
  B on, the floor keeps any target at or above A. A rollback to A from B or
  later would reinstall A's channels, so it needs B's bridge by hand.
- **Corpus retry or rollback during the move.** A pre-move corpus release cannot
  be re-signed under `dntmakers`. The fix is a new run.
