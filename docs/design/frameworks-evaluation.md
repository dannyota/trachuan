# Control frameworks: evaluation

This note owns how a frameworks release is evaluated: the golden cases and their
adjudication, what counts as a hit, the cohorts, the measured retrieval recipe,
the release invariant check, the floors and the report. The steps that run it
are in [build](frameworks-build.md); the gate rules are in
[frameworks](frameworks.md#evaluation); the access rules are in
[framework access](frameworks-access.md).

## Golden cases

The starting cohort is compliary's 205 cases (`deploy/eval/golden.csv`: `id`,
`question`, `framework_code`, `version_label`, `citation_norm`,
`expect_abstain`, `expect_fail`, `notes`); `expect_fail` is compliary's
known-failure flag and must be false for an accepted case. The working copy is
`FRAMEWORKS_SOURCE_DIR/trachuan/golden.csv`, outside Git like the titles and
waivers, owned by the coordinator, and a `snapshot` input, so a run reads only
its frozen copy. It adds `adjudication` (`accepted`, `rejected`,
`out_of_release` or `unverified`) and `reason`. `golden-review.yaml` beside it
holds one entry per change: release, reviewer (`coordinator`), date, the CSV's
SHA-256 and the case ids added, rejected or re-targeted. A CSV whose hash has no
entry fails `snapshot`. The file is closed YAML: a `reviews` list whose entries
have exactly `release`, `reviewer`, `date` (`YYYY-MM-DD`), `csv_sha256` (64
lower-case hex), `added`, `rejected` and `retargeted` (lists of case ids) and
`note` (one line).

**Mapping a case to a control.** `framework_code` must be a registry code;
`version_label` names a registered version by the
[relate rule](frameworks-build.md#steps) (exact version, else its major number);
`citation_norm` is normalized by the version's citation scheme (for
`oscal-catalog`, leading zeros of control and enhancement numbers are removed:
`AC-02(03)` is `AC-2(3)`) and must equal one control's `citation_norm` in that
version. The target is that control's handle. A case whose version is not in the
release is `out_of_release`: counted per version, never scored. A case whose
version is in the release but whose citation matches no control fails the step;
it is fixed by a review entry, not skipped.

**Labels come only from permitted inputs.** A negative is a case whose question
names a framework absent from the registry or a topic outside security and
compliance; it is labelled from the question and the registry alone, never by
searching or reading any version's text. So no change to licensed text can turn
a case into or out of the negative set.

