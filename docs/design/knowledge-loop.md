# Findings and reviewed knowledge

Decision: [ADR 0017](../adr/0017-findings-and-reviewed-knowledge.md). Evidence:
[write-path survey](../research/write-path-findings-2026-11.md). This note owns
detectors, findings, the contract for reviewed knowledge, intake and the
comparison between releases. Each existing check and input keeps its own note.

## Detectors

A detector is a named check with a version, `unmatched_operative_claim/1`. The
Go registry is the only list: `corpus detectors list` prints it, and a test
fails when a gap kind or finding is written by code with no registered detector.
A detector declares its subject type, the inputs it reads, its severity
(`blocking`, `review`, `info`), the values that make a finding's fingerprint,
the discriminator that tells two findings on one subject apart, and what a
complete observation of a subject needs.

The versions and tolerances of all detectors form the `detect` recipe, frozen
per run like every stage recipe ([pipeline](pipeline.md)). A version change that
alters what a detector means sets a new semantics number; a version that only
fixes a bug keeps it.

Existing gap kinds are detectors of the stage that writes them, with today's
names and rules. A gap kind moves to findings only by a named migration that
converts its accepted gaps; until then it is accepted by kind as now.
Cross-signal detectors run in `build` after relations and validity are loaded;
`eval` adds its diagnostics. All are deterministic and offline. A detector that
fails or lacks an input fails the run with `detector_incomplete`; it never
reports zero findings.

### First cross-signal detectors

| Detector                            | A finding per          | Fires when                                                                                                                                                                            | Severity |
| ----------------------------------- | ---------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------- |
| `unmatched_operative_claim`         | operative sentence     | a sentence matching the relation grammar names a target, in any document including those excluded from text relations, and no relation row has that source provision, target and kind | review   |
| `partial_without_provision_effect`  | document               | status class `partial` and no relation targets any of its provisions; coarse, never proof that a document is explained                                                                | info     |
| `raw_field_not_normalized`          | document and field     | a stored `number` or `title` differs from its trimmed NFC form                                                                                                                        | info     |
| `ocr_label_item_outside_labels`     | OCR check item         | an item of kind `document_number` or `article_label` lies in an annex or tail scope                                                                                                   | review   |
| `waived_document_without_relations` | document               | a document with an OCR waiver yields no text relation                                                                                                                                 | info     |
| `tree_transcription_divergence`     | document               | a kept source tree and its transcription differ beyond the heading-only rule                                                                                                          | review   |
| `relation_kind_disagreement`        | relation pair          | a text relation and the source diagram name different kinds for one pair                                                                                                              | review   |
| `count_drift`                       | detector or count name | an open count or a `documents_by_*` count moves beyond its tolerance in [budgets](budgets.md) against the previous release                                                            | review   |

`unmatched_operative_claim` runs the relation grammar of
[relations from text](relations-text.md) in read-only mode over every document
with text; it writes no relation. `count_drift` does not run on the first
release that has no previous `findings.json` (the report says `baseline`), and
never counts itself. No cross-signal detector is `blocking` at first.

## Findings

### Subjects and keys

A subject is typed: `document` (key), `provision` (key, anchor), `relation`
(source, target, kind), `claim` (key, anchor, sentence ordinal), `check_item`
(key, item kind, page, ordinal), `report` (origin, origin id), `absent_document`
(normalized number), `release` (count name). A finding's key is the hash of
detector id, subject type, subject fields and the detector's discriminator, so
two defects from one detector on one document are two findings.

### Observations and decisions

Two things are kept apart.

- **Observations** are immutable run evidence. Workshop table
  `finding_observation(run_id, key, detector, semantics, fingerprint, detail, observed)`:
  one row per finding that fired in the run, and one row with `observed = false`
  for a known open finding whose subject the run could not observe completely.
  Rows are written under the run's fence by the stage that ran the detector and
  never change. `detail` holds short structured values and at most 200
  characters of text.
- **Decisions** are reviewed knowledge in Git (kind `review_decision`, below),
  so they enter the build input hash.

