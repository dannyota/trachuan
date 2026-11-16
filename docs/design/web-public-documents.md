# Public document summaries

Decision: [ADR 0005](../adr/0005-google-login-and-embedded-oauth.md) (History
2026-11-16). Rows, endpoints, capacity and acceptance:
[serving public summaries](web-public-documents-serving.md). Routes and caching:
[web](web.md); indexing: [discovery](discovery.md).

Reading the full text needs an account; a document's identity, status,
structural outline and approved chapter and article titles are public, at a URL
of their own.

## Two URLs

| URL                           | Session                                                                             | Cache          | Robots                                                       |
| ----------------------------- | ----------------------------------------------------------------------------------- | -------------- | ------------------------------------------------------------ |
| `/vn/documents/{key}/summary` | none: no session call of any kind; every request is made with `credentials: 'omit'` | public (below) | indexable when `index` is true, else `X-Robots-Tag: noindex` |
| `/vn/documents/{key}`         | the reader, unchanged                                                               | `no-store`     | signed out gets a 302 to `/login`                            |

The summary is an independent route record,
`app/pages/vn/document-summary/[key].vue` with
`definePageMeta({ path: '/vn/documents/:key/summary' })`, never a child of the
reader's `[key].vue`. Its own layout has a session-independent header (logo,
navigation, language and a plain "Đăng nhập" / "Sign in" link): no account
probe, no `loadSession()`, no session store. Its transport is `openapi-fetch`
with `credentials: 'omit'` and no Cookie header on the server, and it calls only
`GET /api/v1/public/documents/{key}` and `GET /api/v1/public/status`.

"Đọc toàn văn" / "Read the full text" links to the reader, which sends a
signed-out visitor through `/login`. Public topic pages and summary relations
link to summary paths while summaries are available, else to the reader.

## What a summary shows

| Shown                                    | Notes                                                                                                                                                                               |
| ---------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Number, title, type, issuer              | trimmed and white space collapsed ([MCP results](mcp-results.md)); title in Noto Serif                                                                                              |
| Issue date, effective from, effective to |                                                                                                                                                                                     |
| Status stamp, basis and `status_date`    | status evaluated per request for the Vietnam civil date; basis labels as the reader's                                                                                               |
| Unresolved validity                      | typed `unresolved` from `validity_conflict`; fixed label "Hiệu lực chưa xác định: các nguồn mâu thuẫn" / "Validity unresolved: the sources disagree"                                |
| Validity gaps                            | closed enum `missing_effective_date`, `consolidated_status_unordered`, `consolidated_status_unresolved`, with the site's fixed labels                                               |
| Text availability                        | complete text: "Đọc toàn văn"; incomplete: the fixed notice "Nội dung văn bản chưa đầy đủ." and "Đọc toàn văn"; no text: "Tra Chuẩn chưa có nội dung văn bản này." and no read link |
| Relations                                | up to 50 document-level pointers (kind, direction, number, title when in the release) and their total; a target with a summary links to it                                          |
| Topics                                   | up to 10 nodes with code and name                                                                                                                                                   |
| Outline                                  | labels, and approved chapter and article titles (below)                                                                                                                             |
| Source link                              | only when https                                                                                                                                                                     |

Never shown: a heading outside the rule below, provision, lead and annex text,
parts, snippets, provision-level warnings, the validity history and its quotes,
free-form gap messages, files and hashes.

## Outline and headings

The outline lists nodes of kind `phan`, `chuong`, `muc` and `dieu` with their
labels and order. A heading is attached only to `chuong` and `dieu`, and only
when all of these hold; otherwise the entry shows its label alone:

1. The document's `text_source` is `source_tree`.
2. The document has no structural gap, whichever stage recorded it: every kind
   in the site's structure category (`ambiguous_tree_text`,
   `article_number_out_of_range`, `article_sequence`, `structure_uncertain`,
   `text_number_mismatch`, `tree_headings_only`, `tree_headings_partial`,
   `tree_text_partial`, `unknown_node_kind`, `invalid_ancestry`), `no_tree`,
   `text_source_ambiguous`, and any structure kind added later. The allowlist of
   harmless kinds is empty; adding a kind needs a measured sample and an
   amendment.
3. The heading is at most 300 characters; a longer one is omitted, not cut.
4. The heading is not in the exclusion list.
5. The active release carries a matching heading approval.

