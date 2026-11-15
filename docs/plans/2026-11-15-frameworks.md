# Control frameworks plan

**Goal:** the framework corpus kind of ADR 0016 serves free framework text to every account and licensed text to entitled emails, in production behind `SERVER_FRAMEWORKS`, with the owner's email as the only entitlement until the licence terms review.

**Spec:** `docs/adr/0016-control-frameworks-corpus.md`, `docs/design/frameworks.md`, `docs/design/frameworks-access.md`, `docs/research/compliary-survey-2026-11.md`. Sources: the owner's clone of `compliary-private` (never copied into this repository); seeds from `/Users/danny/src/dannyota/compliary/deploy/seed/` and `deploy/eval/golden.csv`.

**Routes:** access, entitlements, serving and release paths are hard (Codex `implementer` `gpt-6.1-sol`, `reviewer` `gpt-6-astra`); parsers and web pages are routine (`gpt-6.1-terra`, `gpt-6.1-sol`). Standing rules: no licensed text in Git, fixtures, logs, reports or PR bodies; fixtures are invented frameworks; a framework version is `licensed` unless the registry says `free` with an open licence.

## 15.0 Design (primary, astra review)
- [ ] ADR 0016, frameworks.md and the survey note reviewed by `gpt-6-astra`; findings fixed; ADR accepted.

## 15.1 Access foundation (server; dev2)
- [ ] Tests first, then: auth migration `framework_entitlement` and `framework_entitlement_event` (mutation and event in one transaction, events immutable); the access-set function with fail-closed cases (unknown framework, missing class, store error, revoked entitlement, entitlement before account, unverified email, email canonical form, account deletion keeps it); `server admin frameworks allow|revoke|list`; the restore step that revokes every live entitlement with reason `restore`; `SERVER_FRAMEWORKS` flag default false. No serving yet.

## 15.2 Registry and free frameworks (corpus; dev)
- [ ] `sources/frameworks.yaml` with validation (hashes, access, licence kind and URL for `free`), seeded from compliary's registry; `FRAMEWORKS_SOURCE_DIR` reader that refuses unknown hashes.
- [ ] Release schema `frameworks` version 1 in `internal/corpusdb`; stages for the kind; structured parsers first: NIST SP 800-53 r5 (OSCAL), NIST CSF 2.0 and CIS Controls v8.1 (XLSX), each with an exact control-count check.
- [ ] Mapping edges from catalogs, NIST OLIR and CIS workbooks, by business key, unresolved targets counted.
- [ ] Body and public-field indexes; dense recipe of the law corpus; golden cases re-adjudicated; first measured run; floors written into frameworks.md.

## 15.3 Licensed frameworks and scans (corpus; dev)
- [ ] PDF text parsers with count checks: PCI DSS v4.0.1, ISO/IEC 27001:2022 with Amd 1, 27002:2022, 27017:2015, 27018:2019, AICPA criteria, COBIT 2019, CSA CCM v4.1 (XLSX), TCVN 11930:2017.
- [ ] OCR path for `tcvn-14423-2025.pdf`: record both providers' data-use terms in the registry first (else gap `ocr_blocked`); Vision OCR, cross-check, flags; waivers in a file under `FRAMEWORKS_SOURCE_DIR`, never in `sources/`; compare with compliary's text file when reviewing flags; report cost before running.
- [ ] Paraphrased titles imported from compliary's `control_title.csv`, coverage test.

## 15.4 Serving (server and web; dev2)
- [ ] `docs/design/frameworks-results.md`: field-level input and output contract under the fixed rules of frameworks.md "Serving"; `gpt-6-astra` review before any serving code.
- [ ] Engine for the kind: authorization before candidate selection, body index per version and public-field index with isolated statistics, exact dense scan, caches partitioned by release and access set, `no-store` responses; `corpus: "frameworks"` on `corpus_status`, `search`, `read`, `quality_gaps`; new tool `mappings`; guide text; reduced records.
- [ ] Invariant suite with synthetic frameworks: changing inaccessible text changes nothing observable (hits, order, scores, abstention, counts, cursors) for reduced and mixed access sets, across cache hits and pagination; it gates signing.
- [ ] `/api/v1/frameworks` routes, OpenAPI, website pages and the corpus switch on search; responsive sweep covers them.

## 15.5 Release (deploy; dev, primary)
- [ ] Corpus kind in release identity: manifest `corpus_kind`, per-kind sequence history, pointer, journal and retired list on the host, first-release bootstrap with `previous: null`, release-check per kind, image then release floor then first release; signing job persists nothing; memory measured with both corpora active; runbook.
- [ ] Owner entitlement created on the host; feature flag on; acceptance with an entitled and a non-entitled account.

## 15.6 Downstream contract (corpus and server; dev, dev2)
- [ ] Reduced export command and its manifest; a test that no licensed field or body-derived value is in it.
- [ ] Change list per release by identity (controls and edges added, removed, changed, superseded), written by build next to the report.
- [ ] Links from controls to legal provisions as reviewed mapping edges with source and evidence (uses the reviewed-knowledge inputs of the knowledge loop design).
- [ ] sc365 reads the export and the change list; record its feedback before freezing the shapes.

## 15.7 Later
- [ ] Licence terms review per publisher and the `free` list.
- [ ] Admin portal behind Cloudflare Access.
