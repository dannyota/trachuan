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

### Classes and entitlements

Every framework version carries `access` from the registry into the release. An
**entitlement** lets one email read licensed text. It is not an OAuth grant
([OAuth](oauth.md)) and not account data: it belongs to the email, so account
deletion leaves it and a later account that verifies the same email holds it
again. Auth database table `framework_entitlement`:

| Column                     | Meaning                                                 |
| -------------------------- | ------------------------------------------------------- |
| `id`                       | key                                                     |
| `email`                    | canonical email ([accounts](accounts.md) parser)        |
| `scope`                    | `*` for every licensed framework, or one framework code |
| `granted_at`, `granted_by` | time and operator name                                  |
| `note`                     | free text, operator only                                |
| `revoked_at`, `revoked_by` | null while live                                         |
| `revoke_reason`            | `operator` or `restore`                                 |

One live row per `(email, scope)`. It applies only to an account whose email is
verified and equals `email`; it may exist before the account does.

`server admin frameworks allow|revoke|list` is the only writer until the admin
portal exists. Each change and its row in `framework_entitlement_event` (who,
when, what; never document text) commit in one transaction; events are never
updated or deleted.

After an auth database restore ([backups](backups.md#restore)), every live
entitlement is revoked with reason `restore` before traffic resumes, because the
backup may predate a revocation. The operator allows again from the event
history.

### Decision

One function returns the caller's **access set** for a release: the framework
codes whose licensed text the account may read, or `*`. For a framework version:

1. `free`: full.
2. `licensed` and the framework is in the access set: full.
3. Otherwise: reduced.

The function reads entitlements through the store that checks sessions, so a
revocation takes effect within the revocation delay in [web API](web-api.md),
for sessions, personal tokens and OAuth tokens alike. Search, read, mappings and
the web API call it on every request, before any cache lookup and before a
cursor continues; no other path builds a control record. A missing class, an
unknown framework or a store error is reduced; a store error is also reported as
unavailable.

### Reduced record

A reduced control carries: framework code and version, citation, kind, tree
position, trachuan's paraphrased title, status, amendment and lineage links,
mapping edges, and `access: "licensed"` with `text_withheld: true`. It never
carries the publisher's title, body text, notes, tables or any snippet cut from
them. Paraphrased titles are ours (seeded from compliary's `control_title.csv`,
reviewed to keep the standard terms) and are served to everyone. The approved
public fields of a licensed version are exactly: citation, kind, tree position,
our title, status, lineage and mapping edges.

### The invariant

For any caller, changing text the caller may not read, while the approved public
fields stay the same, changes nothing the caller can observe: hits, order,
scores, abstention, counts, cursors, timing class and errors. Every rule below
follows from it, and a synthetic test proves it for search, read, mappings,
cache hits and pagination with callers of different access sets.

- **Candidates are chosen after authorization.** The body index of a version is
  searched only for callers with full access to it. Everyone else reaches that
  version through the public-field index.
- **Ranking statistics are isolated.** Lexical statistics are kept per framework
  version for the body index and separately for the public-field index, so one
  version's body never weights another's score.
- **Dense search is an exact scan** over the vectors the caller may use; no
  shared approximate graph.
- **Caches are partitioned** by release and the complete access set, for every
  cache that holds authorization-dependent data (response, candidate, read item
  and document caches).
- **Framework responses are `no-store`**, API and HTML, and are excluded from
  the Nitro and CDN public caches.
- Citation lookups (`PCI DSS 8.3.1`) resolve for everyone through public fields.

### Licensed material and where it may be

Licensed material is a source file of a `licensed` version and everything
derived from its text: extracted and OCR text, check results, reviewer readings,
chunks, embeddings, indexes and dumps. It may exist only in:

| Place                                                   | Why                           |
| ------------------------------------------------------- | ----------------------------- |
| `FRAMEWORKS_SOURCE_DIR` and the MacBook's external disk | sources and run artifacts     |
| The workshop database on the MacBook                    | run state, OCR store          |
| The private releases bucket                             | the dump                      |
| The signing runner, for the length of one job           | hash check; nothing persisted |
| The host's release database and its encrypted backups   | serving                       |
| The OCR and cross-check providers, for scans only       | see [Scans](#scans)           |

It is never in Git, CI logs or artifacts, public buckets, log lines, metrics,
quality gap details, error reports, evaluation reports, cache keys or PR text.
Reviewer readings of licensed text (OCR waivers) live in a file under
`FRAMEWORKS_SOURCE_DIR`, not in `sources/`. Tests and fixtures use invented
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

Run state and artifacts follow
[Licensed material](#licensed-material-and-where-it-may-be).

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
  source's name; an outline is citations and our titles; a gap detail is a kind,
  a count and a citation, never text.
- **Counts** (versions, controls, edges, gaps) are release facts and the same
  for every caller.
- **Web.** `/api/v1/frameworks/...`, pages `/frameworks`,
  `/frameworks/{code}/{version}` and a control page; the search page gains the
  corpus switch. A reduced control shows our title, its mappings and a plain
  statement that the publisher's text is licensed.

## Evaluation

compliary's 205 golden cases are the starting cohort, re-adjudicated against
this release. Floors are set from the first measured run and recorded here;
compliary measured recall@8 of 86.3% without a framework filter and 93.2% with
one. Evaluation runs with full access. A second suite runs the
[invariant](#the-invariant) with reduced and mixed access sets; a release that
fails it is not signed.

## Release and deployment

The corpus kind is part of release identity everywhere, not a channel name:

- **Manifest.** `corpus_kind: "frameworks"`, database `frameworks_<rel>`, schema
  version of this kind, recipes and counts. It holds no text and no control
  citations. The signed manifest is public like the law manifest; that is the
  one exception to "nothing anonymous", and it is limited to those fields.
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

## Admin portal (later)

A separate hostname behind Cloudflare Access. The server accepts admin routes
only on that listener and only with a valid Access token (audience and team
domain from configuration). First scope: entitlements list, allow, revoke,
events.

## Open

- Which frameworks are `free`: the terms review per publisher.
- Links from controls to legal provisions.
- Whether the body index for licensed versions needs its own memory budget on
  the host; measured with the first release.
