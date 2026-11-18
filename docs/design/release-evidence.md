# Release evidence

Decision: [ADR 0017](../adr/0017-findings-and-reviewed-knowledge.md), item 7.
This note owns the three files a corpus release carries beside its dump, their
bindings and the release gate. Detectors, findings and reviewed knowledge are in
[findings and reviewed knowledge](knowledge-loop.md).

The `evidence` stage runs after `eval` and writes one immutable artifact that
binds, and never changes, the build artifact and the evaluation report.

## Conventions

- Every SHA-256 in the three files is the string `sha256:` followed by 64
  lower-case hexadecimal characters.
- A provision's text hash is over the UTF-8 bytes of its `text` column exactly
  as stored: no normalization, not its subtree. An empty text hashes the empty
  byte string. A reader of the dump can recompute every hash and get the same
  value.
- `identity.jsonl` and `changes.jsonl` hold one JSON object per line, at most 1
  MiB each. The first line holds the schema version and the bindings. Later
  lines are sorted by record type and then by document key, anchor, or a
  relation's identity: the bytes of its line before the `count` member, which
  comes last, so the order is read from the file itself and needs no serializer.
  Equal inputs give equal bytes. A relation's identity is its kind, method and
  two ends; equal rows compare as a count. Taxonomy codes are sorted by code.
- An anchor with no `~k` component is derived from the kinds and tokens on the
  provision's path ([text structure](text-structure.md)). A `~k` suffix numbers
  repeated anchors in tree order and is not stable across releases. A provision
  is **ambiguous** when any component of its anchor has a `~k` suffix or a `~k`
  sibling, the rule reviewed knowledge and build use
  ([relations from text](relations-text.md)). Nothing may be pinned to an
  ambiguous provision.

## Selected report

A run decides what it is compared with when it is created. `corpus start` of a
new run requires exactly one of:

- `--previous-report <report.json>`: the uploaded report of the production
  release ([corpus release](corpus-release.md)). Start decodes it, requires
  report version 2 with its retrieval recipe, status complete, this run's
  country and a `rel` below this run's, and copies the bytes into the artifact
  store.
- `--no-previous-report`: the run is compared with nothing and cannot publish.

Neither or both refuses the start, so a forgotten flag cannot look like a
baseline. Both are unrelated to `--previous-manifest`, the ingest baseline. The
snapshot freezes the **decision**: the file's SHA-256 and store path, or `none`.
Two decisions are the same when their SHA-256 are equal. While the run has no
`eval` item a start may replace the decision and prints both SHA-256; after that
a different decision is refused.

The `eval` stage reads the stored file, fails when its SHA-256 differs, and puts
the frozen SHA-256 in its input hash, so another previous report cannot reuse an
evaluation. Its item is done only when the report matches the decision:
`previous_diff.previous.report_sha256` equals the frozen SHA-256, or, for
`none`, `previous_diff` names no previous release. An incompatible comparison
fails the item, also when an existing artifact is reused. The `report.json` of
that item is the **selected report**: `evidence` reads it and it is the file a
release uploads. `corpus eval` stays a diagnostic and its report cannot publish.
This change alters no recipe from `discover` to `eval`; the server checks only
the schema, query, lexical and retrieval recipes.

The `evidence` stage ships with this rule. For a run with a decision it is the
last stage: only its completion makes the run `evaluated`, `--until eval` pauses
the run, and `corpus release-row` reports `complete` only with its item done.

### Upload and signing

`corpus release-row` adds `eval_report_sha256`, `previous_report_sha256` (64
hexadecimal characters, or `none`) and, from the run's verified evidence
artifact, `findings_sha256`, `identity_sha256` and `changes_sha256` to `run`.
The uploader and signing both require the report to hash to `eval_report_sha256`
and `previous_diff.previous.report_sha256` to equal `previous_report_sha256`.

Upload also requires a settled production: the report's previous `rel` and
SHA-256 equal `rel` and `report.sha256` of the verified `<cc>-production`
manifest, and `release` of `GET https://trachuan.vn/api/v1/public/status`, asked
with `Cache-Control: no-cache`, is that same `rel`. Any other answer refuses,
and so does a country other than `vn` until the endpoint takes one. A release
still activating, waiting on recipes, failed or rolled back by hand therefore
blocks the upload, and if production has moved on the release needs a new run
and `rel`.

Signing repeats the manifest comparison first, before it signs or pushes
anything, inside the workflow's concurrency group, so a refusal leaves no tag
and the second of two uploads that froze one previous is refused. It is exempt
only when a `<cc>-<rel>` manifest already exists with a sequence at or below
production's, which keeps retry and rollback working. When a verified
`<cc>-<rel>` manifest exists, every field it holds keeps its value in the new
manifest, except `commit`, `sequence` and `heading_review`, so a `rel`'s signed
content is fixed by its first signing. That exemption cannot tell a rollback
from a release that was pushed but never became production: signing such a
release moves the tag, the host refuses it because its `previous` is not the
active release, and signing the active release again recovers.