`phan` and `muc` entries always show labels only. Rules 1 to 4 decide the
candidate headings that the build stores; rule 5 decides whether any are served.

### Exclusions, scan and approval

- **Exclusion list.** `sources/vn-heading-exclusions.yaml`, closed schema
  `{key, anchor, reason, reviewed}`. It is a build input, pinned and hashed into
  the build input hash like corrections. An entry outside the release is a build
  error.
- **Scan.** After the build, `corpus heading-scan <rel>` reads the stored
  candidate headings (rules 1 to 4, ignoring rule 5) and lists every one that is
  over 200 characters, ends with a sentence-final mark other than its own, or
  contains "phải", "được", "không được" or "có trách nhiệm". Its output is kept
  beside the build record with its SHA-256. The scan is triage, not proof: a
  source title can still carry operative text it does not flag, a residual risk
  the owner accepted on 2026-11-16.
- **Review record.** `sources/vn-heading-reviews.yaml`, closed schema
  `{release, projection, scan_sha256, exclusions_sha256, reviewer, date}`. The
  reviewer named in the entry writes it after reading that release's scan
  output; a rejected heading goes into the exclusion list, which forces a
  rebuild and a new scan.
- **Serving.** The publish step copies the matching review entry into the signed
  release manifest ([corpus release](corpus-release.md)). Activation records
  `headings_approved` for the release only when the entry's release, projection,
  scan hash and exclusion hash all equal the release's own. The public endpoint
  returns headings only when it is set. A release without it, rollback targets
  included, serves labels only.

## Availability and errors

Summaries are available only when the operator flag `SERVER_PUBLIC_SUMMARIES` is
on and the active release has the summary capability (its `document_summary`
table at the expected projection). `GET /api/v1/public/status` reports the
result as `features.public_summaries`.

| Situation                                                   | Public endpoints                          | Summary paths        | Topic links   | Sitemap documents child |
| ----------------------------------------------------------- | ----------------------------------------- | -------------------- | ------------- | ----------------------- |
| available                                                   | 200, or 404 for a key outside the release | the summary          | summary paths | present                 |
| flag off, or an active release known to lack the capability | 404                                       | the fixed 404        | the reader    | absent                  |
| no active release, or release resolution failed             | 503 `unavailable`                         | the unavailable page | the reader    | uncached 503            |

The reader is unaffected in every case.

## Caching and freshness

- **Summary pages.** Nitro cache keyed by path and language, life 300 seconds
  and never past the next Vietnam midnight, `swr: false`; HTTP `no-cache` with
  ETag and `Vary: Cookie`, like every public page. A rendered page over 256 KiB
  is served but not stored; the summary cache holds at most 32 MiB.
- **Render slots.** Summary renders take at most 2 of the 4 concurrent renders;
  the other 2 serve all other pages.
- **Sitemap.** `/sitemap.xml` and its children use a Nitro life of 300 seconds
  with HTTP `no-cache`.
- **Promise.** A response newly served by Nitro or the API reflects the current
  release, flag and status date within 10 minutes: the Nitro life (300 s) plus
  the API's `max-age` (at most 300 s) with `must-revalidate`, which forbids
  stale reuse. Topic pages revalidate in the browser (`no-cache`) and live 600
  seconds in Nitro, which reads status on the server at render without an HTTP
  cache, so after a change of availability their links reach the new state
  within 10 minutes. Pages already displayed or restored from the back-forward
  cache are not withdrawn; the promise covers new responses only.

## Head tags and JSON-LD

- Canonical `https://trachuan.vn/vn/documents/{key}/summary`; title
  `{type} {number} · Tra Chuẩn`, or the number alone over 60 characters;
  description the normalized title cut on a word boundary under 160.
- JSON-LD `Legislation`: `url`, name, `legislationIdentifier`,
  `legislationType`, `legislationDate`, `legislationPassedBy`,
  `legislationJurisdiction` VN, `inLanguage` vi, `sameAs` the source URL when
  published; no `legislationLegalForce` and no relation properties. It goes
  through the discovery serializer, which replaces every `<` with the six
  characters `\u003c`.

## Sitemap

While summaries are available, `/sitemap.xml` is an index over
`/sitemap-pages.xml` and `/sitemap-documents.xml`. The documents child comes
from one call to `GET /api/v1/public/documents`, lists the summary path of every
key with `index: true`, and requires the list's `release` to equal status's;
anything else is an uncached 503. `lastmod` is omitted until the generator reads
the release evidence diff.
