# Knowledge entries

The contract is in [findings and reviewed knowledge](knowledge-loop.md). This
note owns the file format, how a dependency is computed, how an entry is stored
and what the validator reports.

## File

`sources/<cc>-knowledge.yaml` must exist and is one YAML list; an empty file, or
one holding only comments, means no entries. Unknown fields, anchors, aliases,
merge keys and further documents are refused. Entries are in `id` order. The
file is at most 8 MiB and 20,000 entries, and a pull request changes at
most 500.

With at least one entry, the build input hash gains the rules version
`knowledge/1` and the SHA-256 of the file's bytes, read once per build, and the
artifact gains those bytes as `knowledge.yaml` and the outcomes as
`knowledge-report.json` (one record per `id`: applied, redundant, or stale with
its reason). With no entries the input hash and the artifact are unchanged.
`evidence` reads `knowledge.yaml` from the build artifact and treats its absence
as no entries. No stage recipe changes.

Hashes in entries are `sha256:` and 64 lower-case hexadecimal characters
([conventions](release-evidence.md#conventions)). Lengths count Unicode code
points.

## Fields

Each kind has exactly these fields:

```yaml
- id: "decision-mau"
  kind: "review_decision"
  target: { finding: "sha256:..." }
  claim:
    { reason_code: "false_positive", reason: "Why, in 1 to 200 characters." }
  depends: { fingerprint: "sha256:...", semantics: 1, first_rel: "20261118_1" }
  reviewed: { date: "2026-11-18", by: "primary", method: "human" }
- id: "effect-mau"
  kind: "provision_effect"
  target: { key: "vn:vbpl:1", anchor: "dieu-5" }
  claim:
    acting: { key: "vn:vbpl:2", anchor: "dieu-30/khoan-2" }
    relation_kind: "repeals"
  evidence: { quote: "exact text of the acting provision" }
  depends: { contract: 1, acting: <provision>, target: <provision> }
  reviewed:
    date: "2026-11-18"
    by: "primary"
    method: "model-adjudicated"
    batch: "effects-2026-11"
    sample_read: 20
    sample_of: 200
- id: "relation-mau"
  kind: "relation"
  target: { key: "vn:vbpl:1" }
  claim: { acting: { key: "vn:vbpl:2" }, relation_kind: "replaces" }
  evidence: { anchor: "dieu-40", quote: "exact text of that provision" }
  depends:
    contract: 1
    provision: <provision>
    target_number_norm: "14/2020/nd-cp"
  reviewed: { date: "2026-11-18", by: "owner", method: "human" }
```

- `id` is `[a-z0-9][a-z0-9-]{2,63}`, unique. Keys start with `<cc>:`.
  `first_rel` has the `rel` form. An anchor follows
  [text structure](text-structure.md).
- `reason_code` is `false_positive`, `expected`, `source_defect`,
  `out_of_scope`, `deferred` or `resolved`. `reason` holds no name, account or
  user question. One (finding, fingerprint, semantics, `first_rel`) has one
  decision. A decision that matches no finding may be deleted: a returning
  finding is a new occurrence.
- For `provision_effect`, `relation_kind` is `repeals`, `amends_supplements`,
  `replaces` or `suspends`, and acting and target provisions differ. For
  `relation` it is any kind of the relation table except `unknown` and
  `consolidates`, and the two keys differ.
- `quote` has 10 to 200 characters. It is own text only, so where a list's verb
  sits in the parent, the review reads the parent and the validator does not.
  There is no URL field.
- `by` is `owner`, `coordinator` or `primary`, the role that accepted the entry.
- A `model-adjudicated` entry names its `batch` (a slug, like `id`) with
  `1 <= sample_read <= sample_of` and `sample_read * 10 >= sample_of`. Entries
  of one batch agree on both numbers and number at most `sample_of`.

## Provision dependency

`<provision>` holds, from the provision and its document as the run stored them:

```yaml
number_norm: "..."
text_source: "..."
text_sha256: "sha256:..."
heading_sha256: "sha256:..."
ancestors: [{ anchor: "dieu-30", heading_sha256: "sha256:..." }]
```

Hashes are over the UTF-8 bytes of the `text` and `heading` columns, unchanged.
`ancestors` runs from the top of the document to the parent. `contract` is 1. A
change to this computation raises it and ships with
`corpus knowledge rebind <cc> <rel>`, which rewrites `depends` only for entries
whose old dependency still verifies on that release.
`corpus knowledge depends <cc> <rel> <key> <anchor>` prints the block for an
author and refuses an ambiguous provision.

## Validator

`corpus knowledge check <cc> [<rel>]` always runs the **structural** checks; the
CI test runs them over `sources/`. With `<rel>` it reads that release's database
read-only and runs the **run** checks, as build does over the rows it is
building. Both read the same columns, and `knowledge_stale` rows never feed
them.

Structural errors fail the check and the build: anything refused above, a `~k`
component in any written anchor (ancestors included), two entries with one
identity, and two `provision_effect` entries with one acting and target
provision and different kinds. Identity is (acting provision, target provision,
kind) for an effect and (acting key, target key, kind) for a relation.

Run checks take each effect or relation in order, and the first that applies
decides:

| Case                                                                                   | Result                       |
| -------------------------------------------------------------------------------------- | ---------------------------- |
| a named document has no `document` row                                                 | stale `document_absent`      |
| its `build` gap `baseline_served` exists                                               | stale `baseline`             |
| its `text_complete` is false                                                           | stale `incomplete`           |
| a named anchor has no `provision` row                                                  | stale `anchor_absent`        |
| that provision or an ancestor has a `~k` component or sibling                          | stale `ambiguous`            |
| `contract`, `number_norm`, `text_source`, text, heading or ancestors differ            | stale, named after the field |
| every dependency equal and the quote is not an exact substring of the provision's text | error                        |
| a row of another method has the same identity                                          | redundant, nothing written   |
| an effect's acting provision and `target_node_id` match such a row of another kind     | error                        |
| otherwise                                                                              | applied                      |

A stale entry is not applied and writes gap `knowledge_stale`, a registered kind
accepted by kind, at stage `build` with the target document key as item and
`<id>:<reason>` as detail, so an acceptance does not outlive its cause. A
`review_decision` gets structural checks only; it is applied in `evidence`.

## Storage

Build inserts applied entries in `id` order after validity is computed, so they
change no status class, validity row or inheritance. An entry on a document that
is on `hold` or not eligible is applied and not served.

| `relation` column                       | `provision_effect`           | `relation`                    |
| --------------------------------------- | ---------------------------- | ----------------------------- |
| source document, key and number         | acting document              | acting document               |
| target document, key and number         | target document              | target document               |
| `kind`, `raw_label`                     | `relation_kind`              | `relation_kind`               |
| `raw_code`, `method`                    | `reviewed`                   | `reviewed`                    |
| `source_provision_id`, `source_node_id` | acting provision and anchor  | evidence provision and anchor |
| `target_provision_id`, `target_node_id` | target provision and anchor  | null                          |
| `evidence`                              | `quote`                      | `quote`                       |
| `artifact_sha256`                       | hexadecimal hash of the file | hexadecimal hash of the file  |
| `ordinal`                               | entry's position in the file | entry's position in the file  |
