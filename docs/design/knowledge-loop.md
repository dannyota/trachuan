# Findings and reviewed knowledge

Decision: [ADR 0017](../adr/0017-findings-and-reviewed-knowledge.md). Evidence:
[write-path survey](../research/write-path-findings-2026-11.md). This note owns
detectors, findings, the contract for reviewed knowledge, intake and the
evidence each release carries. It covers the law corpus. Each existing check and
input keeps its own note.

## Detectors

A detector is a named check with a version, `unmatched_operative_claim/1`. The
Go registry is the only list: `corpus detectors list` prints it, and a test
fails when code writes a gap kind or a finding with no registered detector. A
detector declares its subject type, the inputs it reads, its severity
(`blocking`, `review`, `info`), the values hashed into a finding's fingerprint,
the discriminator that tells two findings on one subject apart, and what a
complete observation of a subject needs. A version that changes what a detector
means sets a new semantics number; a bug fix keeps it.

Existing gap kinds are registered as detectors of the stage that writes them and
keep their names, rules and acceptance by kind. A gap kind moves to findings
only by a named migration that turns its accepted gaps into review decisions.

Cross-signal detectors do not run inside the stages. They run once, in the
`evidence` stage after `eval`, over frozen inputs only: the build artifact, the
run's final gaps, the evaluation report, the reviewed knowledge in the build
input, and the previous release's evidence. The same inputs give the same
findings; a rewind of earlier stages cannot leave a stale one. A detector that
fails or lacks an input fails the stage with `detector_incomplete`; it never
reports zero findings.

### First cross-signal detectors

| Detector                            | A finding per      | Fires when                                                                                                                                                                                       | Severity |
| ----------------------------------- | ------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | -------- |
| `unmatched_operative_claim`         | operative sentence | a sentence matching the relation grammar names a target, in any document with text, including those excluded from text relations, and no relation row has that source provision, target and kind | review   |
| `partial_without_provision_effect`  | document           | status class `partial` and no relation targets any of its provisions; coarse, never proof that a document is explained                                                                           | info     |
| `raw_field_not_normalized`          | document and field | a stored `number` or `title` differs from its trimmed NFC form                                                                                                                                   | info     |
| `ocr_label_item_outside_labels`     | OCR check item     | an item of kind `document_number` or `article_label` lies in an annex or tail scope                                                                                                              | review   |
| `waived_document_without_relations` | document           | a document with an OCR waiver yields no text relation                                                                                                                                            | info     |
| `tree_transcription_divergence`     | document           | a kept source tree and its transcription differ beyond the heading-only rule                                                                                                                     | review   |
| `relation_kind_disagreement`        | relation pair      | a text relation and the source diagram name different kinds for one pair                                                                                                                         | review   |
| `external_report`                   | intake record      | an intake record is present ([Intake](#intake))                                                                                                                                                  | review   |

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
of that run's frozen inputs and the previous release's `findings.json`:

- **Occurrence.** Each finding carries `first_rel`: inherited from the previous
  release when the key is there, else this release. A finding that disappears
  and returns starts a new occurrence.
- **Coverage.** For every key in the previous release that did not fire now, the
  evidence says `gone` when the subject was observed completely, or `carried`
  when it was not (document absent, served from a baseline, a gap on an input).
  A carried finding keeps its previous fingerprint and occurrence.
- **Status.** `accepted` when a review decision in this run's build input names
  the same key, fingerprint, semantics number and `first_rel`; otherwise `open`.
  So a changed fingerprint, a new semantics number or a recurrence ends an
  acceptance, and a carried finding is judged like a fired one.

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

- Every dependency is recomputed from the frozen run. If one differs or cannot
  be computed (document absent, served from a baseline, a gap on the provision),
  the entry is **stale**: not applied, gap `knowledge_stale`. A stale decision
  accepts nothing, so its finding is open.
- A quote is an exact substring of the anchored provision's own text; no
  white-space folding.
- A malformed entry or a duplicate `id` fails the build.
- An entry that changes nothing is reported as redundant.
- A kind needs a corpus schema version that declares it. The validator checks
  the run's target schema version, a frozen input. That a live server image can
  read the version, and the release floor, are checked at publication by
  [corpus release](corpus-release.md#schema-version-changes), not here.

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

It needs method `reviewed` in the corpus schema (a version change), the server
admitting that method in warnings (`internal/retrieval`), an evaluation case,
and the order of [corpus release](corpus-release.md#schema-version-changes).

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

| Field                 | Values                                                                                                            |
| --------------------- | ----------------------------------------------------------------------------------------------------------------- |
| `origin`, `origin_id` | a registered origin (`issue_report`, `sc365`, `eval`) and its id; the pair is unique, so a replay changes nothing |
| `kind`                | the issue-report kinds of [error reports](error-reports.md)                                                       |
| `target`              | document or provision identity, or a normalized number for a missing document                                     |
| `claim`               | a code from a fixed list (`repealed_by`, `amended_by`, `wrong_number`, ...) with an optional acting identity      |

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
  previous release's `rel` and the SHA-256 of its `findings.json`, or `null` for
  the first); per detector `complete: true` and the number of subjects observed
  and not observable; every finding with key, fingerprint, semantics number,
  `first_rel`, status and `carried`; and the keys that are new, gone and
  carried.
- **`changes.json`**: the same bindings, and before and after records by public
  identity, computed from the previous release's database and this one:
  - documents added and removed, each with a cause class: `membership`,
    `coverage` (a fetch gap, a baseline, a withheld text) or `source`;
  - provisions added, removed and changed (text hash before and after);
  - legal changes: status class, relations and effects added and removed;
  - provenance changes: text source, completeness, served from a baseline.

  It reports no "moved" provision: without a verified correspondence between two
  identities, a move is one removal and one addition for the reader to review.

A second evaluation of the same build (another previous report) yields a new
evidence artifact; the upload names the one it uses. The release upload gains
the two files beside the dump, `MANIFEST.sha256`, `report.json` and
`release.json`, and the signed corpus manifest carries their SHA-256
([corpus release](corpus-release.md)).

### Gate

| Condition                                                                                                                    | Result                            |
| ---------------------------------------------------------------------------------------------------------------------------- | --------------------------------- |
| every existing rule of corpus release: items complete, every gap accepted, evaluation gates                                  | unchanged                         |
| a sidecar missing, or a binding that does not match the dump, the build artifact, the report or the report's `previous_diff` | refuse                            |
| a detector not complete                                                                                                      | refuse                            |
| an `open` finding of a `blocking` detector, fired or carried                                                                 | refuse                            |
| `open` findings of `review` and `info` detectors                                                                             | allowed, counted                  |
| first release with evidence                                                                                                  | allowed with `previous` as `null` |

`changes.json` is the downstream products' input for re-checking their own
mappings.

## What stays out

- No model at build time, and no automatic correction.
- No finding, record or log quotes more than 200 characters of text.
