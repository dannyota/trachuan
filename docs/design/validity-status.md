# Validity corrections

This note owns how the build corrects document validity when the source is wrong
or silent: automatic rules backed by the document text, a reviewed manual list,
and the evidence both leave. [Build normalization](index.md#normalization) owns
source status mapping and the consolidated derivation;
[retrieval](retrieval-recipe.md#validity-and-evidence) owns eligibility at query
time. The changes ship with the next corpus schema version, together with scope
adoption.

## Problem

Run `vn/20261110_2` returned replaced law as current law, measured on the corpus
database:

- `52/2024/NĐ-CP` (issued 2024-05-15): VBPL says status `CCHL` (not yet in
  force), `effFrom` 2027-07-01. Its Điều 37 says "1. Nghị định này có hiệu lực
  thi hành từ ngày 01 tháng 7 năm 2024. 2. Nghị định này thay thế cho Nghị định
  số 101/2012/NĐ-CP ...; Nghị định số 80/2016/NĐ-CP ... và bãi bỏ Điều 3 của
  Nghị định số 16/2019/NĐ-CP".
- `101/2012/NĐ-CP`: source `CHL` (in force), `effTo` 2027-07-01. It ranks first
  as current law. `80/2016/NĐ-CP`: source `CHL`, no end date.
- `134/2025/QH15`: empty status code, so `unknown` and excluded, although its
  Điều 34 and `effFrom` both say 2026-03-01.
- 55 resolved `replaces` pairs have a started current source and a current
  target; 188 pairs counting `repeals`. 6 non-consolidated documents have an
  empty status code.

Reason for acting: a wrong status hides governing text and promotes the text it
replaced. Reason for caution: a derived status is a legal claim we cannot
adjudicate in general, so every rule fails closed and leaves evidence.

## Layers

Precedence, highest first: manual correction, automatic text rule, source.
`status_basis` records which one decided: `manual_correction`,
`derived_from_text`, `source_status`, `derived_from_consolidated`. Raw source
status and dates are always preserved. A lower layer's result that a higher
layer overrides is kept in the validity evidence.

## Text evidence

Automatic rules read only the document's own effect article: a `dieu` whose
heading, NFC and lowercase, equals `hiệu lực thi hành` or `điều khoản thi hành`,
from the run's verified extract artifact. Sentences are split on `.` and `;` at
clause level; text inside quotation marks is ignored. The subject must be the
document itself (`<loại văn bản> này`, with the type equal to the document's own
type).

- Commencement sentence: `<subject> có hiệu lực [thi hành] [kể] từ ngày <date>`
  with a spelled (`D tháng M năm YYYY`) or numeric (`D/M/YYYY`) date. A trailing
  `trừ ...` exception is allowed and recorded as `staggered_effect`.
- Replacement sentence: `<subject> thay thế [cho] <instruments>` or
  `<subject> bãi bỏ <instruments>`, where each instrument is
  `<loại văn bản> số <number> [ngày <date>] ...`. An instrument introduced by
  `Điều`, `khoản`, `điểm`, `Chương`, `Mục` or `Phụ lục`
  (`bãi bỏ Điều 3 của ...`) is a partial effect and never a whole-document
  effect.

The article is usable only when every sentence in it that contains
`có hiệu lực`, `thay thế`, `bãi bỏ` or `hết hiệu lực` parses as one of these
forms or as a clause-scoped sentence about a named `Điều`, `khoản` or `điểm`.
One unparsed sentence makes the whole article unresolved. Reason: a sentence we
cannot read may be the exception that matters.

## Automatic rules

Applied in this order to non-consolidated documents.

**A. Commencement.** For a document whose source class is `not_yet` or whose
status code is empty: when the usable article has exactly one commencement date
D, D is not before the issue date, D is before any retained `eff_to`, and the
corpus holds no resolved `amends_supplements`, `suspends`, `replaces` or
`repeals` relation targeting the document from any source, set `eff_from = D`,
class `in_force,true`, basis `derived_from_text`. Query-time date eligibility
still keeps a future D out of current results. Otherwise no change and a review
entry (below). Reason: the enacting clause is the legal source of the date, but
a later instrument can move it, so any incoming amendment sends the case to
review instead.

**B. Whole-document replacement.** For a source document that is current and
started after rule A and manual corrections: for each instrument in a
replacement sentence that resolves to exactly one corpus document by normalized
number (and by issue date when the sentence states one), with the source's
commencement D after that document's `eff_from`: set the target's class
`expired,false`, `eff_to = D` (exclusive), basis `derived_from_text`, unless the
target already has an earlier end or a source class of `expired`. An existing
later `eff_to` is replaced by D. The effect is an event: a later change to the
source's own status never revives the target. A matching diagram relation is
recorded as agreement but not required. Reason: the replacing text names its
targets explicitly; the diagram edge alone has no scope.

**C. Unconfirmed replacement.** A resolved diagram `replaces` relation from a
current, started source to a current target that rule B did not confirm marks
the target `validity_conflict`: it keeps its source status but is not eligible
for the default current list, and is shown in the non-current group as
unresolved. `repeals` relations are often partial, so they only add a review
entry and the existing warning. Reason: handing an agent a probably replaced
text as current is the failure this note exists to stop; the cost is recall on
targets the source mislabels as replaced, measured before release.

Partial effects (one article repealed) do not change document status in this
release; provision-level validity is later work.

## Manual corrections

`sources/vn-corrections.yaml` is part of the frozen source list. Each entry:

```yaml
- key: vn:vbpl:167087
  number: 52/2024/NĐ-CP
  set: { status_class: in_force, eff_from: "2024-07-01" }
  reason: VBPL metadata gives effFrom 2027-07-01
  evidence:
    {
      anchor: "Điều 37 khoản 1",
      quote: "Nghị định này có hiệu lực thi hành từ ngày 01 tháng 7 năm 2024.",
    }
  reviewed: "2026-11-10"
```

`set` may hold `status_class`, `eff_from`, `eff_to`, or be empty with
`hold: true` to keep the source values and suppress the automatic rules for that
document. The build rejects an entry whose key is absent from the run, whose
number does not match the key, whose quote is not found in the named provision
when an anchor is given, or which produces an invalid interval. Entries apply
last and win. The build reports `correction_redundant` when the source or an
automatic rule now gives the same result, and `correction_conflict` when an
automatic rule gives a different one. Reason: the source and our parser both
fail in ways we cannot enumerate; a short reviewed list repairs them without
widening the rules.

## Review queue and evidence

The build writes `validity-review.json` beside the build record: every
unresolved article, rule A candidate blocked by an incoming relation, rule C
conflict, `repeals` pair with both sides current, empty-status document left
`unknown`, and correction report, each with keys, numbers, raw status and dates,
the sentences read and artifact hashes. It is the input for new manual entries.

Each corrected or conflicted document stores one validity evidence record: rule
(`A`, `B`, `C`, `manual`), rule version, raw status code and name, raw dates,
derived values, the acting document key and D for rule B, provision anchor,
quote capped at 400 characters, and extract artifact hash. Retrieval returns it
with every hit of that document, outside the relation cap.

## Contracts changed

- [Corpus schema](corpus-schema.md): `status_basis` gains `derived_from_text`
  and `manual_correction`; `document.validity_conflict boolean`; a
  `validity_evidence` table. Next schema version.
- [Build normalization](index.md#normalization): the consolidated derivation is
  unchanged and still counts only source-supplied `in_force` or `partial`
  targets; derived and corrected targets do not count in this release.
  `normalizer_version` changes and the corrections file hash joins the build
  recipe, so a frozen run is never rebuilt under new rules.
- [Retrieval](retrieval-recipe.md#validity-and-evidence): eligibility adds "not
  `validity_conflict`"; the `validity` policy version is bumped; `Validity`
  carries the evidence record.
- [Evaluation](evaluation.md): current-law precision is described as consistency
  with stored status. Add adjudicated validity cases, scored over every primary
  hit, for `52/2024`, `101/2012`, `80/2016`, `134/2025` and `16/2019` (partial
  repeal, stays current), and report returned hits by `status_basis` and by
  conflict kind.
- `sources/vn.yaml`: `TNHL` ("Ngưng hiệu lực") maps to `suspended,false`, with a
  recorded-source fixture for the code and label.

## Bounds, tests and release gate

No network or model at build time; only verified artifacts and resolved
relations. Tests cover each sentence form, quoted text, a clause-scoped date, an
unparsed sentence, staggered effect, D before issue, D after `eff_to`, an
incoming amendment, a partial repeal, an ambiguous number, chains (C replaces B
replaces A), every manual entry check, and both retrieval modes. Before the new
baseline is accepted, the primary reviews every rule B expiry, every rule C
conflict and the whole review queue for the run, and records the counts and the
recall cost in the research note.
