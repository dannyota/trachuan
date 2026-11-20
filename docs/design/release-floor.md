# Release floor

This note owns the image release floor of [pull deployment](pull-deployment.md):
how the host stores it and what it refuses.

The policy may set `release_floor`, an image sequence. Every release at or above
the first floor-aware release reads an optional `release_floor` key in
`state.json`; the key is written only once a floor is in force, so older
updaters still read their state before then, and once it is set no older updater
can run again. The key is saved in the same atomic write that journals
`activated`, as the maximum of its previous value and the floor in the activated
bundle's policy; it never falls, and a lower or missing policy value leaves it
unchanged. With a floor in force:

- verification rejects a candidate whose sequence is below the floor its own
  bundle declares, before installation;
- acceptance refuses an image manifest below the stored floor;
- a rollback authorization whose target is below it is refused and not consumed;
- failure recovery never reactivates a retained manifest below it: the journal
  reads `stopped`, the result names the release floor, and the hold keeps Caddy
  down whichever phase runs next.

Losing `state.json` is the existing lost-state case: the updater changes nothing
until bootstrap, so the serving image stays. Corpus manifests are unaffected.
The result file carries `release_floor` and the metrics timer publishes
`deploy_release_floor`. Because a rollback installs the target's own updater,
the floor must be at least the sequence of the first floor-aware release: that
release ships with no floor and the next sets `release_floor` to its sequence. A
feature that depends on the floor ships in a later release, after the floor is
confirmed active ([accounts](accounts.md)).
