# Serving public summaries

What a summary shows and when:
[public document summaries](web-public-documents.md). This note owns the
precomputed rows, the two public endpoints, their capacity and the acceptance
tests.

## Summary rows

The corpus build writes one `document_summary` row per document into the release
database ([corpus schema](corpus-schema.md)). A row holds the class,
eligibility, number, title, type, issuer and dates; the status inputs (stored
class, basis, status basis, `eff_from`, `eff_to`, `is_current`,
`validity_conflict`); the validity gap kinds; `has_text`, `text_complete` and
`source_url`; up to 50 relations with their total; up to 10 nodes with names;
the outline with the candidate chapter and article headings that pass rules 1 to
4; and the projection version. Rows are immutable within a release. The table's
presence at the expected projection is the release's summary capability.

Every bound below is enforced at build. A cap on source-derived text is at least
1.5 times the largest value on the release it was measured on, `vn/20261115_3`
on 2026-11-17 (row sizes are SQL estimates of the serialized payload). Over a
cap the build degrades and records a build gap; no bound on source data fails
the build.

| Bound                           | Cap                       | Largest measured                           | Over the cap                                                             |
| ------------------------------- | ------------------------- | ------------------------------------------ | ------------------------------------------------------------------------ |
| number, type, issuer            | 200 characters each       | 51, 28, 44                                 | no row for the document; build gap `summary_field_capped`                |
| title                           | 1,000 characters          | 631                                        | no row for the document; build gap `summary_field_capped`                |
| relation number, relation title | 200 and 1,000 characters  | 46, 631                                    | that pointer is left out, the total keeps it; gap `summary_field_capped` |
| topic name                      | 600 characters            | 386                                        | that node is left out; build gap `summary_field_capped`                  |
| heading                         | 300 characters (rule 3)   | 8,394; 1,388 headings over 300             | the heading is omitted, the label stays                                  |
| relations                       | 50 pointers               | 943                                        | display list; the total keeps the full count                             |
| nodes                           | 10, most provisions first | 35                                         | display list; the rest are not listed                                    |
| outline                         | 2,200 entries             | 1,432                                      | the ordered rule below                                                   |
| serialized row                  | 64 KiB                    | about 167 KB whole, 99 KB with labels only | the ordered rule below                                                   |

The two display lists and the heading are not raised to 1.5 times: the lists are
ranked previews that lose nothing (the full set stays in the reader and the
relation total is kept), and 943 pointers would not fit the row bound; the
heading cap is rule 3, which keeps operative text out of the public page by
omission. The row bound comes from the response budget, so it is kept and
degrades instead.

The outline and the serialized row are one ordered check, applied after the
other caps and never truncating inside a field: a row whose outline is within
2,200 entries and whose serialization is within 64 KiB is kept whole; otherwise
the build drops every heading (labels stay) and checks again; if the outline
still exceeds 2,200 entries or the row 64 KiB, the build drops the outline, sets
`index` false and records build gap `summary_outline_capped`; if the row without
an outline still exceeds 64 KiB, the document gets no row and the build records
`summary_row_capped`. On `vn/20261115_3` an estimated 4 rows drop their headings
and 2 their outline. A document without a row answers like a key outside the
release: the public endpoint and the page give 404, the list and the sitemap
leave it out, and the page never builds a fallback from other data. The build
fails only on a defect: an unknown gap kind, or a database or encoding error.

Nothing is truncated when serving. Eligibility is computed at build from the
row: `primary` is indexed when its outline holds at least one label and was not
capped; `relation_context` and `secondary` get a summary with `index` false; a
key outside the release has none.

## Endpoints

