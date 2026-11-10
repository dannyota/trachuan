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

Additions to version 2, exact DDL in the corpus schema:

- `document.class text NOT NULL CHECK (class IN ('primary','relation_context','secondary'))`,
  copied from final membership; no default.
- `document.source text NOT NULL` (`vbpl`, `chinhphu`), from the document key.
- `document.status_basis` also accepts `derived_from_text` and
  `manual_correction`; `document.validity_conflict boolean NOT NULL`.
- `validity_decision(document_id, ordinal, layer, rule_version, before jsonb, after jsonb, actor_key, effect_date, relation_id, anchor, quote, artifact_sha256)`,
  at most 8 rows per document, per the validity note.
- `source_file.source`, `source_file.url`, `source_file.role` filled for
  secondary documents from the version 2 fetch bundle.

A legacy run (selection version 0) builds at version 3 with class `primary` for
every document. Reason: one reader, one schema; the class of a legacy run is
known by construction.

## Inventory version 2

For a run with selection version 1 the upstream inventory is
`{version:2,country,source_list_hash,final_membership_hash,expected_keys,items,gaps}`:

- `final_membership_hash` is the frozen `final` phase hash from `run_scope`. The
  canonical snapshot already binds every member, class, disposition, replacement
  key and support tuple, so the inventory does not repeat them.
- `expected_keys` are the final active members plus retained removals, byte
  sorted. Replaced members are not expected keys.
- `items` holds every predecessor through `embed` in frozen stage order,
  including `expand`, then byte-sorted key. `gaps` adds `source`, `origin`,
  `cause` and `evidence_ref` to the existing tuple.

`source_manifest_hash` and `identity_inventory_hash` are computed as today over
these fields; the identity form keeps discover, expand, fetch, extract and
relate items. The build input becomes
`stage.RecipeHash("build-input","3",country,rel,source_manifest_hash,decimal_schema_version,identity_map_sha256)`.
A selection version 0 run keeps the version 1 inventory shape with no `version`
field, so its hashes stay comparable with earlier runs.

Build planning for selection version 1 requires the committed `final` phase, no
durable run failure and no paused source; otherwise it fails with a clear
non-retryable error. It never rebuilds membership from fetch items.

## Loading

- One `document` row per final active member with a done fetch item; class and
  source from membership. A member with disposition `source_replaced`,
  `withdrawn` or `scope_removed`, and a member whose fetch item is a gap, loads
  no document; its key and gaps go to `coverage_gap`.
- A secondary document loads metadata, source files and its `no_tree` gap; it
  has no provisions, chunks or relations until text extraction exists.
- A fetch artifact whose key is not a final active member is ignored, even when
  done (a downloaded secondary later replaced at hand-over).
- Validity rules and manual corrections run after documents and relations load,
  per the validity note; `validity-review.json` is written beside `build.json`.

## Readers and evaluation

- `corpusdb.SchemaVersion = 3`; retrieval `Open` rejects other versions.
- Retrieval returns `class` and `source` on every hit and does not rank by them
  in recipe 1. Eligibility adds "not `validity_conflict`" under the bumped
  validity policy.
- The identity map stays version 2: entries bind to per-document identity
  records, not to an inventory hash, so documents that enter the corpus need
  re-adjudication of the affected `absent` entries only, under the existing
  rule. The four documents the baseline lacked are re-adjudicated after the
  scope run.
- The evaluation report adds document counts by class and source, hits by class,
  and the validity counts from the validity note.

## Tests

`TestInventory_V2BindsFinalMembershipHash`,
`TestInventory_LegacyShapeUnchanged`, `TestBuild_RequiresFinalMembership`,
`TestLoad_ClassAndSourceFromMembership`, `TestLoad_ReplacedMemberNotLoaded`,
`TestLoad_IgnoresFetchOutsideMembership`, `TestLoad_SecondaryMetadataOnly`,
`TestOpen_RejectsSchemaVersion2`, `TestSchema_Version3Constraints`, and a proof
that builds the scope proof run through `build` and `eval`.