### Runs before this rule

A snapshot without a decision is valid only at or below the cutoff
`vn/20261118_2`, one value in the Go and Python code, bound by a test. Such a
run has no `evidence` stage and becomes `evaluated` after `eval`; a restart
never appends `evidence` to it and a decision flag on it is refused. It uploads
a report from `corpus eval --previous`, and release-row writes neither field, so
it skips only the two hash checks: settled production and the signing comparison
still apply. Above the cutoff, `corpus start` refuses a run without a decision
unless a flag adds one while the run has no `eval` item, and planning,
release-row, upload and signing refuse a missing decision. Upload accepts the
older shape only for exactly the cutoff. Signing accepts it for exactly the
cutoff, or below it only when a verified `<cc>-<rel>` manifest already exists,
so older releases can be signed again and no new one appears under a back-dated
`rel`. The first run with a decision starts after the cutoff release is
published and active.

## `findings.json`

Schema version; bindings (`rel`, the dump's SHA-256, the build artifact hash,
the report's SHA-256, the detector registry hash, the predecessor's `rel`, or
`none` without one, and the SHA-256 of its `findings.json`, or `none` when it
has none); per detector `complete: true`, its registry severity and the number
of subjects observed, not observable and excluded, with an entry for every
cross-signal detector of the stage's recipe; every finding with key,
fingerprint, semantics number, `first_rel`, severity, status and `carried`; and
the keys that are new, gone and carried.

## `identity.jsonl`

This release's identity summary. Its bindings are `rel`, the dump's SHA-256, the
build artifact hash and the report's SHA-256. Then:

- per document: key, `number_norm`, status class, `eff_from`, `eff_to`,
  `is_current`, `validity_conflict`, text source, completeness and
  `baseline: true` when the document is served from a baseline; an active member
  with no document row has a line with its key and `absent: true`;
- per provision: document key, anchor, `parent` (the anchor of its parent
  provision, or null at the top of the document), `ambiguous: true` when the
  rule above holds, its text hash, the codes of its effective positive taxonomy
  nodes, resolved by the serving rules of [taxonomy](taxonomy.md), and
  `validity`, its `provision_validity` rows (status class, `eff_from`, `eff_to`,
  method), an empty list while that table has no row for it;