`GET /api/v1/public/documents/{key}` reads one row and evaluates status for the
Vietnam civil date from the injected clock; it touches no other table and never
the read path. Its schema is closed at every level:
`{release, projection, status_date, index, document, relations, relations_total, nodes: [{code, name}], outline: [{kind, label, anchor, heading?}]}`,
with `heading` only under the heading rule. `document` is
`{key, number, title, type, issuer, issued, effective_from, effective_to, status: {class, current, basis, status_basis, unresolved}, gap_kinds, has_text, text_complete, source_url}`:
the status fields are the reader's validity stamp for `status_date` plus
`unresolved` from `validity_conflict`; `gap_kinds` is the closed set
`missing_effective_date`, `consolidated_status_unordered`,
`consolidated_status_unresolved`; dates are null when unknown, and `source_url`
is null unless it is https. Relations keep the row's fields. It reads no cookie
and no bearer token. Headers: a strong `ETag` over the body and
`Cache-Control: public, max-age=N, must-revalidate`, N the smaller of 300 and
the seconds to the next Vietnam midnight.

`GET /api/v1/public/documents` answers every key with a summary in one bounded
response, `{release, projection, total, items: [{key, index}]}`, in key byte
order: at most 10,000 items or 1 MiB, else 503 `too_large`. There is no cursor.
Same admission, availability and headers.

Errors, in this precedence:

1. 400 `invalid_request` for a malformed key or any query parameter.
2. 503 `busy` with `Retry-After: 5` when admission or the pool is full.
3. 503 `unavailable` when no release is active or release resolution fails.
4. 404 `not_found` when the operator flag is off, or the active release is known
   to lack the summary capability.
5. 404 `not_found` for a key outside the active release.

`GET /api/v1/public/status` gains `features.public_summaries` and is served with
`Cache-Control: public, max-age=60, must-revalidate`.

## Admission and capacity

In this order:

1. **Public admission.** A process-wide gate of 8 in-flight requests covers both
   endpoints, cache hits included, through serialization. A request that finds
   it full gets 503 `busy` at once, before release resolution or any cache
   lookup.
2. **Resolution.** A metadata-only resolver reads the active release id and its
   summary capability from the release record. It never opens the reader engine,
   the embedder or postings.
3. **Row cache.** 16 MiB, entries living at most as long as their release
   ([caching](caching.md)).
4. **Database.** A pool of one connection per active release, separate from the
   reader's pool of four. Acquisition does not wait. Each query has a 2-second
   deadline and is cancelled when the client disconnects.
5. **Activation.** The new release's public pool opens with it; the old pool
   stops taking requests, finishes its in-flight ones within 5 seconds and
   closes. Peak PostgreSQL connections during activation stay within the
   cluster's 50 ([host budgets](budgets-host.md)).

These two routes have no per-address bucket, so no forwarded address is trusted
for them.

## Tests

- **Isolation.** The reader component never mounts on a summary visit. Calls to
  `/api/v1/me`, `/api/v1/documents/`, `/api/v1/read`, `/api/v1/search` and any
  `/api/v1/auth/` are counted from Nitro and the browser through SSR, hydration,
  client navigation between summaries and the summary's error pages, with no
  cookie and with a valid, an expired and a forged `__Host-session`; the count
  is zero.
- **Closed schema.** A Go contract test validates every actual summary and list
  response against the closed OpenAPI schema.
- **Credential equivalence.** At a fixed clock, both endpoints return identical
  bodies with no credentials, a valid session cookie and a bearer token, and the
  summary HTML is identical for an anonymous request and one carrying a valid
  `__Host-session`.
- **Decoded payloads.** The summary HTML, its decoded `__NUXT_CSP_DATA__` and
  `__NUXT_CSP_CONFIG__`, and every network response contain none of the markers
  planted in provision text, an annex body and each forbidden heading.
- **Headings.** No heading appears for: a short operative source title in the
  exclusion fixture; a `phan` and a `muc` heading; an `unknown_node_kind`, an
  `article_sequence` and a `tree_text_partial` document; a 301-character
  heading; an OCR document; and a release without approval. One `chuong` and one
  `dieu` heading that pass every rule appear.
