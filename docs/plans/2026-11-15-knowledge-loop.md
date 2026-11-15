# Findings and reviewed knowledge plan

**Goal:** the corpus write path opens findings from registered detectors, applies reviewed knowledge down to provisions and relations under one validator, and every release ships `findings.json` and `changes.json` compared with the previous release.

**Spec:** `docs/adr/0017-findings-and-reviewed-knowledge.md`, `docs/design/knowledge-loop.md`, `docs/research/write-path-findings-2026-11.md`. Related: `docs/design/validity-status.md`, `relations-text.md`, `ocr-check.md`, `error-reports.md`, `evaluation-fixtures.md`, `corpus-release.md`.

**Routes:** the validator, application of knowledge in build and the uploader gate are hard (`gpt-6.1-sol` implements, `gpt-6-astra` reviews); detectors, CLI and reports are routine (`gpt-6.1-terra`, `gpt-6.1-sol`). Executor: trachuan-dev (corpus), after the batch 2 release. Standing rules: no model and no network at build; existing gap kinds keep their names; no server-checked recipe changes without telling the coordinator.

## 16.0 Design (primary, astra review)
- [x] ADR 0017, knowledge-loop.md and the survey note reviewed by `gpt-6-astra` in five rounds (S-01 to S-23, A-01, A-02 fixed) and read by the corpus session against the code; ADR accepted 2026-11-15.
- [ ] sc365's comments on `changes.json`, `identity.jsonl` and the intake record recorded before 16.4 freezes their shapes.

## 16.1 Registry and evidence stage
- [ ] Go detector registry as the only list (`corpus detectors list`): subject types, discriminators, completeness declaration, semantics numbers, registry hash; stage gap validation rejects an unregistered kind at run time, and a test checks every kind constant against the registry.
- [ ] `evidence` stage after `eval` as one item: frozen recipe (registry, configuration, serialization), input hash (build artifact, selected report and its gap snapshot, knowledge bytes, predecessor evidence), normal commit fencing, run `evaluated` only after it; fails with `detector_incomplete`; one immutable artifact.
- [ ] `findings.json` as a pure function: predecessor from the selected report's `previous_diff` (empty when it predates evidence, failure when its artifact is missing), `first_rel` inherited only from an unchanged fired or carried finding, `gone` or `carried` for every previous key, status and `unmatched` decisions computed in this stage; `corpus findings list|show <cc> <rel>` reads the artifact only.

## 16.2 Cross-signal detectors
- [ ] `unmatched_operative_claim` (relation grammar read-only over every document with text, one finding per sentence whose target resolves to a provision; review for sources relate reads, info for incomplete sources), `partial_without_provision_effect` (info), `raw_field_not_normalized`, `ocr_label_item_outside_labels`, `waived_document_without_relations`, with fixtures from this week's defects; counts on the current release reported; none blocking.
- [ ] `relation_kind_disagreement`. `tree_transcription_divergence` waits for a numeric definition from extract's `text.json` records.

## 16.3 Knowledge contract
- [ ] One validator (`corpus knowledge check`, build, CI test over `sources/`), all entries together: per-kind target, evidence and `depends` of knowledge-loop.md recomputed from the frozen run, exact-substring quote, refusal of `~k` anchors and URL evidence, stale handling (`knowledge_stale`) for effects and relations; decisions checked for structure only.
- [ ] `review_decision` bound to key, fingerprint, semantics number and `first_rel`.
- [ ] `provision_effect` and `relation`: relation method `reviewed` (no schema version; `relation.method` is free text); server admits it in `warnings.go`, `warning_sql.go`, `read_sql.go` and `hydrate_tree.go` (dev2) with an evaluation case; identity (S, T, kind), redundant and contradictory rules; that image live and the release floor confirmed at it before the first corpus release that holds such rows; runbook line.

## 16.4 Intake and release gate
- [ ] `sources/<cc>-reports.yaml` with the closed schema, origin registry, 500-record bound, replay idempotence; detector `external_report`; `corpus findings import` converts an export batch into records for a pull request; error-reports.md keeps its batch guarantees.
- [ ] `identity.jsonl` in the evidence artifact and `changes.json` from this summary and the predecessor's (cause classes, legal and provenance changes, no moves, `baseline` when the predecessor has none); upload of both files; their hashes in the signed manifest; uploader and signing validator enforce the gate table of knowledge-loop.md; corpus-release.md updated in the same change.

## 16.5 First use
- [ ] Triage the first run's findings in batches (model-adjudicated with a 10 percent read sample); operative claims in the 394 partly expired documents and the 196 `ocr_label_flagged` documents first.
- [ ] Migrate gap kinds to findings one named migration at a time, converting accepted gaps into review decisions.
- [ ] Rule in instructions/workflow.md and the PR template: a defect found by a person becomes a detector or a knowledge entry before its fix merges.
