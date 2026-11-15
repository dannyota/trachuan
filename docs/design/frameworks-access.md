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
updated or deleted.

After an auth database restore ([backups](backups.md#restore)), every live
entitlement is revoked with reason `restore` before traffic resumes, because the
backup may predate a revocation. The operator allows again from the event
history.

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
  and document caches).
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

| Place                                                    | Why                                                                  |
| -------------------------------------------------------- | -------------------------------------------------------------------- |
| `FRAMEWORKS_SOURCE_DIR` and the MacBook's external disk  | sources and run artifacts                                            |
| The workshop database on the MacBook                     | run state, OCR store                                                 |
| The private releases bucket                              | the dump                                                             |
| The signing runner, for the length of one job            | hash check; nothing persisted                                        |
| The host's release database and its encrypted backups    | serving                                                              |
| Host staging `/srv/trachuan/data/releases/<kind>-<rel>/` | the dump during activation; owner-only, deleted when activation ends |
| The server process's memory                              | indexes and partitioned caches; never written to disk                |
| The OCR and cross-check providers, for scans only        | see [scans](frameworks.md#scans)                                     |

It is never in Git, CI logs or artifacts, public buckets, log lines, metrics,
quality gap details, error reports, evaluation reports, cache keys or PR text.
Reviewer readings of licensed text (OCR waivers) live in a file under
`FRAMEWORKS_SOURCE_DIR`, not in `sources/`. Tests and fixtures use invented
frameworks.
