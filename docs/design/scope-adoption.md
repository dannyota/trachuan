# Scope adoption in build and evaluation

This note owns how `build` and `eval` adopt run membership from
[sources](sources.md) and [membership](membership.md), and what corpus schema
version 3 contains. [Build](index.md) owns loading and recovery;
[corpus schema](corpus-schema.md) owns the exact template;
[validity corrections](validity-status.md) owns the validity rules that ship in
the same version.

## Why one version

Schema version 2 (`document.status_basis`) is merged and the first baseline was
built with it. Membership class and validity corrections both change the
`document` table and the build inputs, and neither can be backfilled into a
version 2 dump. They ship together as version 3 so a release is rebuilt once.
Version 2 dumps stay unchanged and need a version 2 reader; readers at version 3
refuse them with an error naming both versions.

## Schema version 3

Additions to version 2; the [corpus schema](corpus-schema.md) holds the exact
columns and constraints: `document.class`, `document.source`, two more
`status_basis` values, `document.validity_conflict`, the `validity_decision`
table, and `source_file` provenance filled for secondary documents from the
version 2 fetch bundle. A frozen run whose build recipe names schema version 2
is refused by the existing frozen-recipe guard, never rebuilt at version 3; a
run frozen only through ingest may append `build` under the new recipe.

## Inventory

Two versions, chosen by the frozen source list's selection version. Both are
compact JSON with keys in the order shown, arrays never null, absent values as
JSON `null`, and the existing ordering (stage order, then byte-sorted key; gaps
by stage, item, kind, detail).

Version 1 (selection version 0), byte-identical to today:

- source manifest: `{country,source_list_hash,expected_keys,items,gaps}`, item
  `{stage,item_key,state,input_hash,recipe_hash,artifact_sha256}`, gap
  `{stage,item_key,kind,detail}`;
- identity: `{country,source_list_hash,expected_keys,items}` with fetch, extract
  and relate items only;
- build input label `"2"`, with `corrections_sha256` appended only when the
  corrections list is not empty, so existing hashes stay unchanged.

Version 2 (selection version 1):

- source manifest:
  `{version:2,country,source_list_hash,final_membership_hash,expected_keys,items,gaps}`,
  same item tuple with `expand` in stage order after `discover`, gap
  `{stage,item_key,kind,detail,source,origin,cause,evidence_ref}`;
- identity:
  `{version:2,country,source_list_hash,final_membership_hash,expected_keys,items}`
  with discover, expand, fetch, extract and relate items;
- build input label `"3"`, with `provenance_sha256` then `corrections_sha256`
  appended as the last arguments.

`final_membership_hash` is the frozen `final` phase hash from `run_scope`. The
canonical snapshot binds every member, class, disposition, replacement key,
number and support tuple, so the inventory does not repeat them. `expected_keys`
are the final active members plus retained removals; replaced members are not
expected keys. The inventory query reads the snapshot bytes and the items in one
consistent read, verifies the bytes against the hash, and the loader uses only
that verified snapshot, never mutable membership rows. A source-gap retry clears
and republishes the final phase; the new hash changes the build input, so an
earlier build is not reused. Golden fixtures pin the bytes and both hashes of
each version.

