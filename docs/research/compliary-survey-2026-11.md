# compliary survey, 2026-11-15

Read-only survey of `dannyota/compliary` at `a2e3836` and
`dannyota/compliary-private` at `674b604`, for
[ADR 0016](../adr/0016-control-frameworks-corpus.md). File references are paths
in the compliary repository.

## What compliary was

- Archived on 2026-11-01 (`README.md:5`): an evidence-only corpus of InfoSec
  control frameworks with an MCP server, Go and PostgreSQL 17 with pgvector. It
  ran in production for its maintainer only.
- Ingested: 13 framework versions, 14 documents, 3,943 controls, 4,501 mapping
  edges of which 94.8% resolved (`PLAN.md:201`, `docs/ARCHITECTURE.md:69-76`).
  Registry: `deploy/seed/framework.csv` (17 frameworks), `framework_version.csv`
  (30 versions), `file_rule.csv` (files and source URLs).
- Retrieval: hybrid dense (Qwen3-Embedding-0.6B) and BM25 with RRF, direct
  lookup for citation-shaped queries. 205 golden cases
  (`deploy/eval/golden.csv`): recall@8 86.3% unfiltered, 93.2% with a framework
  filter (`docs/design/RETRIEVAL.md:74-77`).
- No OCR, and no model as the canonical parser (`docs/ARCHITECTURE.md:21,33`).

## Licensing and access there

- Framework classes `public-domain`, `open-restricted`, `licensed`, and a serve
  policy per framework (`sql/config/schema.sql:13-33`). Per file: licence kind
  and a `serve_gate` of `public` or `auth-only` (`sql/bronze/schema.sql:12-33`).
- Public domain: NIST CSF and SP 800-53. Open with restrictions: CIS Controls
  (CC BY-NC-ND), CSA CCM. Licensed: ISO, AICPA criteria, PCI DSS, COBIT, SWIFT,
  TCVN (`deploy/seed/framework.csv`). Terms were read on 2026-07-19
  (`PLAN.md:67-79`).
- Titles of licensed frameworks were paraphrased so they could be shown
  (`docs/design/SCHEMA.md:99,244-246`; 2,125 titles in
  `deploy/seed/control_title.csv`).
- No allowlist, entitlement or tenant model: one operator password, and every
  authenticated principal was permitted (`pkg/oauth/store_types.go:21-41`).
- The stored `serve_gate` was not used when serving: the code blanked text for
  every document by caller, public-domain ones included
  (`pkg/mcp/core.go:139-171`).

## Lessons recorded there

- The dense arm was silent in production for weeks because of a model-name
  mismatch (`docs/HISTORY.md:173-186`).
- Re-normalizing deleted curated mapping edges through cascades
  (`PLAN.md:204-226`).
- Paraphrased titles first hurt search by dropping the standard's terms
  (`docs/HISTORY.md:146-154`).
- A test fixture used real ISO headings, caught in review
  (`docs/HISTORY.md:68-69`).

## The private documents

33 files, 84 MB: 20 PDF, 10 XLSX, one OSCAL JSON, one text file, in `nist`,
`pcissc`, `cis`, `iso`, `aicpa`, `isaca`, `csa`, `tcvn`. Its README states the
rule: official publisher sources only, internal use, never mirrored to a public
repository, bucket or unauthenticated endpoint.

Text layer, measured with `pdftotext` on 2026-11-15:

| PDF                                             | Pages | Characters | Text layer |
| ----------------------------------------------- | ----- | ---------- | ---------- |
| aicpa-tsc-2017-points-of-focus-2022             | 75    | 142,969    | yes        |
| cis-controls-guide-v8.1.2                       | 146   | 230,339    | yes        |
| csa-ccm-v4.1-implementation-guidelines          | 421   | 979,400    | yes        |
| csa (four smaller guides)                       | 93    | 170,424    | yes        |
| cobit-2019 governance and management objectives | 302   | 656,125    | yes        |
| cobit-2019 introduction and methodology         | 64    | 125,717    | yes        |
| iso-iec-27001-2022 and Amd 1:2024               | 32    | 52,555     | yes        |
| iso-iec-27002-2022                              | 164   | 378,943    | yes        |
| iso-iec-27017-2015, 27018-2019                  | 78    | 177,351    | yes        |
| iso-22301-2019 Amd 1:2024                       | 6     | 3,540      | yes        |
| nist-csf-2.0                                    | 32    | 58,878     | yes        |
| nist-sp-800-53r5                                | 492   | 1,351,530  | yes        |
| pci-dss-v4.0.1                                  | 397   | 679,928    | yes        |
| tcvn-11930-2017                                 | 73    | 99,301     | yes        |
| tcvn-14423-2025                                 | 48    | 0          | no: a scan |

One PDF needs OCR. A hand-made text file sits beside it. The ISO 22301:2019 base
text was removed from the repository earlier as a scan.