- per relation: kind, method and two ends. Every row of the relation table is
  written, whatever the class, text source or completeness of either document.
  An end has `key`, `number`, `in_release`, and `anchor`: the anchor of the
  provision the row's provision column points to, or null when that column is
  null, which is a document-level end or a provision end build did not resolve
  ([relations from text](relations-text.md#resolution-in-build)). The target end
  also has:
  - `exact`, only when `anchor` is not null, with the meaning of
    [MCP results](mcp-results.md);
  - `named`, for a `text_rule/1` or `reviewed` row whose requested path is not
    exactly `anchor`: the path the act names, from `target_node_id`. It is
    present with a null `anchor` when build resolved no provision, and with the
    deepest resolved ancestor as `anchor` when a clause or point is missing;
  - `node`, for a row of another method with a null `anchor` and a stored
    `target_node_id`: the source's node id as stored. It is opaque and is not a
    path.

## `changes.jsonl`

Its bindings add the predecessor's `rel` and the SHA-256 of the predecessor's
`identity.jsonl`, or `none`. The predecessor's signed manifest carries the same
hash, so a reader checks that the list applies to the summary it holds and
refuses it otherwise; a skipped release shows as a predecessor it does not hold.
The stage runs offline and before signing, so it binds the file, not a manifest
digest.

Records are before and after pairs by public identity, from this summary and the
predecessor's:

- documents added and removed, each with a cause class: `membership`, `coverage`
  (a fetch gap, a baseline, a withheld text) or `source`, read from this run's
  frozen membership inventory. A removed key whose disposition is
  `source_replaced` is `source`; one that is `withdrawn`, `scope_removed` or has
  no entry is `membership`; one still active is `coverage`. An added key is
  `source` when a hand-over names it, `coverage` when the predecessor lists it
  as absent, and otherwise `membership`. A removed document's provisions are
  removed with it, under its cause class, and are not listed one by one;
- document key: the key before and after, only for a hand-over that fetch
  recorded as `source_replaced` naming both keys (a release requires that gap
  accepted) ([sources](sources.md#secondary-discovery-and-hand-over)), and only
  when the old key has a document line in the predecessor's summary and the new
  key has one in this summary, neither key has a line on the other side, and no
  other old key names the same replacement. The document is then a surviving
  document, compared as one: metadata and classification as on a surviving key;
  provisions paired by equal anchor, each record carrying `provenance: true`;
  relation ends compared with the old key read as the new key, so a relation
  that differs only by the key is unchanged. When the new key has no document
  line, as after a failed fetch of the replacement, or when the pair fails
  another condition, the old document is a removal with cause `source` that
  names the replacement key in `replaced_by`, and the replacement is an addition
  in the release that first holds it. Any other change of key is one removal and
  one addition;
- document metadata: `number_norm` before and after on a surviving key;
- provisions added, removed and changed (text hash before and after) in
  surviving documents. Each carries `provenance: true` when its document's text
  source or completeness changed in this release, so a new extraction is not
  read as a change of law, and `ambiguous: true` when the provision is ambiguous
  in either summary;
- legal changes: status class, `eff_from`, `eff_to`, `is_current`,
  `validity_conflict`, relations and effects added and removed, and
  `provision_validity` rows added and removed;
- provenance changes: text source, completeness, served from a baseline;
- classification changes: node codes added to and removed from a provision.

Classification and validity records carry the same `provenance` and `ambiguous`
flags as provision records. The record types are `document`, `document_key`,
`document_metadata`, `document_legal`, `document_provenance`, `provision`,
`provision_classification`, `provision_validity` and `relation`.

Added, removed and hand-over rules count only document lines that are not
`absent`. When the predecessor has no summary (it predates evidence, or there is
none), the file says `baseline` and lists nothing.

It reports no "moved" provision: without a verified correspondence between two
identities, a move is one removal and one addition for the reader to review.

## Upload

The predecessor is frozen when the run is created, so comparing with another
release takes a new run. Once a `rel` has an upload prefix it is never reused
([corpus release](corpus-release.md)). A release whose `release.json` carries
the [decision fields](#upload-and-signing) uploads `findings.json`,
`identity.jsonl` and `changes.jsonl` beside the dump, `MANIFEST.sha256`,
`report.json` and `release.json`. Its signed corpus manifest gains
`evidence: {findings_sha256, identity_sha256, changes_sha256}`, each 64
lower-case hexadecimal characters.

`findings.json` has the 16 MiB limit of the other JSON files. `identity.jsonl`
and `changes.jsonl` may reach 1 GiB each and are hashed and validated line by
line while streaming, because a release holds hundreds of thousands of
provisions.

The host updater accepts the optional `evidence` field and does not read the
files. An older updater refuses the manifest at verification, before it binds
the sequence, and the host accepts the same manifest once the newer updater is
installed. Before the first release with a decision is signed, that updater is
live and the [release floor](pull-deployment.md#release-floor) is confirmed at
its image, so an image rollback cannot strand corpus activation.

Upload and signing both apply every row of the gate. The five `run` fields
appear together or not at all. Upload and signing require the three file hashes,
compared without the `sha256:` prefix, to equal the `run` fields of
`release.json`, and the predecessor's `rel`, findings hash and identity hash to
equal the `evidence` of its verified manifest, or `none` when that manifest has
none.

## Gate

| Condition                                                                                                                    | Result                                                                                                                                       |
| ---------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------- |
| every existing rule of corpus release: items complete, every gap accepted, evaluation gates                                  | unchanged                                                                                                                                    |
| a sidecar missing, or a binding that does not match the dump, the build artifact, the report or the report's `previous_diff` | refuse                                                                                                                                       |
| a detector not complete                                                                                                      | refuse                                                                                                                                       |
| an `open` finding whose detector is `blocking` in `findings.json`, or has no entry there, fired or carried                   | refuse                                                                                                                                       |
| `open` findings of `review` and `info` detectors                                                                             | allowed, counted                                                                                                                             |
| first release with evidence                                                                                                  | allowed: the predecessor's identity is recorded, its findings are empty and `changes.jsonl` is `baseline`; every existing rule still applies |

The sidecar rows apply exactly to a release whose `release.json` carries the
decision fields. Above the [cutoff](#runs-before-this-rule) a release without
them is refused, so they cannot be dropped to avoid these rows. Their change to
the `evidence` recipe means a run with a decision started earlier takes a new
`rel`. A predecessor without a decision counts as having no summary for
`changes.jsonl`; one with a decision but no `identity.jsonl` fails the stage.

## Downstream use

`identity.jsonl` and `changes.jsonl` are the downstream products' input for
selecting what to import and re-checking their own mappings. An effect has no
date. The acting document's `eff_from` describes that document only; a provision
may commence later, so no date of effect is to be inferred from it.
