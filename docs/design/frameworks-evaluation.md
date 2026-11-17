# Control frameworks: evaluation

This note owns how a frameworks release is evaluated: the golden cases and their
adjudication, what counts as a hit, the cohorts, the measured retrieval recipe,
the release invariant check and the floors. The `evaluate` step that runs it is
in [build](frameworks-build.md); the gate rules are in
[frameworks](frameworks.md#evaluation); the access rules are in
[framework access](frameworks-access.md).

## Golden cases

The starting cohort is compliary's 205 cases (`deploy/eval/golden.csv`: `id`,
`question`, `framework_code`, `version_label`, `citation_norm`,
`expect_abstain`, `notes`). The working copy is
`FRAMEWORKS_SOURCE_DIR/trachuan/golden.csv`, outside Git like the titles and
waivers, and a `snapshot` input, so a run reads only its frozen copy. It adds
two columns per case: `adjudication` (`accepted`, `rejected` or
`out_of_release`) and `reason` for a rejection; one header file beside it,
`golden-review.yaml`, records the release, reviewer, date and the CSV's SHA-256.
A changed CSV without a matching review entry fails `evaluate`.

**Mapping a case to a control.** `framework_code` must be a registry code;
`version_label` names a registered version by the
[relate rule](frameworks-build.md#steps) (exact version, else its major number);
`citation_norm` is normalized by the version's citation scheme (for
`oscal-catalog`, leading zeros of control and enhancement numbers are removed:
`AC-02(03)` is `AC-2(3)`) and must equal one control's `citation_norm` in that
version. The target is that control's handle. A case whose version is not in the
release is `out_of_release`: counted per version in the report, never scored. A
case whose version is in the release but whose citation matches no control fails
`evaluate`; it is fixed by adjudication, not skipped. A case with an empty
`framework_code` and `expect_abstain: true` is a negative.

**Adjudication.** Before the first measured run every in-release case is
re-adjudicated against this release. For a `free` version the reviewer reads the
target control's text and marks the case `accepted` when the control answers the
question, else `rejected` with a reason. For a `licensed` version nobody reads
the text: the reviewer confirms the target resolves and keeps compliary's
verdict, which the owner verified against the purchased text. Negatives are
accepted when no release version covers the topic. Only `accepted` cases are
scored. Adding or rejecting a case after the first run is a review entry and is
reported as a cohort change, never a silent edit.

## Hits and metrics

A positive case hits at rank _r_ when the _r_-th hit's handle equals the target
handle. A hit at another version of the same framework, at the target's parent
or at a child does not count; the report counts those separately as `near_hits`
(same framework and citation in another version; ancestor; descendant) for
diagnosis. A lookup response (citation routing) counts like a ranked list, in
its order.

- **recall@8**: positives with a hit at rank 8 or better, over positives.
- **MRR@8**: the mean of 1/_r_ for a hit at rank _r_ ≤ 8, zero otherwise.
- **current-version rate**: among hits of a framework with more than one version
  in the release, the share at the target version; `not_applicable` while every
  framework has one version.
- **abstention**: for positives, the share answered (not abstaining); for
  negatives, the share abstaining; balanced accuracy is their mean.

Every metric is reported with numerator and denominator, overall and per
framework version.

## Cohorts

Each accepted case runs in two modes, through the same engine entry points the
server uses, at limit 8, on the restored evaluation copy:

| Mode       | Filter                                | Cases                |
| ---------- | ------------------------------------- | -------------------- |
| `open`     | none                                  | positives, negatives |
| `filtered` | `frameworks: [target framework code]` | positives            |

`filtered` recall@8 must be at least `open` recall@8 for the same cases;
otherwise the report flags `filter_regression` and `quality_pass` is false.

## Access sets

Two callers run every case:

- **Full**: every version readable (the owner's entitlement). The reported
  metrics and floors use this caller, as frameworks.md says.
- **Free-only**: `free` versions readable, every `licensed` version reduced (its
  public fields only). Calibration uses this caller and nothing else.

## The measured recipe

The serving parameters come from free versions' text and the public fields of
licensed versions only. The fusion constant, arm weights and depths keep the
values the placeholder took from the active law release; they are structural and
depend on no text. The abstention threshold is calibrated:

- positives are the accepted cases whose target is a `free` version; negatives
  are the accepted negatives;
- each case runs in `open` mode as the free-only caller, and the top hit's best
  permitted cosine is recorded;
- candidate thresholds are the midpoints between consecutive distinct observed
  cosines plus the always-answer and always-abstain sentinels; the chosen one
  maximizes balanced accuracy, ties to the lowest, as the law
  [calibration](calibration.md) rule.

Because the free-only caller can read no licensed body, and its candidates from
licensed versions are public-field records only, no licensed text can move the
threshold. The calibration record (threshold, every candidate's TP/FP/TN/FN, the
case-file and engine hashes) is written beside the report, its SHA-256 goes into
the recipe, and the recipe status becomes `measured`. The first measured run is
a second build of the same `<rel>` inputs whose `build` step writes the measured
recipe; the placeholder release is never signed. Calibrating on the cohorts that
are also scored biases their abstention result; the report says so.

## Release invariant check

The synthetic invariant suite runs in CI. At `evaluate` the release itself is
checked as well: the evaluator makes a second copy of the restored database in
which, for every `licensed` version, every text-derived value is replaced
deterministically (body, `title_original`, notes, tables, body chunk content,
body vectors and body postings with their statistics), leaving public fields,
public chunks and edges unchanged. For the free-only caller and one mixed caller
(one licensed version readable), every golden query in both modes, a read of
every target control with all its continuations, the mappings of every target,
`corpus_status` and every `quality_gaps` page must give byte-identical responses
on both copies. The full caller must see at least one difference, which shows
the mutation took effect. A failure sets `quality_pass` false and names the
operation and case, never text.

## Floors

The first measured run sets the floors, recorded in
[frameworks](frameworks.md#evaluation) with the run's report SHA-256: recall@8
and MRR@8 in each mode for the full caller, and negative abstention, each the
measured value rounded down to the whole percent. A later release meets every
floor and, from the second release, does not regress against the previous signed
report. Changing a floor needs a review entry and is reported.

## Report

`report.json` carries, besides the fields of
[build](frameworks-build.md#gaps-and-signing): the case file and review hashes;
per-version counts of accepted, rejected and `out_of_release` cases; every
metric per mode, caller and version with numerators and denominators;
`near_hits`; the calibration record's hash and threshold; the invariant check's
result; each floor with its value and result; and the retrieval recipe status.
It holds case ids, handles and counts, never question text or control text.
