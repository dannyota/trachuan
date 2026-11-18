# Knowledge entries

The contract is in [findings and reviewed knowledge](knowledge-loop.md). This
note owns the file format, how a dependency is computed, and what the validator
reports.

## File

`sources/<cc>-knowledge.yaml` is a YAML list. An empty file or `[]` means no
entries. Unknown fields are refused. Build pins the file's bytes like the
corrections file. When the file has entries, their SHA-256 enters the build
input hash and the bytes are written to the build artifact as `knowledge.yaml`;
with no entries the input hash and the artifact are unchanged, so adding the
feature invalidates nothing. `evidence` reads `knowledge.yaml` from the build
artifact and treats its absence as no entries.

Every hash is `sha256:` and 64 lower-case hexadecimal characters
([conventions](release-evidence.md#conventions)).

## Fields

Every entry has:

| Field      | Value                                                                                                                                                                                                   |
| ---------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `id`       | `[a-z0-9][a-z0-9-]{2,63}`, unique in the file                                                                                                                                                           |
| `kind`     | `review_decision`, `provision_effect` or `relation`                                                                                                                                                     |
| `reviewed` | `date`, `by` (a role, never a person's name), `method` (`human` or `model-adjudicated`), and for `model-adjudicated` `sample_read` and `sample_of`, the entries a person read and the size of the batch |

`target`, `claim`, `evidence` and `depends` by kind:

```yaml
- id: quyet-dinh-mau
  kind: review_decision
  target: { finding: "sha256:..." }
  claim: { reason_code: false_positive, reason: "At most 200 characters." }
  depends: { fingerprint: "sha256:...", semantics: 1, first_rel: 20261118_1 }
  reviewed: { date: 2026-11-18, by: primary, method: human }
- id: tt-39-bai-bo-dieu-5
  kind: provision_effect
  target: { key: vn:vbpl:1, anchor: dieu-5 }
  claim:
    { acting: { key: vn:vbpl:2, anchor: dieu-30/khoan-2 }, effect: repeals }
  evidence: { quote: "exact text of the acting provision" }
  depends: { contract: 1, acting: <provision>, target: <provision> }
  reviewed: { date: 2026-11-18, by: primary, method: human }
- id: nd-15-thay-the-nd-14
  kind: relation
  target: { key: vn:vbpl:1 }
  claim: { acting_key: vn:vbpl:2, relation: replaces }
  evidence: { anchor: dieu-40, quote: "exact text of that provision" }
  depends:
    contract: 1
    provision: <provision>
    target_number_norm: "14/2020/nd-cp"
  reviewed: { date: 2026-11-18, by: primary, method: human }
```

- `reason_code` is `false_positive`, `source_defect`, `out_of_scope` or
  `deferred`. `first_rel` is the release the occurrence began in.
- `effect` is `repeals`, `amends_supplements`, `replaces` or `suspends`.
- `relation` is any kind of the relation table except `unknown`.
- `quote` has at most 200 characters.

## Provision dependency

`<provision>` is computed from the provision as the run stored it:

```yaml
{
  number_norm: "...",
  text_source: "...",
  text_sha256: "sha256:...",
  ancestors: [{ anchor: "dieu-30", heading_sha256: "sha256:..." }],
}
```

`text_sha256` is over the UTF-8 bytes of the `text` column, as in
[release evidence](release-evidence.md#conventions). `ancestors` runs from the
top of the document to the parent, with each ancestor's `heading` column hashed
the same way. The document key and anchor come from the entry. `contract` is 1
until a change to this computation, which makes every older entry stale.
`corpus knowledge depends <cc> <rel> <key> <anchor>` prints the block for an
author; nobody writes one by hand.

## Validator

`corpus knowledge check <cc> [<rel>]` always checks structure: the schema,
unique ids, field bounds, refused `~k` anchors and URL evidence. With `<rel>` it
also reads that release's database read-only and checks every effect and
relation. The CI test runs the structural check over `sources/`. Build runs the
full check over the rows it is building.

| Outcome                                                                                  | Result                                                            |
| ---------------------------------------------------------------------------------------- | ----------------------------------------------------------------- |
| malformed entry, duplicate `id`, `~k` anchor, or URL evidence                            | error; build fails                                                |
| two entries or an entry and a row with the same acting and target provision, other kinds | error; build fails                                                |
| a dependency differs or cannot be computed, or the quote is not an exact substring       | stale: not applied, gap `knowledge_stale` with the `id` as detail |
| the row already exists                                                                   | redundant: reported, nothing written                              |
| `review_decision`                                                                        | structure only; applied in `evidence`                             |

A quote that stops matching is stale, not malformed, because the text it quotes
belongs to the run. `knowledge_stale` is a registered gap kind, accepted by kind
like every other.
