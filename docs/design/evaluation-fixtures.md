# Evaluation fixtures and identity adjudication

This note owns the authored case fixtures and the identity map that
[evaluation](evaluation.md) scores; it was split from that note.

Commit the authored banhmi `deploy/eval/golden.json` data unchanged to
`apps/corpus/internal/evaluation/testdata/vn/legacy.json`. Retain all 92 IDs,
questions, expectations, alternate numbers, notes and flags. The fixture is
public authored evaluation data, not captured user questions. Copy no matcher
implementation. `fixtures.json` records its source path, source commit if
available, byte hash, authoring origin and case count. `old-product.json`
references the legacy fixture hash and the three rejection IDs, rather than
duplicating questions or rewriting expectations. `development.json` holds the
development cases in the legacy case format plus a required `tags` list;
`fixtures.json` records its hash, authoring origin and case count, an empty
array is valid, and case IDs are unique across legacy and development. Its
identity entries live in `identity-map.json` beside the old product entries.

Legacy case format is the existing JSON array:
`{id,question, expected_citations:[{doc_number,article?,clause?,point?,relation_ok?, alt_doc_numbers?}],expect_abstain?,expect_fail?,notes?}`.
Missing booleans mean false. `expect_fail` never removes a case from scoring.
Validate IDs, explicit expectations and contradictory abstention labels before
any model call.

`identity-map.json` is `{version:3,legacy_sha256,entries:[...]}`. Each entry
contains `case_id`, zero-based `citation_index`, `state` (`resolved` or
`absent`), `expected_number`, `reason`, `reviewed_at`,
`evidence:[{source_url,fetch_sha256,document_key,document_number,anchor,quoted_span}]`,
and `alternatives`. Each alternative is
`{country,document_key,document_number,anchor:null|string,kind}`; kind is
`document`, `provision`, or `relation`. A relation additionally requires
`source_key,source_number,target_key,target_number,relation_kind,evidence_sha256`.
Every number field tied to a document key is required and stores its normalized
number under [build normalization](normalization.md). Evidence is identified by
`(document_key,anchor,quoted_span)`, never by a node id: node ids of
transcription and OCR text hash their text, so a text change would orphan them.
`anchor` is null for document-level evidence and otherwise an exact provision
anchor of that document. With an anchor, `quoted_span` is verbatim text of that
provision; with a null anchor it is the reviewed title line, held as review
evidence and covered by the number check. Document and document-level relation
anchors are null; provision anchors are exact. Use official legal text and
stored evidence, never retrieval rank, to adjudicate.