Build planning for selection version 1 requires the committed `final` phase, no
durable run failure and no paused source, and reconciles membership with items:
every final active member has a fetch item in state `done` or `gap`, and every
present document ([presence](#presence)) has terminal extract and relate items.
A missing or non-terminal item fails planning with non-retryable
`membership_unreconciled` naming the keys. It never rebuilds membership from
fetch items.

## Provenance snapshot

Bundles carry no fetch time. For inventory version 2, planning reads, in the
same consistent read as the inventory, the workshop file projection of every
present document's fetch artifact and writes `provenance.json`:
`{version:1,files:[{document_key,source,name,sha256,size,path,url,role,fetched_at}]}`,
sorted by document key, then name, then hash; `fetched_at` is RFC 3339 UTC or
null. A file's row is written once at its first artifact-row commit and never
updated, so the bytes are stable for an unchanged inventory. Its SHA-256 is
`provenance_sha256` in the build input and the build record, not part of the
source manifest or identity hashes. `source_file.url`, `role` and `fetched_at`
load from this snapshot; a null time records `source_provenance_missing` as
today. A legacy run keeps its current provenance loading.

## Presence

A document is present in a run when its key is a final active member with a done
fetch item (selection version 0: an expected key with a done fetch that no
exclusion rule removes). Everything that asks "is this document in the corpus"
uses presence: build loading, number-to-document resolution for relations,
inventory export for adjudication, and identity-map validation.

A done fetch artifact that is not present (a secondary replaced at hand-over, a
member removed from scope) is retained identity evidence only. It is decoded
with the reader for its own source, it never resolves a number, never makes an
`absent` map entry look present, and never loads rows. Re-adjudication is
required when an `absent` expectation becomes present, when a resolved identity
stops being present, and when a resolved secondary key has disposition
`source_replaced` (the issue names the replacement key).

## Loading

- One shared source-aware reader decodes a fetch artifact by its key's source:
  VBPL `detail.json`, or the secondary `document.json` with its hash-linked
  files. Build loading, relation loading and inventory export all use it.
- One `document` row per present document; class and source from the verified
  snapshot. A legacy run assigns class `primary` and source `vbpl`.
- A final member that is not present loads no document; its key, disposition and
  gaps go to `coverage_gap`. Chunk and embed artifacts are verified and
  reconciled for every key in the inventory, as
  [build](index.md#inclusion-and-losses) requires, before exclusions; those of a
  key that is not present then load no rows. Artifacts of present keys are
  reused when their lineage and recipes match.
- A secondary document loads metadata, source files and its `no_tree` gap. It
  has no provisions, chunks or outgoing relations until text extraction exists;
  incoming relations from present documents are kept.
- Relations keep the [corpus schema](corpus-schema.md) rule unchanged: an edge
  whose source or target is not present keeps its key and number with no
  document row. Only present documents have relate items, so a replaced
  secondary contributes no edges.
- The build document cap applies after exclusions.
- Validity rules and manual corrections run after documents and relations load,
  per the validity note; `validity-review.json` is published with the build
  record.

## Readers and evaluation

- `corpusdb.SchemaVersion = 3`; retrieval `Open` rejects other versions with an
  error naming both.
- Retrieval returns `class` and `source` on every hit; the retrieval recipe does
  not rank by them, and admits `relation_context` documents only by number
  reference ([retrieval](retrieval-recipe.md#validity-and-evidence)).
  Eligibility adds "not `validity_conflict`" under the bumped validity policy.
  "Primary hits" in metrics means the primary response group, whatever the
  membership class.
- The identity map stays version 2: entries bind to per-document identity
  records, validated against present documents.
- `report.json` adds `documents_by_class`, `documents_by_source`,
  `hits_by_class` (primary response group, limit 8) and the validity counts of
  the validity note (`hits_by_status_basis`, `hits_by_conflict_kind`, same
  group).

## Tests

`TestInventory_V2GoldenBytesAndHashes`, `TestInventory_V1GoldenBytesUnchanged`,
`TestInventory_RepublishedFinalChangesBuildInput`,
`TestBuild_RequiresFinalMembership`, `TestBuild_MembershipUnreconciled`,
`TestBuild_FrozenSchema2RecipeRefused`, `TestLoad_ClassAndSourceFromSnapshot`,
`TestLoad_EachExcludedDispositionNotLoaded`,
`TestLoad_IgnoresArtifactsOfAbsentKeys`, `TestLoad_SecondaryMetadataOnly`,
`TestRelations_ReplacedSecondaryNeverResolvesNumber`,
`TestIdentity_ReplacedSecondaryNeedsReadjudication`,
`TestProvenance_SnapshotStableAndBound`, `TestOpen_RejectsSchemaVersion2`,
`TestSchema_Version3Constraints`, `TestDump_RestoresValidityDecisions`, and a
proof that takes the scope proof run through `build` and `eval`.
