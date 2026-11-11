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

`identity-map.json` is `{version:2,legacy_sha256,entries:[...]}`. Each entry
contains `case_id`, zero-based `citation_index`, `state` (`resolved` or
`absent`), `expected_number`, `reason`, `reviewed_at`,
`evidence:[{source_url,fetch_sha256,document_key,document_number,node_id,quoted_span}]`,
and `alternatives`. Each alternative is
`{country,document_key,document_number,anchor:null|string,kind}`; kind is
`document`, `provision`, or `relation`. A relation additionally requires
`source_key,source_number,target_key,target_number,relation_kind,evidence_sha256`.
Every number field tied to a document key is required and stores its normalized
number under [build normalization](normalization.md). Evidence nodes are
identified by `(document_key,node_id)`, never node id alone. Document and
document-level relation anchors are null; provision anchors are exact. Use
official legal text and stored evidence, never retrieval rank, to adjudicate.

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
numbers, and each document-qualified evidence node. Missing records or
missing/changed identities yield `mapping_incomplete` for that case, with
citation index and a detail naming the field, such as
`alternatives[0].source_number` or `evidence[0].document_key`. This includes
version 1 maps lacking these records until re-adjudication; missing records are
case defects, not malformed-file errors. Block that case's credit and score the
others. Keep cases and citation denominators; affected metrics are unavailable,
so quality gates cannot pass. A refetch preserving mapped identities stays valid
despite changed artifact hashes. Fetch hashes record review evidence, not
freshness. Relation credit still requires the edge and evidence hash below.

For a new run, use `corpus inventory <cc> <rel> --out FILE` after relate. It
exports the inventory, not a rewritten map. The adjudicator reviews verified
artifacts and official text, then edits `identity-map.json`; the primary reviews
the evidence before accepting those bytes. Reuse an unchanged valid map. No
build/eval command rewrites or automatically re-adjudicates it. Re-adjudicate
version 1 maps into version 2, removing `identity_inventory_hash` and supplying
every required identity record above. Map bytes are pinned by the
[build input hash](index.md#build-stage) and eval input hash, not a recipe;
editing the map requires new build/eval input, not changed frozen recipes.
