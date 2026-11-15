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

[Control frameworks: access](frameworks-access.md) owns the access classes,
entitlements, the reduced record, the invariant and the places licensed material
may exist. In short: a `licensed` version's text is served only to an email with
an entitlement; everyone else gets a reduced record; and text a caller may not
read cannot change anything that caller observes.

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

Run state and artifacts follow
[licensed material](frameworks-access.md#licensed-material-and-where-it-may-be).

### Scans

A PDF with no text layer (today `tcvn-14423-2025.pdf`, 48 pages) goes through
[OCR](ocr-check.md): Vision OCR, the model cross-check, flags and reviewed
waivers. [ADR 0014](../adr/0014-ocr-on-the-write-path.md) lets only public legal
text reach the two providers; ADR 0016 adds the exception for framework scans,
on the owner's decision. Before the first licensed scan is sent, the registry
entry records each provider's data-use terms (URL and date read: no training on
the content, retention period); without that record the scan stays blocked with
gap `ocr_blocked`. Its text source is `ocr`, labels are asserted at the level
the OCR design allows, and the law corpus's gap rules apply. compliary's
hand-made `.txt` is not used as text; it may serve as a comparison when
reviewing flags.

## Serving

Framework tools and routes need a session or token; nothing is anonymous. The
field-level contract is its own note, `frameworks-results.md`, written and
reviewed before serving work starts. Its fixed rules:

- **Inputs.** `corpus_status`, `search`, `read` and `quality_gaps` take
  `corpus: "frameworks"` in place of `country`, with filters by framework code
  and version. The query gate gains framework citation schemes, so a citation
  routes to a direct lookup. One new tool, `mappings`, returns the edges of a
  control or between two framework versions. `guide` explains both corpora and
  the reduced record.
- **Handles.** `fw:<code>:<version>#<citation>`; law handles and framework
  handles never mix in one read.
- **Outputs.** Control records, not law records: no law validity stamp, no
  ancestor headings from text. Every nested object has a reduced form: a mapping
  edge end is citation, version and our title; provenance is the mapping
  source's name; an outline is citations and our titles.
- **Diagnostics and counts.** For a version the caller may not read, gaps and
  counts come only from approved public fields; text-dependent gaps are withheld
  ([invariant](frameworks-access.md#the-invariant)). A gap detail never quotes
  text for anyone.
- **Web.** `/api/v1/frameworks/...`, pages `/frameworks`,
  `/frameworks/{code}/{version}` and a control page; the search page gains the
  corpus switch. A reduced control shows our title, its mappings and a plain
  statement that the publisher's text is licensed.

## Evaluation

compliary's 205 golden cases are the starting cohort, re-adjudicated against
this release. Floors are set from the first measured run and recorded here;
compliary measured recall@8 of 86.3% without a framework filter and 93.2% with
one. Evaluation runs with full access. A second suite runs the
[invariant](frameworks-access.md#the-invariant) with reduced and mixed access
sets; a release that fails it is not signed.

## Release and deployment

The corpus kind is part of release identity everywhere, not a channel name:

- **Manifest.** The signed manifest is public like the law manifest; that is the
  one exception to "nothing anonymous". Its complete field list: `kind`,
  `version`, `corpus_kind: "frameworks"`, `repository`, `commit`, `rel`,
  `database` (`frameworks_<rel>`), `sequence`, `previous`, `dump` (object,
  SHA-256, size), `report` (SHA-256, `quality_pass`), `build_artifact_sha256`,
  `schema_version`, `extensions`, `recipes` (parameters only) and the number of
  framework versions and controls. It holds no text and no citations. The
  integrity fields are the one body-derived class that is published
  ([access](frameworks-access.md#licensed-material-and-where-it-may-be));
  nothing else computed from body text is (vocabulary sizes, frequencies,
  lengths).
- **Channel and sequences.** `ghcr.io/dannyota/trachuan-corpus-release` tag
  `frameworks-production`, with its own sequence history, pointer row, journal
  and retired list on the host, keyed by kind; one kind's pending or failed
  state never blocks or supersedes the other's.
- **First release.** A kind with no active release accepts a manifest with
  `previous: null` once; evaluation then has no previous report and the
  non-regression gates start with the second release.
- **Compatibility.** `server release-check` reports supported schema versions
  per kind and refuses a kind the image does not serve. The first image that
  serves the kind is deployed, then the release floor is raised to it, then the
  first release is signed ([corpus release](corpus-release.md)).
- **Memory.** The host limit is measured with both corpora active before the
  flag is turned on ([host budgets](budgets-host.md)).

Until the first release is active the feature is off: `SERVER_FRAMEWORKS=false`
hides the tools, routes and pages.

## Downstream products

The owner's other products (sc365 first) build private knowledge on top of this
corpus: mappings from an organization's assets to the requirements that apply.
Those mappings stay downstream. What this corpus gives them:

- **Identity.** `(framework code, version, citation)` for a control and
  `(country, document key, anchor)` for a legal provision. A downstream mapping
  stores these keys, never text.
- **A reduced export.** The public fields of every version, the full text of
  `free` versions, mapping edges and lineage, with the release's signed
  manifest. It holds no licensed text, so it may leave the host and the MacBook.
  The full dump has no hand-over path.
- **A change list per release**, by identity: controls added, removed, changed
  (public fields or, for `free` versions, text), superseded or amended, and
  edges added or removed, so a downstream mapping knows what to review.
- **Links from controls to legal provisions**, as reviewed mapping edges with a
  source and evidence, once the reviewed-knowledge inputs of the corpus carry
  them.

The export and the change list are built only from the approved public
projection and permitted free text, applied to every nested value (mapping
evidence and provenance included). A field with no classification is refused. A
change to licensed body text alone leaves both artifacts byte-identical apart
from the manifest's integrity fields; a test holds this.

## Admin portal (later)

A separate hostname behind Cloudflare Access. The server accepts admin routes
only on that listener and only with a valid Access token (audience and team
domain from configuration). First scope: entitlements list, allow, revoke,
events.

## Open

- Which frameworks are `free`: the terms review per publisher.
- Whether the body index for licensed versions needs its own memory budget on
  the host; measured with the first release.
