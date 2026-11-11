# Fetch bundles and observation evidence

Proposed execution refinement under
[ADR 0012](../adr/0012-vietnam-corpus-sources.md). Keep fetch content
independent of collection time. Retain the existing VBPL format; use version 2
envelopes for chinhphu. [Data](data.md#scope-workshop-contracts) owns workshop
fields; this note owns bundle bytes and observation handling.

## Byte stability

For identical source bytes, attachment outcomes and recipe, every bundle file
and `MANIFEST.sha256` must be byte-identical. Never serialize a worker clock,
fetch/adoption time, run id, generation, attempt, cache-hit flag, local path,
request duration or HTTP `Date` header into a bundle. Preserve dates supplied as
document content. Changed source bytes or attachment availability may still
change the hash, even when legal text is unchanged.

Keep the existing VBPL serialization unchanged, except for the view counter that
[fetch reuse](fetch-reuse.md#stored-detail-drops-the-view-counter) removes from
the stored detail. For new envelopes use compact UTF-8 JSON from fixed-field
structs, explicit nulls and empty arrays, with no trailing newline. Pin field
order and escaping in the fetch recipe. Sort file entries bytewise by
`(name,url,sha256)` and evidence references by `(path,sha256)`; deduplicate
exact entries. Preserve raw field occurrences in source order. Go's
[JSON encoder](https://pkg.go.dev/encoding/json#Marshal) defines escaping and
array encoding; fixture byte comparisons must pin the chosen representation.

Redact secrets before hashing HTML under the
[client prerequisites](#client-prerequisites). Never persist cookies or headers
carrying secrets. Changes to the redactor or serialization require a recipe
change. Retain other source HTML, including source-supplied dates; this is byte
stability for equal redacted source content, not semantic HTML deduplication.

Reason: [chunk](chunk.md#stage-and-artifact) binds `fetch_sha256` both in its
input and output. [Embed](embedder.md#embed-stage) keys reuse by the entire
chunk manifest, with no separate text cache. A timestamp alone would re-embed
every affected document despite unchanged text.

## Bundle formats

VBPL keeps `detail.json`, `tree.json`, `diagram.json`, the unversioned
`files.json` array of `{name,sha256,size}`, `report.json` with `missing` and
`too_large` name arrays, and attachments at `files/<sha256>`. Do not add
`document.json`, wrap its file array or re-encode its raw JSON on scope
adoption. The common reader decodes the known VBPL format into the projections
in data; absent URL and role stay null and `unknown`. An attachment path follows
its verified hash. Existing metadata and identity come from `detail.json`.

Chinhphu uses `{version:2,document:{...}}` in `document.json` and
`{version:2,files:[...]}` in `files.json`. Document fields are the common
projection in data. File fields are that projection's content fields only:
`source`, `name`, `sha256`, `size`, `path`, `url`, `role`. There is no
`fetched_at` field, including no null placeholder. All entries have source
`chinhphu`; document key, country, source and source id must agree with the
planned item. No adapter fabricates VBPL payloads for another source.

Dispatch readers by the expected source and recognized format, not by guessing
from a JSON error. Reject unknown versions, conflicting identities and mixed
formats. Old artifacts remain immutable; old runs retain compatible readers.

### Secondary contents

Every successful secondary bundle contains:

| Path              | Content                                                                                                         |
| ----------------- | --------------------------------------------------------------------------------------------------------------- |
| `detail.html`     | Token-redacted detail response used for parsing.                                                                |
| `metadata.json`   | Version 1: source id, canonical URL, raw field occurrences and complete linked attachment inventory.            |
| `document.json`   | Version 2 common projection, with identity references to `metadata.json` and `detail.html` by path and SHA-256. |
| `files.json`      | Version 2 entries for every successfully downloaded PDF.                                                        |
| `files/<sha256>`  | Exact downloaded PDF bytes; identical bytes share one path.                                                     |
| `report.json`     | Version 1: `missing` and `too_large` arrays of `{name,url}`; empty arrays are explicit.                         |
| `MANIFEST.sha256` | Sorted manifest covering all files above except itself.                                                         |

`metadata.json` is `{version:1,source_id,source_url,occurrences,attachments}`.
Occurrences retain `{name,value,cells,ambiguous}` from the detail parser in
source order; absent values use null and absent cells use `[]`. `cells` retains
structurally ambiguous rows. Apply
[repeated-field rules](secondary-discovery.md#central-issuer-evidence) before
constructing the common identity; never trust a last-value map. Attachments
carry `{name,label,url}`: original filename from the linked URL, raw displayed
link label and validated absolute URL. Sort by `(name,url,label)` and
deduplicate exact entries. Keep different URLs even when their bytes match.
Every inventory `(name,url)` must occur in either files or one report outcome,
never silently disappear. Failed downloads never produce a partial PDF file.
Other failures follow [source execution](source-execution.md); do not label
timeouts, invalid PDFs or access denials as missing attachments.

For identical `(name,sha256)` under different URLs, workshop and corpus file
rows use the byte-smallest URL and its matching observation. Keep every alias in
the bundle inventory and file entries. This avoids duplicate corpus keys without
losing the alternative provenance evidence.

Use the validated detail request's canonical URL with `pageid=27160` and the
requested decimal `docid`; preserve it in `source_url`. No cookies, tracking
parameters or postback tokens belong in that URL. Validate returned detail
against the requested identity; contradictory evidence remains a source gap.
Unknown status, effective dates and roles remain explicit. `has_content` means
usable legal text, not merely a linked PDF; secondary bundles have no tree.

### Client prerequisites

The chinhphu client needs two changes before secondary fetch:

1. Parse form fields and replace each token field's value with a placeholder
   determined only by its field name. Do not globally replace remembered token
   strings. Output must be independent of token values, lengths, overlap, shared
   values, earlier requests and map iteration order. Preserve non-token content
   even when it contains text equal to a token value. This prevents request
   history and replacement order from changing bundle hashes.
2. Expose attachment links losslessly as raw label text, URL-derived filename
   and validated URL, separately. Do not trim or collapse label whitespace or
   use a display label as the filename. This preserves the inventory's source
   evidence instead of requiring the stage to reconstruct discarded text.

Client tests must cover overlapping token strings, shared values across token
fields, changed token values and lengths, identical pages after different prior
requests, and label whitespace preserved alongside the filename and URL. Compare
redacted bytes exactly and prove non-token text remains unchanged.

The stage separates `FileInfo.FetchedAt` from content before serialization; hash
and size stay in the bundle. Parsed field occurrences remain required. Request
safety, limits and the distinction between PDF signature bytes and cryptographic
signature verification remain in [sources](sources.md#fetch-and-safety). No live
source behavior is newly established by this contract; the
[first-party entry points](sources.md#evidence-and-boundaries) and recorded
fixtures remain its source evidence.

## Observation commit and recovery

`Process` returns transient observations beside the artifact reference. Record
only source payloads retained in the bundle: VBPL detail, tree and diagram
responses; chinhphu redacted detail HTML; and downloaded attachments. Bind each
observation to its stored content hash, size and path; HTML hashes cover
redacted bytes. Unstored responses have no observation, including VBPL's
attachment-list response that only feeds generated `files.json`. Generated JSON
envelopes have no download time of their own. Record a cache or artifact
adoption as such, never as a network download. Reusing an attachment requires
the existing [cache-validity rule](pipeline.md#source-ingest-rules); hash
equality alone does not authorize skipping a fresh source read.

The fenced item transaction writes `run_fetch_observation` only when writing a
new committed item result with a retained artifact, even when its directory and
artifact row already exist. Follow the shared
[terminal replay fence](pipeline.md#items-artifacts-and-gaps) before any writes:
an identical same-attempt replay writes no observations; conflicting evidence,
including different timestamps, rolls back without changing the committed
result. A later source pause cannot replace that terminal result. For a new
result, a pause recheck that commits failed `source_paused` writes no
observations. New successful attempts append evidence. Use the same
all-or-nothing
[PostgreSQL transaction](https://www.postgresql.org/docs/current/tutorial-transactions.html)
as the item and derived rows, not a later best-effort write.

`Rows` remains a pure bundle decoder. At first artifact-row insertion, item
commit fills `fetched_file.fetched_at` from the matching observation: the actual
network completion time, or a verified cache/adoption origin time, else null.
Match artifact hash, resource identity, file path, hash and size. For a reused
artifact row, retain that original value, including null; newer downloads belong
only to the run's observations. This field describes the file's recorded origin,
not the run's latest check. Never use artifact `written_at`, filesystem mtime,
commit time, HTTP dates or cache-copy time as a substitute.

Cache/adoption provenance references must identify an exact retained observation
or trusted cache record with matching bytes. A cache eligibility timestamp is
not proof of a download time unless its documented meaning establishes that.
Absent such evidence, store null. Copying an orphan directory without surviving
observations reconstructs content rows but leaves times unknown. A fresh
download before adopting identical bytes supplies a real time for that attempt.

Checkpoint the observation rows with the workshop. Retry invalidation preserves
audit history but detaches superseded observations from the current item.
Uncommitted observations may be lost on a crash; recovery never invents them.
Historical artifacts are not rewritten to fill missing provenance. Unknown times
retain the existing `source_provenance_missing` coverage rule in
[schema](corpus-schema.md). No legal content or earlier release is discarded.

## Hand-over reader

Fetch finalization uses the committed artifact for each item, with its expected
source, key and manifest hash. Verify the manifest and every file used for
comparison before decoding. VBPL identity comes from `detail.json`; its drift
check also reads `diagram.json`. Secondary identity comes from `document.json`
and its hash-linked `metadata.json` and `detail.html`. Check agreement with raw
occurrences using the pinned parser and normalizer; missing or conflicting
fields cannot become a proven match. Do not use PDF text or attachment names as
identity evidence, and do not reread PDFs merely to decide hand-over.

For skipped secondary downloads, read the candidate-specific discovery record
and its bound detail/probe evidence instead. Verify those referenced artifact
and file hashes; require the completeness and identity proof already specified
in [sources](sources.md#secondary-discovery-and-hand-over). A failed fetch
cannot erase a proven discovery match or supply missing identity fields.
Conflicting discovery and fetch identity stays ambiguous; never select the
latest response.

Comparison references identify committed artifact hashes and relative paths,
never mutable workshop metadata or fetch timestamps. Thus observation-only
changes do not alter the
[fetch-close input](source-execution.md#fetch-close-finalization), membership or
`identity_inventory_hash`. Finalizers make no network requests.

## Adoption and cost

Retaining VBPL's format is the proposed choice: no format-only change to its
manifest, chunk bytes or embedding cache key. The rejected alternative, adding
both version 2 envelopes to VBPL, simplifies new readers but changes every
existing fetch manifest once and cascades through chunk and embed. The supplied
current-release estimate is about 98,500 chunks, not a new measurement.

The chosen path still needs a shared dual-format reader, workshop migration,
observation commit plumbing and a build provenance snapshot, specified with the
scope build integration after the index plan merges. The scope fetch recipe
changes, so unchanged bundles may occupy a second recipe directory; extract and
relate recipe changes can require one processing pass. Equal resulting manifest
hashes permit chunk and embed reuse under unchanged recipes. Changed source
content, the already planned normalizer change or a changed document embedding
recipe can still require re-embedding; this proposal promises no blanket reuse
across those.

Observation storage grows with committed attempts and fetched files, without
duplicate PDF bodies in the workshop or a new service. Apply existing request,
file and run byte limits; measure additional database and artifact disk use
before release. Build may change for provenance alone; chunk and embed must not.

Release order: independent design review; shared readers, migrations, the shared
commit fence and client prerequisites; observation writes, secondary fetch and
finalizer integration; build snapshot support; a new scope run with frozen
recipes. Never migrate a frozen run or rewrite published artifacts. No new
source choice, loss policy or budget ceiling is proposed. Owner approval remains
required for the existing unapproved limits and deliberate losses; expanding
this work to invalidate all VBPL bundles would need an explicit owner cost
decision.

Required primary checks: identical content at two injected clock times gives
identical bundle, chunk and embed hashes while recording distinct observations;
cache hit versus fresh download gives identical bytes; token rotation leaves
redacted HTML stable; actual metadata/PDF changes invalidate affected inputs;
legacy, orphan and cache origins never acquire fabricated times; replay, stale
attempt, pause, crash and checkpoint recovery preserve the commit rules. Cover
secondary inventory reconciliation, corrupted identity references, skipped
downloads and contradictory discovery/fetch identity. These are acceptance
requirements, not reported check results.