A finding's state in a run is computed from the two, never stored by hand:

| State        | When                                                                                               |
| ------------ | -------------------------------------------------------------------------------------------------- |
| `open`       | it fired and no decision applies                                                                   |
| `accepted`   | it fired and a decision names this key with the same fingerprint and semantics number              |
| `resolved`   | the subject was observed completely and the detector did not fire                                  |
| `unobserved` | the subject could not be observed completely; the previous state stands and is reported as carried |

A resolved finding that fires again is `open`, whatever its fingerprint. A
changed fingerprint or semantics number ends an acceptance. A decision never
applies by subject alone.

`corpus findings list|show` reads a run's findings; decisions are made by adding
entries to Git, not by a command that edits state.

## Reviewed knowledge

### The contract

Every reviewed entry of the new kinds carries:

| Field      | Meaning                                                                     |
| ---------- | --------------------------------------------------------------------------- |
| `id`       | stable, unique in its file                                                  |
| `kind`     | the claim type                                                              |
| `target`   | public identity of a document, provision or finding                         |
| `claim`    | typed fields for the kind; no free text beyond a `reason` of 200 characters |
| `evidence` | document key, anchor and verbatim quote from a document in the run          |
| `depends`  | everything the review relied on, as hashes (below)                          |
| `reviewed` | date, reviewer, and `human` or `model-adjudicated` with the sample read     |

`depends` is complete per kind. For a provision target or evidence: the
provision's own text hash, the anchors and heading hashes of its ancestors, the
document's `number_norm` and text source, and the contract version. For a
finding: its key, fingerprint and semantics number. An anchor with a `~k`
component, or with a `~k` sibling at any level, is refused as target or
evidence, as build refuses it for relation targets. Evidence by URL is not
accepted for the new kinds, because no artifact in the run can verify it.

One validator, used by `corpus knowledge check`, by build and by a CI test over
`sources/`:

- Every dependency is recomputed from the frozen run. If any differs or cannot
  be computed (document absent, served from a baseline, gap on the target), the
  entry is **stale**: not applied, gap `knowledge_stale`, and the finding it
  answered is `open` again by the state rules.
- The quote is an exact substring of the anchored provision's own text; no
  white-space folding.
