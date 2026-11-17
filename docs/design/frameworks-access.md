# Control frameworks: access

Decision: [ADR 0016](../adr/0016-control-frameworks-corpus.md). This note owns
who may read licensed framework text, the invariant that protects it, and where
licensed material may exist. The corpus kind itself is in
[control frameworks](frameworks.md).

## Classes and entitlements

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
updated or deleted. A database trigger rejects updates, deletes and truncation
of events, which guards against defects and operator mistakes. It does not stop
a compromised server: the server's database role owns the auth tables and could
disable the trigger, but that server is also the access check and could serve
licensed text directly, so separate roles would protect only the history.

After an auth database restore ([backups](backups.md#production-restore)), every
live entitlement is revoked with reason `restore` before traffic resumes,
because the backup may predate a revocation. The operator allows again from the
event history.

## Decision

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

## Reduced record

A reduced control carries: framework code and version, citation, kind, tree
position, trachuan's paraphrased title, status, amendment and lineage links,
mapping edges, and `access: "licensed"` with `text_withheld: true`. It never
carries the publisher's title, body text, notes, tables or any snippet cut from
them. Paraphrased titles are ours (seeded from compliary's `control_title.csv`,
reviewed to keep the standard terms) and are served to everyone. The approved
public fields of a licensed version are exactly: citation, kind, tree position,
our title, status, lineage and mapping edges.

## The invariant

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
  and document caches); the per-version index partitions a release loads for
  every version when it opens are not such a cache
  ([results](frameworks-results.md#format)).
- **Framework responses are `no-store`**, API and HTML, and are excluded from
  the Nitro and CDN public caches.
- Citation lookups (`PCI DSS 8.3.1`) resolve for everyone through public fields.
- **Diagnostics and counts obey it too.** For a version the caller may not read,
  `quality_gaps`, `corpus_status` and every count are built only from the
  approved public fields (controls, edges, lineage). A gap that depends on text
  (parsing, OCR flags, completeness) is shown only to callers who may read that
  version.

## Licensed material and where it may be

Licensed material is a source file of a `licensed` version and everything
derived from its text: extracted and OCR text, check results, reviewer readings,
chunks, embeddings, indexes and dumps. It may exist only in:

| Place                                                                                                                                         | Why                                                                              |
| --------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------- |
| `FRAMEWORKS_SOURCE_DIR` and `FRAMEWORKS_WORK_DIR` on a local disk of the MacBook, never under a third-party sync folder                       | sources and run artifacts ([build](frameworks-build.md))                         |
| The MacBook's PostgreSQL instance on its internal disk, holding `frameworks_<rel>` and `frameworks_<rel>_eval` (tables, WAL, temporary files) | the release while it is built and evaluated                                      |
| Memory of the build command, its parsers, the evaluator and the document embedder on the MacBook                                              | during a run; temporary files go under `FRAMEWORKS_WORK_DIR`, core dumps are off |
| The private releases bucket `trachuan-frameworks-releases` ([release](frameworks-release.md#bucket-and-keys))                                 | the dump                                                                         |
| The signing runner, for the length of one job                                                                                                 | hash check; nothing persisted                                                    |
| The host's release database and its encrypted backups                                                                                         | serving                                                                          |
| Host staging `/srv/trachuan/data/releases/<kind>-<rel>/`                                                                                      | the dump during activation; owner-only, deleted when activation ends             |
| The server process's memory                                                                                                                   | indexes and partitioned caches; never written to disk                            |
| The OCR and cross-check providers, for scans only                                                                                             | see [scans](frameworks.md#scans)                                                 |
| The owner's own backups of the MacBook                                                                                                        | internal use of the owner's licensed files                                       |

Third-party sync services are not a place:
[build](frameworks-build.md#runs-resume-and-failure) refuses a source or work
directory inside a synced folder. It is never in this repository's Git, CI logs
or artifacts, public buckets, log lines, operational metrics, quality gap
details, error reports, cache keys or PR text. The frameworks evaluation report
and the licensed cohort's floors ([evaluation](frameworks-evaluation.md)) are
licensed derivatives and stay in the places above: the work directory, the
source directory and the private releases bucket. Two value classes are
published although they are computed from licensed files. The integrity fields
of the signed manifest (`dump` SHA-256 and size, `report` SHA-256,
`build_artifact_sha256`) are needed to verify and to refuse replays, and reveal
only that a release differs from another. The SHA-256 and byte size of each
source file in the registry (`sources/frameworks.yaml`) let the reader refuse an
unregistered file, and reveal only which file it is. The number of controls of a
version is an approved public fact, served to every caller
([results](frameworks-results.md#diagnostics-and-counts)), and the registry
holds it for every version. Beyond these and the approved public fields of the
[reduced record](#reduced-record) (with counts of them, such as mapping edges,
that [results](frameworks-results.md#diagnostics-and-counts) serves), no value
derived from a licensed file is published. In Git, the registry holds no page or
character counts, no counts finer than the per-version total (by kind or by
section), no tree shape and no titles taken from a licensed file. Machine OCR
evidence for licensed scans is written under `FRAMEWORKS_WORK_DIR/ocr/`, where
it can be reproduced. Reviewer readings of licensed text (OCR waivers) are
knowledge and are versioned in the private repository, in
`FRAMEWORKS_SOURCE_DIR/trachuan/ocr-waivers.yaml`, a non-source path the reader
ignores as a source; the owner commits them and no worker writes there. A build
reads both only through its snapshot. Tests and fixtures use invented
frameworks.
