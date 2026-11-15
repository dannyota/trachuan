# Control frameworks

Decision: [ADR 0016](../adr/0016-control-frameworks-corpus.md). Evidence:
[compliary survey](../research/compliary-survey-2026-11.md). This note owns the
framework corpus kind: its sources, access rule, data model, pipeline, serving
and release. Law stays in the notes it has today.

## Terms

- **Framework**: a published set of controls or requirements (ISO/IEC 27001, PCI
  DSS, NIST SP 800-53, TCVN 11930). A **framework version** is one edition
  (`pci-dss` `4.0.1`). An amendment is a version that amends another.
- **Control**: the citation unit, a node in a version's tree with a citation
  (`A.5.1`, `8.3.1`, `AC-2(1)`).
- **Mapping edge**: a stated relation between two controls of different
  framework versions, with its source (publisher catalog, NIST OLIR, CIS
  workbook, structural, reviewed by us).

## Sources

`sources/frameworks.yaml` is the registry, in Git: one entry per framework
version with publisher, code, version, language, `access` (`free` or
`licensed`), licence kind as the publisher states it, the source files with
their SHA-256 and role (text, catalog, mapping workbook), and the parser. It
holds no document text. Seeds come from compliary's `framework.csv`,
`framework_version.csv` and `file_rule.csv`.