- A malformed entry or a duplicate `id` fails the build. Entries are validated
  together, never one by one (see [effects](#provision-effects)).
- An entry that changes nothing is reported as redundant.

Existing inputs keep their files, rules and hashes
([validity status](validity-status.md), [OCR check](ocr-check.md),
[evaluation fixtures](evaluation-fixtures.md),
[pinned documents](pinned-documents.md), [taxonomy](taxonomy.md)). Reviewed
knowledge enters the build input hash, not a recipe, except inputs that act
before build (OCR waivers, as today).

### New kinds

- **`review_decision`**: a finding was reviewed and nothing changes. Target: the
  finding (key, fingerprint, semantics number). Claim: a reason code and a short
  reason.
- **`provision_effect`**: see below.
- **`relation`**: a reviewed document-level relation that neither the diagram
  nor the text rules yield, with the same fields and meaning as a diagram
  relation.

Text is never rewritten by knowledge. A wrong text is a gap or a held document.

### Provision effects

A `provision_effect` says: provision S repeals, amends, replaces or suspends
provision T from a date, quoting S. It is stored and served exactly as a
relation from text with the same kind would be:

- **Stored** as a `relation` row from S to T with the kind, the date, the quote
  as evidence and method `reviewed`. `provision_validity` stays empty and the
  document's status class is unchanged ([validity status](validity-status.md)).
- **Served** as the warning a text relation gives on T and on its document
  ([MCP results](mcp-results.md)): the acting provision, kind and date.
  Retrieval eligibility, validity stamps and historical reads do not change.
- **Combined rules.** The same source, target and kind as an existing row is
  redundant. The same source and target with a different kind fails the build.
  Effects on an article and on one of its clauses are both kept, and several
  effects on one provision at different dates are kept in date order, as for
  text relations. An effect whose document is on `hold` or not eligible is
  applied and not served.

It needs: method `reviewed` in the corpus schema (a version change), the server
admitting that method in warnings (`internal/retrieval`), an evaluation case,
and the deployment order of
[corpus release](corpus-release.md#schema-version-changes): image first, floor,
then the first release that holds such rows. Until the image is live the
validator refuses the kind.

## From finding to knowledge

1. A detector, an intake record or a person opens a finding.
2. Triage reads the evidence. A model may draft entries in bulk; every draft
   must pass the validator, and a model-adjudicated batch records the sample a
   person read
   ([validity status](validity-status.md#bounds-tests-and-release-gate)).
3. The entry merges through a pull request with the validator in CI.
4. The next run applies it; the finding becomes `accepted`, or `resolved` when
   the entry changes the data so the detector no longer fires.

When a person or an outside report finds a defect that no detector opened, the
fix does not merge until a detector for its class exists, or the pull request
says in one line why the class cannot be seen in data.

## Intake

Outside signals enter as records in `sources/<cc>-reports.yaml`, in Git and in
the build input hash; the detector `external_report` opens one finding per
record. A record has a closed schema and no free text:

| Field                 | Values                                                                                                             |
| --------------------- | ------------------------------------------------------------------------------------------------------------------ |
| `origin`, `origin_id` | a registered origin (`issue_report`, `sc365`, `eval`) and its id; the pair is unique, so a replay changes nothing  |
| `kind`                | the issue-report kinds of [error reports](error-reports.md)                                                        |
| `target`              | document or provision identity, or a normalized number for a missing document                                      |
| `claim`               | a code from a fixed list (`repealed_by`, `amended_by`, `wrong_number`, ...) with optional acting identity and date |

At most 500 records per file change. Records carry no account, question or
client data. An intake record never applies knowledge; it only opens a finding.
The export batches of error reports keep their guarantees (fixed membership,
accepted reports only, deduplication); `corpus findings import` converts a
batch's files into records for a pull request and writes nothing else.

## Release evidence

`eval`, the last stage, finalizes two sidecars from the run's observations and
the two release databases:

- **`findings.json`**: schema version, `rel`, the dump's SHA-256, the `detect`
  recipe hash, the previous release (`rel` and its report SHA-256), every
  detector with `complete: true`, counts by state, and the keys that are new,
  reopened, resolved or carried unobserved.
- **`changes.json`**: schema version, the same bindings, and before and after
  records by public identity, computed from the previous release's database and
  this one:
  - documents added and removed, each with a cause class: `membership`,
    `coverage` (a fetch gap, a baseline, a withheld text) or `source`;
  - provisions added, removed, and changed (text hash before and after);
  - legal changes: status class, relations and effects added and removed;
  - provenance changes: text source, completeness, served from a baseline.

  It reports no "moved" provision: without a verified correspondence between two
  identities, a move is one removal and one addition for the reader to review.

Both files are in the build artifact manifest and are uploaded. The signed
corpus manifest carries their SHA-256. The uploader and the signing validator
refuse a release when a sidecar is missing, its bindings do not match the dump,
the report and the report's `previous_diff`, or any detector is not complete.

### Gate

| Condition                                                                                                        | Result                                                           |
| ---------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------- |
| every existing rule of [corpus release](corpus-release.md): items complete, every gap accepted, evaluation gates | unchanged                                                        |
| a detector did not complete                                                                                      | refuse                                                           |
| an `open` finding of a `blocking` detector                                                                       | refuse                                                           |
| `open` findings of `review` detectors                                                                            | allowed; counted, and `count_drift` watches them                 |
| first release with sidecars                                                                                      | allowed with `previous` bindings and `count_drift` as `baseline` |

`changes.json` is the downstream products' input for re-checking their own
mappings; for control frameworks the
[reduced export](frameworks.md#downstream-products) carries its own.

## What stays out

- No model at build time, and no automatic correction.
- No finding detail, record or log quotes more than 200 characters of text, and
  none quotes licensed framework text at all.
