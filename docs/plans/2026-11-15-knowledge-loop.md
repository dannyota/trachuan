# Findings and reviewed knowledge plan

**Goal:** the corpus write path opens findings from registered detectors, applies reviewed knowledge down to provisions and relations under one validator, and every release ships `findings.json` and `changes.json` compared with the previous release.

**Spec:** `docs/adr/0017-findings-and-reviewed-knowledge.md`, `docs/design/knowledge-loop.md`, `docs/research/write-path-findings-2026-11.md`. Related: `docs/design/validity-status.md`, `relations-text.md`, `ocr-check.md`, `error-reports.md`, `evaluation-fixtures.md`, `corpus-release.md`.

**Routes:** the validator, application of knowledge in build and the uploader gate are hard (`gpt-6.1-sol` implements, `gpt-6-astra` reviews); detectors, CLI and reports are routine (`gpt-6.1-terra`, `gpt-6.1-sol`). Executor: trachuan-dev (corpus), after the batch 2 release. Standing rules: no model and no network at build; existing gap kinds keep their names; no server-checked recipe changes without telling the coordinator.

## 16.0 Design (primary, astra review)
- [ ] ADR 0017, knowledge-loop.md and the survey note reviewed by `gpt-6-astra`; findings fixed; ADR accepted; sc365's comments on `changes.json` recorded.

## 16.1 Registry and findings
- [ ] Go detector registry as the only list (`corpus detectors list`), subject types, discriminators, completeness declaration, semantics numbers; the `detect` recipe; a test that every gap kind written anywhere is registered; `detector_incomplete` run failure.
- [ ] Workshop migration `finding_observation` (immutable, written under the run fence, `observed` flag); state computation (open, accepted, resolved, unobserved; recurrence reopens; fingerprint or semantics change ends an acceptance) with tests; `corpus findings list|show`.

## 16.2 Cross-signal detectors
- [ ] `unmatched_operative_claim` (relation grammar read-only over every document with text, one finding per sentence), `partial_without_provision_effect` (info), `raw_field_not_normalized`, `ocr_label_item_outside_labels`, `waived_document_without_relations`, with fixtures from this week's defects; counts on the current release reported; none blocking.
- [ ] `tree_transcription_divergence`, `relation_kind_disagreement`, `count_drift` (tolerances in budgets.md, `baseline` on first comparison, never counts itself).

## 16.3 Knowledge contract
- [ ] One validator (`corpus knowledge check`, build, CI test over `sources/`): complete `depends` per kind recomputed from the frozen run, exact-substring quote, refusal of `~k` anchors and URL evidence for new kinds, stale handling (`knowledge_stale`), entries validated together.
- [ ] `review_decision` bound to key, fingerprint and semantics number.
- [ ] `provision_effect` and `relation`: corpus schema version with relation method `reviewed`; server admits the method in warnings; evaluation case; combined rules (redundant, contradictory kind fails, ancestry and dates kept); deployment order image, floor, release; the validator refuses the kind until the image is live. Tell the coordinator before the schema change.

## 16.4 Intake and release evidence
- [ ] `sources/<cc>-reports.yaml` with the closed schema, origin registry, 500-record bound, replay idempotence; detector `external_report`; `corpus findings import` converts an export batch into records for a pull request; error-reports.md keeps its batch guarantees.
- [ ] `findings.json` and `changes.json` finalized by `eval` with bindings (rel, dump hash, detect recipe hash, previous rel and report hash); both in the artifact manifest and uploaded; their hashes in the signed manifest; uploader and signing validator refuse missing, mismatched or incomplete evidence; the gate table of knowledge-loop.md as tests.

## 16.5 First use
- [ ] Triage the first run's findings in batches (model-adjudicated with a 10 percent read sample); operative claims in the 394 partly expired documents and the 196 `ocr_label_flagged` documents first.
- [ ] Migrate gap kinds to findings one named migration at a time, converting accepted gaps.
- [ ] Rule in instructions/workflow.md and the PR template: a defect found by a person becomes a detector or a knowledge entry before its fix merges.