**Adjudication.** Every in-release positive is adjudicated against the release
before it is scored. For a `free` version the reviewer reads the target
control's text and marks it `accepted` or `rejected` with a reason. For a
`licensed` version no licensed text is read into any model context: the case
keeps compliary's verdict when an owner attestation binds that verdict to the
same source file (the registry SHA-256 of the version's catalog) and the
version's parse passed its control-count check in this release; otherwise it is
`unverified`. `unverified` cases are run and reported but never gated.

## Hits and metrics

A positive case hits at rank _r_ when the _r_-th hit's handle equals the target
handle. A hit at another version of the same framework, at the target's parent
or at a child does not count; the report counts those as `near_hits` for
diagnosis. A lookup response (citation routing) counts like a ranked list, in
its order.

- **recall@8**: positives with a hit at rank 8 or better, over positives.
- **MRR@8**: the mean of 1/_r_ for a hit at rank _r_ ≤ 8, zero otherwise.
- **current-version rate**: among hits of a framework with more than one version
  in the release, the share at the target version; `not_applicable` while every
  framework has one version.
- **abstention**: for positives, the answer rate; for negatives, the abstention
  rate; balanced accuracy is their mean.

Every metric has its numerator and denominator, overall and per version. A
metric with a zero denominator, or one with an errored case among its gated
cases, is `unavailable`, and an unavailable gated metric fails. `unverified`
cases are diagnostic only: no gate, error, filter or floor counts them.

## Cohorts and callers

Each scored case runs through the engine entry points the server uses, at limit
8, on the restored evaluation copy, in two modes:

| Mode       | Filter                                | Cases                |
| ---------- | ------------------------------------- | -------------------- |
| `open`     | none                                  | positives, negatives |
| `filtered` | `frameworks: [target framework code]` | positives            |

The filter gate is per case: every positive that hits in `open` must hit in
`filtered`; any loss fails. Aggregate gains are reported separately.

Two callers run every case: **free-only** (free versions readable, licensed
versions reduced to their public fields) and **full** (every version readable).

| Cohort     | Caller    | Cases                                          | Floors live in                   |
| ---------- | --------- | ---------------------------------------------- | -------------------------------- |
| `public`   | free-only | accepted positives on free versions, negatives | `sources/frameworks-floors.yaml` |
| `licensed` | full      | every accepted positive and negative           | `trachuan/floors.yaml`           |

The `public` cohort depends only on free text and licensed public fields, so its
metrics and floors may be published. The `licensed` cohort's ranking depends on
licensed bodies; its metrics stay in the private report and its floors in
`FRAMEWORKS_SOURCE_DIR/trachuan/floors.yaml`, coordinator-owned, never in Git.

## The measured recipe

Only the abstention threshold is measured. The fusion constant, arm weights and
depths keep the law release's values: 33 free-version positives cannot tune more
parameters without fitting noise; they are revisited when the cohort grows.

`build` calibrates after loading and indexing and before it writes the release
row and the dump, the way the law build does ([calibration](calibration.md)):

- it runs every `public` cohort case in `open` mode as the free-only caller on
  the build database, through the engine without the threshold, recording for
  each answered response the top hit's best permitted cosine;
- an empty response is a fixed abstention; a lookup response is not fitted (the
  threshold does not apply to lookups) and is counted; a missing class (no
  positives or no negatives), a non-finite score or an execution error fails the
  step;
- candidate thresholds are the midpoints between consecutive distinct observed
  cosines plus the always-answer and always-abstain sentinels; the chosen one
  maximizes balanced accuracy, ties to the lowest.

The calibration record (threshold, every candidate's TP/FP/TN/FN, the case-file
hash, the engine and query-embedder recipe hashes) is written in the `build`
step directory; its SHA-256 goes into `release.retrieval_recipe`, whose status
becomes `measured`. When the snapshot holds no reviewed case file, `build`
writes the `placeholder` recipe instead and `quality_pass` stays false. A retry
of `build` recomputes the calibration inside the same atomic step; `evaluate`
always runs on the dump that step published. Calibrating on the scored cohort
biases its abstention result; the report says so.

## Release invariant check

The synthetic suite runs in CI. At `evaluate` the release is checked too, once
per comparison caller: free-only, and, when the release has licensed versions of
at least two frameworks, one caller per such framework that can read that
framework's licensed versions only, since entitlements are per framework code
(otherwise mixed coverage is `not_applicable`). For each caller a copy of the
restored database is mutated in every version that caller cannot read:

| Private class    | Mutation                                                            |
| ---------------- | ------------------------------------------------------------------- |
| control text     | body, `title_original`, notes and tables replaced deterministically |
| body index       | chunk content, vectors, postings and their statistics replaced      |
| mapping evidence | each value with that version in its set replaced; one added if none |
| text gaps        | each text-class gap detail changed; one text gap added if none      |
| source files     | text source and completeness of its files changed                   |

Public fields, public chunks and the public fields of edges stay unchanged. The
classes are mutated one at a time on the copy, in the table's order, and the
comparison runs after each one, not only after the last, so leaks in two classes
cannot cancel: a difference at any stage fails. At each stage the caller must
get byte-identical responses on both copies for every case in both modes and
every page of each, a read of every target with all continuations, the mappings
of every target, `corpus_status`, every `quality_gaps` page, each operation
twice (so a cache hit is covered), and a recalibration on the mutated copy must
give the same threshold. A positive control per class shows the mutation took
effect: the full caller sees a difference in each class (read text, search
scores, evidence, text gaps, sources). Any failure, or a class with no observed
difference, sets `quality_pass` false and names the operation, class and case
id, never text.

## Floors

Floors are adopted, not inferred. After the first measured run the coordinator
reviews its numbers and records the baseline: recall@8 and MRR@8 per mode and
the balanced abstention accuracy, each rounded down to the whole percent, for
the `public` cohort in `sources/frameworks-floors.yaml` (in Git) and for the
`licensed` cohort in `floors.yaml`, each with the report's SHA-256. Until a
baseline is recorded, `quality_pass` is false. The cohorts are small (33
free-version positives, 12 licensed, 15 negatives), so one case moves recall by
several points; the report states that next to every metric.

Both floor files are closed YAML with the same shape: a `baselines` list whose
entries have exactly `release`, `report_sha256`, `reviewer`, `date`,
`cohort_change` (empty, or the recorded change that explains a lower floor) and
`floors`, which holds `open` and `filtered`, each with `recall_at_8` and
`mrr_at_8`, and `abstention_balanced_accuracy`, every value a whole percent. The
last entry is in force; `evaluate` reads both files from the snapshot and fails
when an entry lowers a floor below the one before it with an empty
`cohort_change`.

The previous signed report is a `snapshot` input:
`FRAMEWORKS_SOURCE_DIR/trachuan/previous-report.json`, the active frameworks
release's `report.json`, placed by the coordinator. When it is present, paired
non-regression runs and gates `quality_pass`; when it is absent the report
records `previous: null` and the comparison as `not_applicable`, and the
uploader refuses such a report while a frameworks release is active
([build](frameworks-build.md#gaps-and-signing)).

Later releases:

- every floor is met on the current cohort;
- non-regression compares paired cases only: the same id with the same target,
  accepted in both releases; a case that hit at rank 8 or better in the previous
  signed report and misses now fails, and the paired recall and MRR may not
  fall;
- added, rejected or re-targeted cases and newly in-release versions are cohort
  changes, reported apart from retrieval changes; they take effect only through
  a review entry made before the build, never to admit a failing release. A
  version is newly in the release when this report counts any of its cases as
  accepted, rejected or `unverified` and the previous report's per-version
  counts have it only as `out_of_release` or not at all; case ids play no part;
- a floor never falls below the previous release's floor unless a recorded
  cohort change explains it.

## Report

`report.json` carries, besides the fields of
[build](frameworks-build.md#gaps-and-signing): the case file and review hashes;
per-version counts of accepted, rejected, `unverified` and `out_of_release`
cases; every metric per cohort, mode and version with numerators, denominators
and the noise statement; `near_hits`; the per-case filter result; the
calibration record's hash and threshold; the invariant check per caller and
class; each floor with its value and result; the paired comparison; the
retrieval recipe status; and, per case, its id, adjudication, target handle, and
per cohort and mode its rank or miss and its abstention outcome, so the next
release can pair cases and recompute paired metrics. It holds case ids, handles
and counts, never question text or control text, and stays beside the dump,
never in Git.