- **Scan.** On an unapproved release, a candidate that triggers the scan appears
  in the scan output while the endpoint serves its entry as a label only.
- **End to end.** One fixture document with an approved article heading and a
  named topic passes through the build, the API and the page, and both show.
- **Availability.** A new-schema release, a rollback to an old-schema release,
  then a new-schema release again, each with warm caches, check status, both
  endpoints, topic links and the sitemap; so do flag-off and no active release.
- **Freshness.** With warm caches, across Vietnam midnight, activation, rollback
  and flag-off, each new response reflects the new state within the bound, and
  nothing is reused after `max-age`.
- **Escape.** A fixture title containing `</script><script>alert(1)</script>`
  stays inside one JSON-LD block in the server-rendered summary and parses back
  unchanged.

## Capacity acceptance

Run on the real release on the local stack, then on the A1 host with the flag on
for the test window only. The thresholds are proposals until measured on A1;
measured results, including web and server memory against their rows in
[host budgets](budgets-host.md), go into [budgets](budgets.md). The load tool,
accounts, sampling and A1 window are in
[summary capacity harness](web-public-documents-capacity.md).

| Phase               | Traffic                                                                                                                                                                                          | Duration       |
| ------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | -------------- |
| A, baseline         | three accounts: distinct-query searches at 2 per second in total; completed reader loads (HTML and the first document page) at 1 per second                                                      | 10 min         |
| B, cold coverage    | A, plus every summary URL at 8 per second from 16 addresses; a `busy` answer is retried at most 3 times with backoff                                                                             | until complete |
| C, warm working set | A, plus 500 summary URLs each requested twice within 60 seconds; the second must be a Nitro cache hit by the render counter                                                                      | 5 min          |
| D, overload         | A, plus 40 summary pages per second from 40 addresses, 50 direct public API calls per second, the full list 10 times per second, 5% of requests aborted, and one activation; 429 and 503 ignored | 5 min          |

| Gate                               | Proposal                                                                                                                                                                             |
| ---------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Search, every phase                | p95 at most 1 s in A and 1.25 s in B to D, p99 at most 2 s in A and 2.5 s in B to D, and p95 at most 1.25 times phase A's; in the 60 s after an activation, only the absolute bounds |
| Completed reader load, every phase | p95 at most 1.5 s, p99 at most 3 s, and p95 at most 1.25 times phase A's; in the 60 s after an activation, only the absolute bounds                                                  |
| Signed-in errors, every phase      | none                                                                                                                                                                                 |
| Coverage in B                      | every summary URL succeeds within its retries                                                                                                                                        |
| Successful summary latency         | p95 500 ms cold (B), 50 ms on a cache hit (C)                                                                                                                                        |
| Hit rate of C's second request     | at least 99%                                                                                                                                                                         |
| Reader-pool wait                   | p99 at most 50 ms                                                                                                                                                                    |
| Peaks over all phases              | PostgreSQL connections at most 40; host CPU at most 80% in A to C, recorded in D; web cgroup memory charge at most 300 MB; server within its row; no restart                         |

An activation opens the new release with cold caches and pools, so the minute
after it is held to the absolute bounds only; the local run on 2026-11-17 put
search p95 near 1 s in that minute and at 266 ms outside it.

The first A1 run, on 2026-11-19, put search p95 at 981 ms in phase A at 62% host
CPU. On 4 OCPUs the 1 s bound leaves no headroom for added public traffic, so B
to D take absolute bounds of 1.25 s and 2.5 s. While A stays under 1 s the
relative bound is the tighter one, and it is what guards search against public
traffic. D is overload by design: its host CPU is recorded, not gated. Web
memory is the container's cgroup charge, not process RSS, which counts shared
pages charged elsewhere: in that run web RSS peaked at 359.83 MB and its cgroup
at 213 MiB. The harness samples and aborts by these measures.
