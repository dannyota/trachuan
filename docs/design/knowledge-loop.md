# Findings and reviewed knowledge

Decision: [ADR 0017](../adr/0017-findings-and-reviewed-knowledge.md). Evidence:
[write-path survey](../research/write-path-findings-2026-11.md). This note owns
detectors, findings, the contract for reviewed knowledge, intake and the
evidence each release carries. It covers the law corpus. Each existing check and
input keeps its own note.

## Detectors

A detector is a named check with a version, `unmatched_operative_claim/1`. The
Go registry is the only list: `corpus detectors list` prints it, stage gap
validation rejects a kind that is not registered, and a test checks every kind
constant against the registry. A detector declares its subject type, the inputs
it reads, its severity (`blocking`, `review`, `info`), the values hashed into a
finding's fingerprint, the discriminator that tells two findings on one subject
apart, and what a complete observation of a subject needs. A version that
changes what a detector means sets a new semantics number; a bug fix keeps it.

Existing gap kinds are registered as detectors of the stage that writes them and
keep their names, rules and acceptance by kind. A gap kind moves to findings
only by a named migration that turns its accepted gaps into review decisions.

Cross-signal detectors do not run inside the stages. They run once, in the
`evidence` stage after `eval`, over frozen inputs only. The same inputs give the
same findings; a rewind of earlier stages cannot leave a stale one. A detector
that fails or lacks an input fails the stage with `detector_incomplete`; it
never reports zero findings.

`evidence` is a stage with one item per run ([pipeline](pipeline.md)):

- **Recipe**, frozen when the run first includes the stage: the registry
  (detector versions and semantics numbers), detector configuration and the
  serialization rules of the files it writes. A changed recipe needs a new run,
  as for every stage.
- **Input hash**: the build artifact, the selected evaluation report with that
  report's gap snapshot, the reviewed knowledge bytes of the build input, and
  the predecessor's evidence ([Findings](#findings)).
- It commits under the run's fence like every stage, and the run becomes
  `evaluated` only when it is done.

### First cross-signal detectors

