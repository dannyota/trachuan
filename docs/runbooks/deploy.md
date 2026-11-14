# Deploy a release

Releases deploy themselves: a protected `v*` tag with green CI on its commit
publishes a signed release, and the root `trachuan-deploy` timer on the
[production host](host.md) verifies and activates it
([pull deployment](../design/pull-deployment.md)). This page covers the one-time
bootstrap, the host proofs, and what to do when the updater stops.

## Release

Merge to `main`, wait for green CI, then tag the merge commit:

```sh
git tag v0.1.2 <commit>
git push origin v0.1.2
```

The `Images` run builds, attests, waits for `ci`, signs and publishes
`ghcr.io/dannyota/trachuan-release:v0.1.2` and moves `production`. The host
activates it within about five minutes. Watch `deploy_failed` and the custom log
`trachuan-host-jobs`.

## Bootstrap

The last planned SSH session. It needs the Podman secret `server_signup_emails`
and a release whose images, env files and units the host already runs.

1. Tag the release and wait for its `Images` run. On the MacBook, read the
   manifest hash:

   ```sh
   oras pull -o /tmp/rel ghcr.io/dannyota/trachuan-release:<version>
   shasum -a 256 /tmp/rel/manifest.json
   ```

2. Copy `/tmp/rel/host-bundle.tar.gz` to the host, unpack it to
   `/tmp/trachuan-host`, and run
   `sudo bash /tmp/trachuan-host/deploy/host/install.sh /tmp/trachuan-host/deploy/host`.
   The timer stays disabled because `/etc/trachuan/deploy-mode` does not exist.
3. Install the bundle's `server.env` and `web.env` over the host copies, write
   `/srv/trachuan/host/images.env` from the manifest's `images` and `upstream`
   (`SERVER_IMAGE=...`, one line per name), and deploy as in
   [Deploy a new digest](host.md#deploy-a-new-digest).
4. `sudo /usr/local/libexec/trachuan/trachuan-deploy bootstrap <manifest-sha256>`.
   It verifies the release and every image, confirms the host runs exactly that
   release, writes mode `verify` and enables the timer.

## Host proofs

In mode `verify`, run the proofs with an image built by an `Images` dispatch
from `main`:

```sh
sudo /usr/local/libexec/trachuan/proof.sh ghcr.io/dannyota/trachuan-server@sha256:<digest>
```

Each case (dispatch-build identity, wrong digest, replayed rollback, older
sequence) must be rejected at its named stage, with no state change, in
`/var/log/trachuan/deploy.log`.

## Activate

```sh
printf 'activate\n' | sudo tee /etc/trachuan/deploy-mode >/dev/null
sudo systemctl start trachuan-deploy.service
sudo /usr/local/libexec/trachuan/trachuan-deploy status
```

Confirm the result, then remove the deploy SSH key from `opc` and `trachuan` and
disable the Run Command plugin
([remote execution](../design/pull-deployment.md#remote-execution)).

## When the updater stops

`status` and `/srv/trachuan/state/deploy/result.json` name the step and reason.
A `stopped` journal keeps `/srv/trachuan/state/deploy-hold`, so Caddy stays down
even after a reboot. Fix forward: merge the fix and tag a new patch release; the
updater accepts any higher sequence. To return to an older release, dispatch
`rollback.yml` from `main` with the current and target manifest hashes; it
applies only when the auth schema is unchanged. After the first rollback
publish, confirm the package is public: an anonymous
`oras repo tags ghcr.io/dannyota/trachuan-rollback` must list its tag. Until
then the host logs `rollback channel unreadable` and withholds rollbacks.
