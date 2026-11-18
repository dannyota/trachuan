# ADR 0017: Findings and reviewed knowledge on the write path

Status: accepted 2026-11-15.

## Context

In one week four corpus defects were found by chance, not by the pipeline: an
article repealed by a later circular was served as current (reported by the
sc365 importer), a decree's text was withheld by a false OCR flag (found by
reading the flag), nine documents store their number and title with leading
white space, and 196 documents are withheld by the same OCR flag kind with one
of them read.

The write path already records much
([survey](../research/write-path-findings-2026-11.md)): gaps from every stage, a
validity review file with 16 entry kinds, OCR check items, relation gaps,
evaluation diagnostics. It also takes reviewed inputs: 39 validity corrections,
one OCR waiver, 209 identity-map entries, pins, named numbers. But:

- Signals are not compared with each other. A document with source status
  "partly expired" counts as explained by any document-level relation, so a
  missing provision-level repeal is invisible.
- A finding has no identity. Gaps are accepted by kind, for one run; a new
  instance hides among accepted ones and "reviewed, no change" is recorded
  nowhere, so the same entries return every run.
- The validity review file and the exported report files have no reader.
- Nothing compares counts with the previous release.
- Reviewed knowledge stops at the document: there is no reviewed provision-level
  effect and no reviewed relation.

The owner asked on 2026-11-15 that the write path detect defects itself and
apply what we learn, and that the owner's other products (sc365) build on that
knowledge.

## Decision

1. **Detectors.** Every automatic check that records a problem is a named,
   versioned detector in one Go registry, the only list. Existing gap kinds are
   registered under their names. New detectors compare signals the pipeline
   already holds. Detectors are deterministic: no model, no network. A detector
   that cannot complete fails the run; it never means "nothing found".
2. **Findings have identity and no stored state.** A finding is a detector, a
   typed subject and a discriminator; its fingerprint hashes the inputs that
   fired it. The findings of a release are a pure function of that run's frozen
   inputs and the previous release's findings: each records when its occurrence
   began, whether it fired or was carried because its subject could not be
   observed, and whether a review decision accepts it. A decision binds to one
   finding, its fingerprint, the detector's semantics and the occurrence, so a
   changed or returning defect is open again.
3. **Reviewed knowledge has one contract.** Every new reviewed entry names its
   target by public identity, states a typed claim, and gives the evidence and
   the dependencies its kind requires: an effect or a relation quotes a document
   in the run and lists everything the review depended on as hashes; a review
   decision has no quote and binds to its finding. One validator checks entries
   before merge and at build, all together. An effect or relation whose
   dependencies cannot be verified against the frozen run is stale and not
   applied; a decision that matches no current finding accepts nothing.
4. **Knowledge reaches provisions.** A reviewed provision effect (one provision
   repeals, amends, replaces or suspends another) is stored and served exactly
   as a relation read from text with the same kind: a warning on the target, no
   date, no change of status class or eligibility. A reviewed document relation
   and a review decision complete the kinds. Text is never rewritten.
5. **A defect found by a person becomes a detector or a knowledge entry before
   its fix merges.** A detector when the class can be seen in data; otherwise an
   entry and one line on why not.
6. **One intake, closed and in Git.** Issue reports, reports from downstream
   products and evaluation diagnostics become records with a closed schema, no
   free text and no account data. A record opens a finding and applies nothing.
7. **Every release carries its evidence.** A last stage, after evaluation, runs
   the cross-signal detectors over frozen inputs and writes one immutable
   artifact with `findings.json`, `identity.jsonl` and `changes.jsonl`, bound to
   the dump, the build, the report, the detector registry and the previous
   release's findings. All three are uploaded and hashed into the signed
   manifest. A missing or mismatched file, an incomplete detector or an open
   blocking finding refuses the release; every existing release rule stays.
8. **Models draft, the validator decides.** A model may propose knowledge
   entries and triage findings; nothing it writes applies without passing the
   validator, and bulk adjudication keeps a human-read sample.
9. **Law only, for now.** Framework text is licensed and must not be in Git
   ([ADR 0016](0016-control-frameworks-corpus.md)), so this contract covers the
   law corpus. A link from a control to a legal provision may be reviewed
   knowledge here when it names the control by identity and quotes law text
   alone.

## Consequences

- Review work becomes countable: open findings by detector are the queue, and
  the release report shows whether it grew.
- The first run with cross-signal detectors will open many findings (274 partly
  expired documents have no incoming repeal relation). They are triaged in
  batches; only detectors marked blocking stop a release.
- Reviewed entries carry their dependencies, so a source change, or a change of
  extraction that moves structure, makes the entries it touches stale. That
  costs a re-review after each such change, not a silent wrong answer.
- Downstream products get a change list and stable identities to hang private
  mappings on.

## Alternatives

- **Keep adding one-off checks and files.** Rejected: each fix this week was a
  new file or rule with its own validation; three quote-matching rules already
  differ.
- **Let a model decide validity or relations at build time.** Rejected: the
  build stays deterministic and offline
  ([validity status](../design/validity-status.md)).
- **Fail the run on every new finding.** Rejected: most findings need review,
  not a stop; blocking is a property of a detector.

## History

- 2026-11-15: proposed after the owner's request and the write-path survey.
- 2026-11-15: revised after independent review (S-01 to S-12, A-01): typed
  subjects, frozen observations and decisions in Git, complete dependencies,
  provision effects as reviewed relations, claim-level detector, closed intake,
  sidecars bound to the signed release, one gate table.
- 2026-11-15: second review round (S-13 to S-19, A-02): no stored finding state,
  an evidence stage and artifact after evaluation, per-kind evidence and
  dependencies, effects without dates, schema capability checked at build and
  deployment at publication, no count detector, law only.
- 2026-11-15: third review round (S-13, S-18, S-20 to S-22) and the corpus
  session's reading: occurrence inherits only from an unchanged finding, the
  predecessor comes from the selected report, decisions apply only in the
  evidence stage, the stage has a recipe and input hash, no schema version for
  effects, an identity summary instead of the previous database.
- 2026-11-15: fourth review round (S-22, S-23): a changed evidence recipe needs
  a new run; the release floor is set at the first image that admits reviewed
  effects before a corpus holding them is published.
- 2026-11-15: accepted after the fifth review round found nothing open.
- 2026-11-15: after sc365 read the contract: `identity.jsonl` uploaded and
  signed, per-provision hash defined as SHA-256 of the stored text, validity
  dates and taxonomy codes in the summary, the change list as JSON Lines, the
  intake claim codes fixed.
- 2026-11-15: after sc365's application read: the evidence files move to their
  own note; the summary gains parent anchors, an ambiguity mark, relation ends
  with `exact` and `named`, and provision validity rows; the change list binds
  the predecessor's summary and records a hand-over's key change; one hash
  format.
- 2026-11-18: the selected report is the `eval` stage's own, compared with a
  previous report the run freezes at start; the uploader requires that previous
  to be the production release. Reviewed knowledge lives in
  `sources/<cc>-knowledge.yaml`.