The files themselves are read from a local directory named by
`FRAMEWORKS_SOURCE_DIR` (the owner's clone of `compliary-private`). A file whose
hash is not in the registry is refused. Nothing in CI or on the host reads that
directory.

An entry without `access`, or with a value outside the two, fails registry
validation. `access: free` needs `licence_kind` of `public-domain` or an open
licence and a `licence_url`; the validator refuses `free` otherwise.

## Access

### Classes and grants

Every framework version carries `access` from the registry into the release.
Auth database table `framework_grant`:

| Column                     | Meaning                                                               |
| -------------------------- | --------------------------------------------------------------------- |
| `id`                       | key                                                                   |
| `email`                    | canonical account email ([accounts](accounts.md) parser), the grantee |
| `scope`                    | `*` for every licensed framework, or one framework code               |
| `granted_at`, `granted_by` | time and operator name                                                |
| `note`                     | free text, operator only                                              |
| `revoked_at`, `revoked_by` | null while live                                                       |

One live row per `(email, scope)`. A grant applies to the account whose verified
email equals `email`; a grant may exist before the account does. Deleting an
account does not delete grants; they bind again only to an account that verifies
the same email.

`server admin frameworks grant|revoke|list` writes and reads the table and
appends each change to `framework_grant_event` (who, when, what, never document
text). There is no other writer until the admin portal exists.

### Decision function

One function decides, for an authenticated account and a framework version:

1. `free`: full.
2. `licensed` and a live grant with scope `*` or this framework's code: full.
3. Otherwise: reduced.

It reads grants through the same store as session checks, so a revocation takes
effect within the revocation delay in [web API](web-api.md). Search, read,
mapping and the web API call this function and no other path builds a control
record. A missing class, an unknown framework or a store error is `reduced`,
never `full`; a store error is also reported as unavailable.

### Reduced record

A reduced control carries: framework code and version, citation, kind, tree
position, trachuan's paraphrased title, status, amendment and lineage links,
mapping edges, and `access: "licensed"` with `text_withheld: true`. It never
carries the publisher's title, body text, notes, tables or any snippet cut from
them. Paraphrased titles are ours (seeded from compliary's `control_title.csv`,
reviewed to keep the standard terms) and are served to everyone.

### Search

For a caller with full access to a version, search runs as for law: dense and
lexical arms over control text. For a caller with reduced access, that version's
controls are candidates only through the public fields (citation, paraphrased
title): body text is neither embedded into the query path nor matched lexically
for that caller, so result order cannot reveal it. The engine therefore keeps
two indexes per release: body (full callers) and public fields (everyone).
Citation lookups (`PCI DSS 8.3.1`) resolve for everyone.

### What never happens

Licensed text is not written to logs, metrics, quality gaps, error reports,
evaluation reports or cache keys. Response caches are keyed by the decision
(`full` or `reduced`) as well as the request. Tests and fixtures use invented
frameworks.

## Data model

A release database `frameworks_<rel>` (schema version 1):

- `release`: singleton with schema version, recipes and counts.
- `framework`, `framework_version`: registry fields, `access`, lineage
  (`supersedes`, `amends`).
- `source_file`: role, SHA-256, licence kind, text source (`structured`,
  `pdf_text`, `ocr`), completeness.
- `control`: version, parent, ordinal, `citation`, `citation_norm`, kind,
  `title` (ours), `title_original`, `body`, status.
- `control_mapping`: source and target by business key (framework, version,
  citation), relation, mapping source, provenance; a target not yet ingested
  stays unresolved and is counted.
- `chunk`, embeddings and lexical postings, split by index (body, public).
- `quality_gap`: as for law, with kinds for parse counts, unresolved mappings
  and OCR flags.

Public identity is `(framework code, version, citation)`; integer ids are
release-local.

## Pipeline

The stages of [pipeline](pipeline.md) with framework inputs: `discover` reads
the registry, `fetch` copies and hashes files from the source directory,
`extract` runs the entry's parser (OSCAL JSON, XLSX catalog, PDF text, OCR),
`relate` builds mapping edges, then `chunk`, `embed`, `build` and `eval`.
Parsers are deterministic; no model writes canonical text. Each parser has an
exact control-count check against the registry.

Run state for this kind lives in the workshop database like law runs; artifacts
with licensed text stay on the MacBook's external disk and in the private
releases bucket, nowhere else.

### Scans

A PDF with no text layer (today `tcvn-14423-2025.pdf`, 48 pages) goes through
[OCR](ocr-check.md): Vision OCR, the model cross-check, flags and reviewed
waivers. Its text source is `ocr`, article-level labels are asserted, and the
gap rules of the law corpus apply. compliary's hand-made `.txt` is not used as
text; it may serve as a comparison when reviewing flags.

## Serving

- **MCP**: `corpus_status`, `search`, `read` and `quality_gaps` take
  `corpus: "frameworks"` in place of `country`; the result shapes follow
  [MCP results](mcp-results.md) with control records. One new tool, `mappings`,
  returns the edges of a control or between two framework versions. `guide`
  explains the two corpora and the reduced record.
- **Web API and website**: `/api/v1/frameworks/...` and pages `/frameworks`,
  `/frameworks/{code}/{version}` and a control page; the search page gains the
  corpus switch. A reduced control shows our title, its mappings and a plain
  statement that the publisher's text is licensed.
- Nothing about frameworks is public without a session.

## Evaluation

compliary's 205 golden cases are the starting cohort, re-adjudicated against
this release. Floors are set from the first measured run and recorded here;
compliary measured recall@8 of 86.3% without a framework filter and 93.2% with
one. Evaluation runs with full access; a second pass with reduced access checks
that no licensed field appears in any result.

## Release and deployment

The framework release follows [corpus release](corpus-release.md) with channel
`frameworks-production`: upload to the private bucket, signed manifest (no
text), host activation as its own database. The server image must support the
kind before a release is signed. Until then the feature is off:
`SERVER_FRAMEWORKS=false` hides the tools, routes and pages.

## Admin portal (later)

A separate hostname behind Cloudflare Access. The server accepts admin routes
only on that listener and only with a valid Access token (audience and team
domain from configuration). First scope: grants list, grant, revoke, events.

## Open

- Which frameworks are `free`: the terms review per publisher.
- Links from controls to legal provisions.
- Whether the body index for licensed versions needs its own memory budget on
  the host; measured with the first release.
