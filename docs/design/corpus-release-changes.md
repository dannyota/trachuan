# Corpus release changes

Part of [corpus release](corpus-release.md).

## Schema version changes

`server release-check` refuses a release whose `schema_version` the serving
image cannot read. A version change ships in this order:

1. Tag and deploy the first image whose server reads the new version; it keeps
   serving the active release. Confirm it live and healthy.
2. Tag the next image with `release_floor` in its policy set to the sequence of
   the image from step 1 ([release floor](pull-deployment.md#release-floor)) and
   confirm `deploy_release_floor` reports it, so no rollback can reach an image
   that cannot read the new version.
3. Upload and sign the corpus release of the new version; the host activates it
   as usual. The retained previous-version release, still readable, stays the
   corpus rollback for seven days.

## Recipe changes

The pair activation path is not yet implemented; recipe mismatches wait and
image manifests with a non-null `corpus_release` are refused.

A release whose recipes differ from the running server waits until the
[image manifest](pull-deployment.md#image-manifest) for the matching code names
it in `corpus_release`. The updater activates the pair in one locked run:

1. Verify both manifests and the images. Run activation steps 0 to 2 for the
   corpus while the old pair serves.
2. Stop Caddy; activate the images up to started containers, without the
   readiness gate.
3. Run step 3 with the new server image, then step 4, then require loopback
   `/readyz`, start Caddy and run the edge probe; a failed probe stops Caddy.
4. On failure after step 4, one transaction reverses it: compare-and-swap the
   pointer back, delete the old release's `retired_release` row, insert the new
   one with its digest and `retain_until` seven days ahead, and journal
   `reverted`. Then restore the previous images under the
   [failure rule](pull-deployment.md#failure-and-recovery) and start Caddy once
   the previous pair passes loopback readiness, then run the edge probe; a
   failed probe stops Caddy and records the failure. An interrupted pair run is
   recovered by the same rule: its image journal is treated as failed, and a
   corpus journal ending in `activated` for that pair is reversed first.
