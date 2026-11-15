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
- `identity.jsonl` and `changes.jsonl` hold one JSON object per line. The first
  line holds the schema version and the bindings.
- An anchor with no `~k` component is derived from the kinds and tokens on the
  provision's path ([text structure](text-structure.md)). A `~k` suffix numbers
  repeated anchors in tree order and is not stable across releases. A provision
  is **ambiguous** when any component of its anchor has a `~k` suffix or a `~k`
  sibling, the rule reviewed knowledge and build use
  ([relations from text](relations-text.md)). Nothing may be pinned to an
  ambiguous provision.

## `findings.json`

Schema version; bindings (`rel`, the dump's SHA-256, the build artifact hash,
the report's SHA-256, the detector registry hash, the predecessor's `rel` and
the SHA-256 of its `findings.json`, or `none` when it predates evidence); per
detector `complete: true` and the number of subjects observed and not
observable; every finding with key, fingerprint, semantics number, `first_rel`,
status and `carried`; and the keys that are new, gone and carried.

## `identity.jsonl`

This release's identity summary. Its bindings are `rel`, the dump's SHA-256, the
build artifact hash and the report's SHA-256. Then:

- per document: key, `number_norm`, status class, `eff_from`, `eff_to`,
  `is_current`, `validity_conflict`, text source, completeness and how it was
  obtained;
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
  (a fetch gap, a baseline, a withheld text) or `source`. A removed document's
  provisions are removed with it, under its cause class, and are not listed one
  by one;
- document key: the key before and after, only for a hand-over that fetch
  recorded as an accepted `source_replaced` naming both keys
  ([sources](sources.md#secondary-discovery-and-hand-over)), and only when the
  old key has a document line in the predecessor's summary and the new key has
  one in this summary. The document is then a surviving document, compared as
  one: metadata and classification as on a surviving key; provisions paired by
  equal anchor, each record carrying `provenance: true`; relation ends compared
  with the old key read as the new key, so a relation that differs only by the
  key is unchanged. When the new key has no document line, as after a failed
  fetch of the replacement, the old document is a removal with cause `source`
  that names the replacement key in `replaced_by`, and the replacement is an
  addition in the release that first holds it. Any other change of key is one
  removal and one addition;
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

When the predecessor has no summary (it predates evidence, or there is none),
the file says `baseline` and lists nothing.

It reports no "moved" provision: without a verified correspondence between two
identities, a move is one removal and one addition for the reader to review.

## Upload

An evaluation against another predecessor is a new evidence item and artifact.
Once a `rel` has an upload prefix it is never reused, so that case needs a new
`rel` ([corpus release](corpus-release.md)). The release upload gains
`findings.json`, `identity.jsonl` and `changes.jsonl` beside the dump,
`MANIFEST.sha256`, `report.json` and `release.json`, and the signed corpus
manifest carries the SHA-256 of all three.

## Gate

| Condition                                                                                                                    | Result                                                                                                                                       |
| ---------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------- |
| every existing rule of corpus release: items complete, every gap accepted, evaluation gates                                  | unchanged                                                                                                                                    |
| a sidecar missing, or a binding that does not match the dump, the build artifact, the report or the report's `previous_diff` | refuse                                                                                                                                       |
| a detector not complete                                                                                                      | refuse                                                                                                                                       |
| an `open` finding of a `blocking` detector, fired or carried                                                                 | refuse                                                                                                                                       |
| `open` findings of `review` and `info` detectors                                                                             | allowed, counted                                                                                                                             |
| first release with evidence                                                                                                  | allowed: the predecessor's identity is recorded, its findings are empty and `changes.jsonl` is `baseline`; every existing rule still applies |

## Downstream use

`identity.jsonl` and `changes.jsonl` are the downstream products' input for
selecting what to import and re-checking their own mappings. An effect has no
date. The acting document's `eff_from` describes that document only; a provision
may commence later, so no date of effect is to be inferred from it.
