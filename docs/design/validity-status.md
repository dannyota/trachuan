# Validity corrections

This note owns how the build corrects document validity when the source is wrong
or silent: automatic rules backed by the document text, a reviewed manual list,
and the evidence both leave. [Build normalization](index.md#normalization) owns
source status mapping and the consolidated derivation;
[retrieval](retrieval-recipe.md#validity-and-evidence) owns eligibility at query
time. The changes ship in corpus schema version 3 per
[scope adoption](scope-adoption.md).

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

## Fail-closed principle

Automatic rules apply only to the narrow shapes below. Every other shape changes
nothing and becomes a review entry with a proposed manual correction. Reason:
reading legal effect from text has more variants than a grammar can enumerate;
the grammar's job is to be right when it fires and loud when it does not, and
the manual list carries the rest.

## Text evidence

Automatic rules read only the document's own effect article: a `dieu` whose
heading, NFC and lowercase, equals `hiệu lực thi hành` or `điều khoản thi hành`,
from the run's verified extract artifact. Text inside quotation marks is
ignored. Sentences end at `.`; within a sentence, list items are separated by
`;`, `,` before an instrument, or `và` before an instrument, and every item
inherits the sentence's subject and verb. The subject must be the document
itself (`<loại văn bản> này`, the type equal to the document's own type).

- Commencement sentence: `<subject> có hiệu lực [thi hành] [kể] từ ngày <date>`,
  date spelled (`D tháng M năm YYYY`) or numeric (`D/M/YYYY`).
- Replacement sentence: `<subject> thay thế [cho] <items>` or
  `<subject> bãi bỏ <items>`, also when coordinated in one sentence
  (`thay thế cho A; B và bãi bỏ Điều 3 của C`). A whole item is
  `<loại văn bản> số <number> ngày <date> [của <issuer>] ...`. An item
  introduced by `Điều`, `khoản`, `điểm`, `Chương`, `Mục`, `Phụ lục` or `cụm từ`
  is partial.

The article is usable only when it holds exactly one commencement sentence, that
sentence has no `trừ` exception, the article has no clause-scoped commencement
sentence, and every sentence containing `có hiệu lực`, `thay thế`, `bãi bỏ` or
`hết hiệu lực` parses as one of the forms above. Anything else makes the article
unresolved. The complete Điều 37 of `52/2024/NĐ-CP` is an acceptance fixture:
two whole replacements and one partial repeal.

## Automatic rules

Non-consolidated documents only. Compute in three passes over all documents, so
the result does not depend on traversal order or on a document's latest status:

1. Operative commencement date of every document: the manual correction's
   `eff_from` when one exists; else rule A's date; else the source `eff_from`
   when the usable article states the same date or the document has no usable
   article. A source date that differs from the stated date on a document
   outside rule A is a date conflict: the document has no operative date, makes
   no events and enters the review queue.
2. Replacement events from every document with an operative date.
3. Apply events and conflicts, then the remaining manual fields.

**A. Commencement.** Candidate: source class `not_yet`, or status code and
status name both empty. Conditions: usable article with commencement date D; D
not before the issue date; raw `eff_to` empty; no date or interval normalization
gap on the document; no resolved `amends_supplements`, `suspends`, `replaces` or
`repeals` relation targeting it from any source. Result: `eff_from = D`, class
`in_force,true`, basis `derived_from_text`. Query-time date eligibility still
keeps a future D out of current results. An empty code with a nonempty status
name is never a candidate.

**B. Whole-document replacement event.** For every document with a usable
article and an operative date D, whatever its own status: each whole item is an
event `(actor, target, D)` when its number, document type and stated issue date
all match exactly one corpus document, D is after the target's `eff_from`, and
the issuer is verified: the item states an issuer equal to the target's, or the
normalized number has the form `n/YYYY/<type>-<issuer code>` whose code names
the issuer (`NĐ-CP`, `QH15`, `TT-NHNN`). Apply: the target's `eff_to = D`
(exclusive) with basis `derived_from_text`, unless it already has an earlier
end; the status class is not changed. Retrieval's date interval then keeps the
target current until D and not after, so a replacement scheduled for a future
date never hides the predecessor early. Several events on one target use the
earliest D. An item that matches no document or several, lacks a stated date, or
lacks a verified issuer produces no event and a review entry naming the failure.
A diagram relation is recorded as agreement, never required.

**C. Unconfirmed replacement.** A resolved diagram `replaces` relation from a
current, started source to a target whose source class is `in_force` (not
`partial`) sets `validity_conflict` on the target only when the source's effect
article is usable and names the target in no item at all, whole or partial. The
target then keeps its status but is not eligible for the default current list
and appears in the non-current group as unresolved. When the source's article is
unresolved or missing, the relation changes nothing and becomes a review entry
`replaces_unverified` with the article text, so a manual correction can settle
it. `repeals` relations and `partial` targets only add a review entry. Reason,
measured on `vn/20261110_4`: the parser read few effect articles (1,081
unresolved), and rule C fired 24 times on unread articles, hiding texts that are
only partly replaced, such as `23/2024/TT-NHNN` and `15/2019/TT-NHNN` (the
replacing circular names single articles). Hiding valid law is as harmful as
showing replaced law, so an unread article is never evidence.

Partial effects never change document status in this release; provision-level
validity is later work.

## Manual corrections

`sources/vn-corrections.yaml` is reviewed input data like the identity map: its
parsed entries are pinned at build planning and the SHA-256 of their canonical
JSON (entries sorted by key, fixed field order) is part of the build input, not
of the frozen source list. A new entry therefore needs only a new build and
evaluation, never a new ingest run; a missing file is an empty list. Each entry:

```yaml
- key: vn:vbpl:167087
  number: 52/2024/NĐ-CP
  set: { status_class: in_force, eff_from: "2024-07-01" }
  eligible: true
  reason: VBPL metadata gives effFrom 2027-07-01
  evidence:
    {
      anchor: "Điều 37 khoản 1",
      quote: "Nghị định này có hiệu lực thi hành từ ngày 01 tháng 7 năm 2024.",
    }
  reviewed: "2026-11-10"
```

`set` may hold `status_class`, `eff_from`, `eff_to`. Optional `eligible` (`true`
or `false`) overrides `validity_conflict` in either direction. Optional
`hold: true` suppresses rules A, B and C for that document as a target. An entry
needs at least one of the three. The build rejects an entry whose key is absent
from the run, whose number does not match the key, whose quote is not found in
the named provision when an anchor is given (the provision of the corrected
document, or of the document named by optional `evidence.key` when the evidence
lives in another document, such as the replacing text), or which produces an
invalid interval. Entries apply last and win. The build reports
`correction_redundant` when the source or an automatic rule now gives the same
result and `correction_conflict` when an automatic rule gives a different one.
Reason: the source and our parser both fail in ways we cannot enumerate; a short
reviewed list repairs them without widening the rules.

## Review queue and evidence

The build writes `validity-review.json` beside the build record: every
unresolved effect article of any non-consolidated document; every date conflict
from pass 1; every skipped rule B item with its identity or timing failure;
every rule A candidate blocked by a relation, date gap or status name; every
rule C conflict; every `repeals` pair with both sides current; every document
left `unknown`; every correction report. Each entry has keys, numbers, raw
status and dates, the sentences read, artifact hashes and, where the parser has
a reading, a proposed manual entry.

Each affected document stores a decision history of at most 8 records, in
application order, the last one deciding: layer (`source`, `A`, `B`, `C`,
`manual`), rule version, values before and after, acting document key and D for
B and C, the relation id for C, provision anchor, quote capped at 400 characters
and extract artifact hash. More than 8 fails the build for that document's
review. Retrieval returns the history with every hit of the document, outside
the relation cap.

## Contracts changed

- [Corpus schema](corpus-schema.md): `status_basis` gains `derived_from_text`
  and `manual_correction`; `document.validity_conflict boolean`; a
  `validity_decision` table. Schema version 3, per
  [scope adoption](scope-adoption.md).
- [Build normalization](index.md#normalization): the consolidated derivation is
  unchanged and still counts only source-supplied `in_force` or `partial`
  targets; derived and corrected targets do not count in this release.
  `validity_rules_version` (`validity/1`) joins the build recipe and the
  corrections file hash joins the build input; the shared `normalizer_version`
  is not bumped, because chunk recipes depend on it, so a frozen run is never
  rebuilt under new rules.
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
replaces A), both processing orders of a chain, an expired actor, an adverse
status name, a cleared invalid interval, list continuation, every manual entry
check, and both retrieval modes. Before the new baseline is accepted, the
primary reviews every rule B expiry, every rule C conflict and the whole review
queue for the run, and records the counts and the recall cost in the research
note.