| Detector                            | A finding per      | Fires when                                                                                                                                                                                     | Severity                                                                    |
| ----------------------------------- | ------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------- |
| `unmatched_operative_claim`         | operative sentence | a sentence matching the relation grammar names a target that resolves to a provision in the release, in any document with text, and no relation row has that source provision, target and kind | review for a source that text relations read; info for an incomplete source |
| `partial_without_provision_effect`  | document           | status class `partial` and no relation targets any of its provisions; coarse, never proof that a document is explained                                                                         | info                                                                        |
| `raw_field_not_normalized`          | document and field | a stored `number` or `title` differs from its trimmed NFC form                                                                                                                                 | info                                                                        |
| `ocr_label_item_outside_labels`     | OCR check item     | an item of kind `document_number` or `article_label` lies in an annex or tail scope                                                                                                            | review                                                                      |
| `waived_document_without_relations` | document           | a document with an OCR waiver yields no text relation                                                                                                                                          | info                                                                        |
| `relation_kind_disagreement`        | relation pair      | a text relation and the source diagram name different kinds for one pair                                                                                                                       | review                                                                      |
| `external_report`                   | intake record      | an intake record is present ([Intake](#intake))                                                                                                                                                | review                                                                      |

`unmatched_operative_claim` applies the grammar of
[relations from text](relations-text.md) read-only; it writes no relation. No
cross-signal detector is `blocking` at first. A detector on counts is not
defined: the evidence carries counts per detector and the uploader prints the
change from the previous release.

## Findings

A subject is typed: `document` (key), `provision` (key, anchor), `relation`
(source, target, kind), `claim` (key, anchor, sentence ordinal), `check_item`
(key, item kind, page, ordinal), `report` (origin, origin id), `absent_document`
(normalized number). A finding's **key** is the hash of detector id, subject
type, subject fields and the detector's discriminator, so two defects from one
detector on one document are two findings. Its **fingerprint** hashes the inputs
that fired it.

A finding has no stored state. `findings.json` of a release is a pure function
of the evidence item's frozen inputs:

- **Predecessor.** The release named by the selected evaluation report's
  `previous_diff`, frozen in the evidence item; it is separate from the run's
  ingest baseline. When that release predates evidence, its identity is recorded
  and its findings are the empty set. When no release exists, there is no
  predecessor. A release that has evidence whose artifact is missing or corrupt
  fails the stage.
- **Occurrence.** Each finding carries `first_rel`. It is inherited only from a
  finding that fired or was carried in the predecessor with the same fingerprint
  and semantics number; otherwise it is this release. So a finding that changes
  and changes back, or disappears and returns, is a new occurrence.
- **Coverage.** For every key in the previous release that did not fire now, the
  evidence says `gone` when the subject was observed completely, or `carried`
  when it was not (document absent, served from a baseline, a gap on an input).
  A carried finding keeps its previous fingerprint and occurrence. A `gone` key
  is a record only: it is not a finding, and nothing accepts or inherits from
  it.
- **Status.** `accepted` when a review decision in this run's build input names
  the same key, fingerprint, semantics number and `first_rel`; otherwise `open`.
  So a changed fingerprint, a new semantics number or a new occurrence ends an
  acceptance, and a carried finding is judged like a fired one. Whether a
  decision applies is decided only here, against the findings computed in this
  stage. A decision that matches no finding is listed as `unmatched`; it writes
  no gap and changes no earlier input.

`corpus findings list|show <cc> <rel>` reads a release's evidence artifact. It
never reads the current Git tree or other runs.

## Reviewed knowledge

### The contract

A reviewed entry of a new kind carries `id` (unique in its file), `kind`,
`target`, `claim`, `evidence`, `depends` and `reviewed` (date, reviewer, and
`human` or `model-adjudicated` with the sample read). Target, evidence and
dependencies are defined per kind:

| Kind               | Target      | Claim                                                                                   | Evidence                               | Depends on                                                       |
| ------------------ | ----------- | --------------------------------------------------------------------------------------- | -------------------------------------- | ---------------------------------------------------------------- |
| `review_decision`  | a finding   | reason code and a reason of at most 200 characters                                      | none                                   | the finding's key, fingerprint, semantics number and `first_rel` |
| `provision_effect` | provision T | acting provision S and a kind (`repeals`, `amends_supplements`, `replaces`, `suspends`) | a verbatim quote from S                | S and T as provisions (below)                                    |
| `relation`         | document B  | acting document A and a relation kind                                                   | a verbatim quote from a provision of A | that provision; `key` and `number_norm` of A and B               |

A provision dependency is: the provision's own text hash, the anchors and
heading hashes of its ancestors, its document's key, `number_norm` and text
source, and the contract version. An anchor with a `~k` component, or with a
`~k` sibling at any level, is refused as target or evidence, as build refuses it
for relation targets. Evidence by URL is not accepted for the new kinds: no
artifact in the run can verify it. A decision on an aggregate or a missing
document needs no quote, because `review_decision` has none.

One validator, used by `corpus knowledge check`, by build and by a CI test over
`sources/`, validates all entries together:

- A malformed entry or a duplicate `id` fails the build.
- For `provision_effect` and `relation`, every dependency is recomputed from the
  frozen run. If one differs or cannot be computed (document absent, served from
  a baseline, a gap on the provision), the entry is **stale**: not applied, gap
  `knowledge_stale`.
- A quote is an exact substring of the anchored provision's own text; no
  white-space folding.
- An entry that changes nothing is reported as redundant.
- A `review_decision` is checked for structure only. Its bytes are frozen in the
  build input, and it is applied in `evidence` ([Findings](#findings)), never at
  build.

Existing inputs keep their files, rules and hashes
([validity status](validity-status.md), [OCR check](ocr-check.md),
[evaluation fixtures](evaluation-fixtures.md),
[pinned documents](pinned-documents.md), [taxonomy](taxonomy.md)). Reviewed
knowledge enters the build input hash, not a recipe, except inputs that act
before build (OCR waivers, as today). Text is never rewritten by knowledge.

### Provision effects

A `provision_effect` says that provision S repeals, amends, replaces or suspends
provision T, quoting S. It carries no date: relation rows have none and warnings
state none ([MCP results](mcp-results.md)). It is stored and served exactly as a
relation read from text with the same kind:

- **Stored** as a `relation` row from S to T with the kind, the quote as
  evidence and method `reviewed`. `provision_validity` stays empty and the
  document's status class is unchanged ([validity status](validity-status.md)).
- **Served** as the warning a text relation gives on T and on its document.
  Retrieval eligibility, validity stamps and historical reads do not change.
- **Identity** is (S, T, kind). An entry equal to an existing row is redundant.
  The same S and T with a different kind fails the build. Effects on an article
  and on one of its clauses are both kept, as for text relations. An effect on a
  document that is on `hold` or not eligible is applied and not served.

It needs no schema version: `relation.method` is free text. The server must
admit `reviewed` wherever it lists the methods that reach a provision
(`internal/retrieval`: `warnings.go`, `warning_sql.go`, `read_sql.go`,
`hydrate_tree.go`), with an evaluation case. An image without that change serves
such a row as a plain document-level relation and gives no warning on the
provision; nothing fails, but the warning is lost. So before a corpus release
that holds such rows is published, the first image that admits the method is
live and the release floor is confirmed at that image
([pull deployment](pull-deployment.md#release-floor)), so no rollback can reach
an older one.

### Control frameworks

Framework text is licensed material that must not be in Git
([framework access](frameworks-access.md#licensed-material-and-where-it-may-be)),
so this contract does not cover framework findings or decisions. A link from a
control to a legal provision may be reviewed knowledge here only when it names
the control by identity and quotes law text alone. Anything that needs framework
text waits for a private evidence store defined under ADR 0016.

## From finding to knowledge

1. A detector, an intake record or a person opens a finding.
2. Triage reads the evidence. A model may draft entries in bulk; every draft
   must pass the validator, and a model-adjudicated batch records the sample a
   person read
   ([validity status](validity-status.md#bounds-tests-and-release-gate)).
3. The entry merges through a pull request with the validator in CI.
4. The next run applies it: the finding is `accepted`, or gone when the entry
   changes the data so the detector no longer fires.

When a person or an outside report finds a defect that no detector opened, the
fix does not merge until a detector for its class exists, or the pull request
says in one line why the class cannot be seen in data.

## Intake

Outside signals enter as records in `sources/<cc>-reports.yaml`, in Git and in
the build input hash. A record has a closed schema and no free text:

| Field                 | Values                                                                                                                                                                                                                                                                                                            |
| --------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `origin`, `origin_id` | a registered origin (`issue_report`, `sc365`, `eval`) and its id; the pair is unique, so a replay changes nothing                                                                                                                                                                                                 |
| `kind`                | the issue-report kinds of [error reports](error-reports.md)                                                                                                                                                                                                                                                       |
| `target`              | document or provision identity, or a normalized number for a missing document                                                                                                                                                                                                                                     |
| `claim`               | a code from a fixed list, with the acting identity where it has one: `repealed_by`, `amended_by`, `replaced_by`, `suspended_by`, `wrong_number`, `missing_document`, `ambiguous_anchor`, `empty_text`, `status_conflict`, `missing_source_url`, and `reported` for an accepted report with no more specific claim |

At most 500 records change per pull request. Records carry no account, question
or client data. A record never applies knowledge; it only opens a finding. The
export batches of error reports keep their guarantees (fixed membership,
accepted reports only, deduplication); `corpus findings import` converts a
batch's files into records for a pull request and writes nothing else.

## Release evidence

The `evidence` stage runs after `eval` and writes one immutable artifact that
binds, and never changes, the build artifact and the evaluation report:

- **`findings.json`**: schema version; bindings (`rel`, the dump's SHA-256, the
  build artifact hash, the report's SHA-256, the detector registry hash, the
  predecessor's `rel` and the SHA-256 of its `findings.json`, or `none` when it
  predates evidence); per detector `complete: true` and the number of subjects
  observed and not observable; every finding with key, fingerprint, semantics
  number, `first_rel`, status and `carried`; and the keys that are new, gone and
  carried.
- **`identity.jsonl`**: this release's identity summary, one JSON object per
  line. The first line holds the schema version and the bindings. Then:
  - per document: key, `number_norm`, status class, `eff_from`, `eff_to`,
    `is_current`, `validity_conflict`, text source, completeness and how it was
    obtained;
  - per provision: document key, anchor, the SHA-256 of its own `text` column
    exactly as stored (UTF-8 bytes, no normalization, not its subtree), and the
    codes of its effective positive taxonomy nodes, resolved by the serving
    rules of [taxonomy](taxonomy.md);
  - every relation by its two ends, kind and method.

  A reader of the dump can recompute every hash and get the same value.

- **`changes.jsonl`**: one JSON object per line; the first line holds the schema
  version and the bindings. Records are before and after pairs by public
  identity, from this summary and the predecessor's:
  - documents added and removed, each with a cause class: `membership`,
    `coverage` (a fetch gap, a baseline, a withheld text) or `source`. A removed
    document's provisions are removed with it, under its cause class, and are
    not listed one by one;
  - document metadata: `number_norm` before and after on a surviving key;
  - provisions added, removed and changed (text hash before and after) in
    surviving documents. Each carries `provenance: true` when its document's
    text source or completeness changed in this release, so a new extraction is
    not read as a change of law;
  - legal changes: status class, `eff_from`, `eff_to`, `is_current`,
    `validity_conflict`, and relations and effects added and removed;
  - provenance changes: text source, completeness, served from a baseline;
  - classification changes: node codes added to and removed from a provision.

  When the predecessor has no summary (it predates evidence, or there is none),
  the file says `baseline` and lists nothing.

  It reports no "moved" provision: without a verified correspondence between two
  identities, a move is one removal and one addition for the reader to review.

An evaluation against another predecessor is a new evidence item and artifact.
Once a `rel` has an upload prefix it is never reused, so that case needs a new
`rel` ([corpus release](corpus-release.md)). The release upload gains
`findings.json`, `identity.jsonl` and `changes.jsonl` beside the dump,
`MANIFEST.sha256`, `report.json` and `release.json`, and the signed corpus
manifest carries the SHA-256 of all three.

### Gate

| Condition                                                                                                                    | Result                                                                                                                                       |
| ---------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------- |
| every existing rule of corpus release: items complete, every gap accepted, evaluation gates                                  | unchanged                                                                                                                                    |
| a sidecar missing, or a binding that does not match the dump, the build artifact, the report or the report's `previous_diff` | refuse                                                                                                                                       |
| a detector not complete                                                                                                      | refuse                                                                                                                                       |
| an `open` finding of a `blocking` detector, fired or carried                                                                 | refuse                                                                                                                                       |
| `open` findings of `review` and `info` detectors                                                                             | allowed, counted                                                                                                                             |
| first release with evidence                                                                                                  | allowed: the predecessor's identity is recorded, its findings are empty and `changes.jsonl` is `baseline`; every existing rule still applies |

`identity.jsonl` and `changes.jsonl` are the downstream products' input for
selecting what to import and re-checking their own mappings. An effect has no
date. The acting document's `eff_from` describes that document only; a provision
may commence later, so no date of effect is to be inferred from it.

## What stays out

- No model at build time, and no automatic correction.
- No finding, record or log quotes more than 200 characters of text.