There is one mapping entry per original expectation, including absent documents.
Resolved entries need at least one unique adjudicated identity; absent entries
have no alternatives and are always misses. An absent entry names every
original/alternate number searched and why no corpus identity is available.
Ambiguous mappings or missing entries are `mapping_incomplete` for that case,
not invented absences. If an absent document becomes present, or a resolved one
stops being present ([presence](scope-adoption.md#presence)), require
re-adjudication, never auto-credit it. A document appears when it has the
searched number; for an expectation naming an article without relation credit, a
document with no provision anchor stays in the reason. Do not invent a document
key or provision anchor from a label.

Validate every recorded identity against the run's present documents: document
key and normalized number, provision anchor, both relation endpoints and their
numbers, and each evidence record: its `quoted_span` is not empty, and a
non-null anchor names a provision of that document whose own text contains
`quoted_span` after NFC and white-space collapse on both sides. Missing records
or missing/changed identities yield `mapping_incomplete` for that case, with
citation index and a detail naming the field, such as
`alternatives[0].source_number` or `evidence[0].document_key`. This includes
version 1 and 2 maps lacking these records until re-adjudication or conversion;
missing records are case defects, not malformed-file errors. Block that case's
credit and score the others. Keep cases and citation denominators; affected
metrics are unavailable, so quality gates cannot pass. A refetch preserving
mapped identities stays valid despite changed artifact hashes. Fetch hashes
record review evidence, not freshness. Relation credit still requires the edge
and evidence hash below.

For a new run, use `corpus inventory <cc> <rel> --out FILE` after relate. It
exports the inventory, not a rewritten map. The adjudicator reviews verified
artifacts and official text, then edits `identity-map.json`; the primary reviews
the evidence before accepting those bytes. Reuse an unchanged valid map. No
build/eval command rewrites or automatically re-adjudicates it. Re-adjudicate
version 1 maps into version 3, removing `identity_inventory_hash` and supplying
every required identity record above. A version 2 map converts to version 3 by
replacing each evidence `node_id` with that node's anchor in the release the map
was adjudicated on (null when it had none); a converted record whose quote is
not found under the rule above stays `mapping_incomplete` until re-adjudicated.
Map bytes are pinned by the [build input hash](index.md#build-stage) and eval
input hash, not a recipe; editing the map requires new build/eval input, not
changed frozen recipes.

## Revisions

The development fixture and its identity entries change only through a reviewed
Git change; build and eval never write them. The legacy and old product fixtures
are never revised. When the development fixture's hash differs from the previous
report's, the gated `development` records are compared over the **comparable
cases** instead of the whole cohort. A case is comparable when it has the same
ID in both fixtures, `question`, `expected_citations` and `expect_abstain` equal
as decoded (other fields are ignored; an absent list equals an empty one), and
the same cohort in both reports, `development` or
[`development_period`](evaluation-period-cases.md). A case is never removed
([ADR 0010](../adr/0010-retrieval-evaluation-for-agents.md), item 3), and an ID
rename is a removal: a revision that removes a case is refused.

- The previous value and the candidate value are both recomputed from the case
  records over the comparable cases, in fixture order, with the metric's own
  definition. Recall@20 compares integer matched counts over the same
  denominator. A value that cannot be recomputed (a missing or duplicate case
  record, a mapping issue, or no comparable case to score) makes the record
  `unavailable`, except that when only the previous value is missing and the
  previous report's metric is null, the status is `no_previous`.
- `high_water` is the recomputed previous value plus the
  [tolerance used](evaluation-mrr-tolerance.md#rule) by the previous record. A
  tag-only revision so keeps the watermark exactly, and a revision never resets
  it. The [MRR@8 tolerance](evaluation-mrr-tolerance.md) applies against it.
- The evaluator reads the previous fixture from
  `testdata/vn/development-previous/<sha256>.json` beside the current one: a
  revising change commits the replaced bytes there under their SHA-256. It
  checks the hash and refuses when the file is missing; with equal hashes no
  file is needed. It also refuses a previous report that holds a case in both
  development cohorts, whose case records do not match its fixture (an ID
  missing or extra, or a citation count that differs), or that moves a case
  between cohorts under an unchanged fixture hash.
- A revision record's `previous` and `high_water` hold the comparable values its
  status is judged on, and its `value` stays the value over all its cases. Its
  `revision` is `{cases, value, revised}`: the count of comparable `development`
  cases, the comparable candidate value, and every case of either report's
  `development` cohort outside the comparable set as `{case_id, reason}`, sorted
  by ID, with one reason chosen in the order `added`, `cohort`, `changed`. A
  recall record also states `numerator` and `denominator` over the candidate's
  comparable cases and `previous_numerator` and `previous_denominator` over the
  previous ones; `previous_value` holds the comparable previous value on every
  gated record, while a non-gating record such as abstention keeps the previous
  value over all its cases. `within_tolerance`, `losses` and tolerance condition
  4 use the comparable cases. When the recipes differ, `high_water` is the
  recomputed previous value alone, since a watermark never crosses a recipe
  change.
- The uploader and the signing validator take each fixture from the main
  branch's first-parent history: the version of `development.json` whose SHA-256
  equals that report's development fixture hash, refusing when there is none.
  The candidate's version must come at or after the previous report's version in
  that history, comparing the latest commit at which each version appears, so a
  run built from a stale fixture is refused. They recompute the comparable set,
  both values, `high_water`, the tolerance check and the status, and refuse a
  record whose stated fields differ from theirs. They also check that the
  candidate's `development_period` holds exactly the fixture's cases tagged
  `historical` and `development` exactly the others, and that `old_product`
  holds exactly the legacy fixture's cases less the old product fixture's
  `rejection_ids`, both taken from the same history by the report's `Legacy` and
  `OldProduct` hashes. In each of these cohorts every case has a `hybrid` row in
  both modes at limits 8 and 20, with `expected` equal to its citation count,
  and `rejections` names exactly the rejection IDs. Each rejection record holds
  only `case_id`, `reason` and `passed`, with `passed` true and `reason`
  `diacritics_required`, the only refusal the evaluator counts as passed. The
  previous report's `old_product` cohort and `rejections` are checked the same
  way. An old product fixture whose `legacy_sha256` is not the legacy fixture's
  hash is refused. The `legacy` cohort is not checked, since no gate reads it.

Accepted costs: a loss on an added or changed case is not gated in the release
that revises it, and a case moved into `development_period` leaves the gate for
good, which takes an ADR 0010 History line naming it. The tolerance already used
is carried as a difference measured over the previous membership. The guard is
the Git review of the fixture change, and `revised` lists every such case.
