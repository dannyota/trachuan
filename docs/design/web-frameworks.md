# Framework pages on the website

Decision: [ADR 0016](../adr/0016-control-frameworks-corpus.md). This note owns
the website pages for control frameworks: which pages exist, what each shows,
and the words around the data. The data contract is
[framework results](frameworks-results.md); who may read what is
[framework access](frameworks-access.md); the routes are in [web](web.md).

The pages render what `/api/v1/frameworks` returns and decide nothing about
access: the server already reduces every record. A page never computes a count,
a title or a text the response does not carry, and never falls back from one
field to another.

## Pages

| Path                                      | Content                                                                  | API                                                     |
| ----------------------------------------- | ------------------------------------------------------------------------ | ------------------------------------------------------- |
| `/frameworks`                             | every framework version of the release                                   | `GET /api/v1/frameworks`                                |
| `/frameworks/search`                      | search of the frameworks corpus                                          | `POST /api/v1/frameworks/search`                        |
| `/frameworks/{code}/{version}`            | one version: head, outline, public gaps                                  | `GET .../versions/{handle}`, `GET .../gaps?versions=`   |
| `/frameworks/{code}/{version}/{citation}` | one control: head, text when readable, mappings, outline of its children | `GET .../controls/{handle}`, `GET .../mappings?handle=` |

Search has its own path, so a law search link never becomes a frameworks search.
The query form on `/vn/search` and on `/frameworks/search` carries a two-option
control, "Văn bản pháp luật" / "Legal documents" and "Khung kiểm soát" /
"Control frameworks", that moves between the two paths without the query; the
query text never enters a URL on either.

**URLs and handles.** `{code}` and `{version}` must match the handle grammar of
[results](frameworks-results.md#handles-and-identity), else the page is the
fixed 404. `{citation}` is the citation, encoded once with `encodeURIComponent`;
the router decodes it once. The page builds the canonical handle from code,
version and citation with the handle encoding rule, passes it to the API as one
segment with `encodeURIComponent`, and never decodes and re-encodes a handle the
API returned: links to controls are built from the returned `code`, `version`
and `citation` fields. `invalid_handle` and `not_found` render the fixed 404.

## What each page shows

**`/frameworks`.** One row per version from `versions`, in the release's order:
name and publisher, version, the access label, the account line, and the counts
the response carries (`controls`, `edges_resolved`, `edges_unresolved`), plus
lineage as links to the other versions. Readable versions add their `sources`
(text source and completeness). Totals come from `totals`. A link opens
`/frameworks/search`.

**Version page.** The version head (name, publisher, version, access label,
account line, lineage), then the outline as citations and our titles, each a
link to its control. For a version the account cannot read, one neutral sentence
says where access comes from: "Quyền đọc được cấp theo địa chỉ email đã xác minh
của tài khoản." / "Access is granted to the verified email of an account." It
offers no contact, request or purchase flow. Public gaps of the version follow,
with their fixed kind labels and details as served.

**Control page.** The head first: citation, kind, path (ancestor citations as
links), our title, status, lineage, access label. Then:

- **Readable control**: the publisher's title (`title_original`), each body
  paragraph, notes and tables, in the order the units arrive, as the words of
  the source: Noto Serif, verbatim, never shortened or reworded.
- **Reduced control** (`text_withheld: true`): one fixed line, "Nội dung của nhà
  xuất bản cần được cấp quyền mới đọc được. Tài khoản này chưa được cấp quyền."
  / "The publisher's text needs access to read. This account does not have it."
  No length, no paragraph count, no placeholder blocks, no preview and no other
  hint of the text.
- **Title**: our title when present; when absent, the citation and kind alone.
  Never `title_original` in its place, never text from another field.

Then the mappings: one row per edge with the relation label and both ends
(citation, version, our title; an unresolved end shows its business key and no
title), the source name, and `evidence` only when the edge carries it, with "(đã
rút gọn)" / "(shortened)" when `evidence_truncated`. An edge without evidence
shows nothing in its place. Then the outline of child controls.

A read with `complete: false` shows "Xem tiếp" / "Show more", which calls the
same route with `next`; `invalid_next` or `release_expired` reloads the page
from the start with one fixed line.

**Search.** The query form, a framework filter (codes from the status), and the
hits. A hit shows citation, kind, our title (or citation and kind alone), the
version name, the access label when reduced, the mapping count, and a snippet
only when the hit carries one, verbatim in Noto Serif. A lookup hit is labelled
"Tra theo số hiệu" / "Citation match" instead of a score. Abstention shows the
response's fixed message. "Xem thêm" / "Show more" continues with `next_cursor`;
`cursor_expired` restarts the search with one fixed line. A hit links to its
control page.

## Words

| Field or state       | vi                                                                     | en                                                               |
| -------------------- | ---------------------------------------------------------------------- | ---------------------------------------------------------------- |
| `access: free`       | Đọc tự do                                                              | Free to read                                                     |
| `access: licensed`   | Cần cấp quyền                                                          | Access required                                                  |
| `readable: true`     | Bạn đọc được                                                           | You can read this                                                |
| `readable: false`    | Bạn chưa được cấp quyền                                                | You do not have access                                           |
| `access_unavailable` | Tạm thời chưa kiểm tra được quyền đọc; trang đang hiện phần đọc tự do. | Access could not be checked right now; this shows the free part. |
| counts               | {n} mục kiểm soát · {n} liên kết · {n} liên kết chưa xác định đích     | {n} controls · {n} mappings · {n} unresolved mappings            |
| relation             | tương đương, là một phần của, bao gồm, giao với, liên quan             | equivalent to, subset of, superset of, intersects, related to    |

Names, publishers, versions, citations, our titles, kinds without a label,
statuses without a label, source names and gap details are data: shown as
served, never translated or rewritten. Citations and handles use Roboto Mono.
The site's own sentences follow [web voice](web-voice.md) and never claim more
than the response states. `access_unavailable` is a notice, not an error page.
`retrieval_recipe_status` is not shown.

## Access, caching and the flag

- Every page needs a session: a signed-out visit gets the 302 to `/login?next=`
  of [web](web.md), with `next` the path alone. Pages answer
  `Cache-Control: no-store` and `X-Robots-Tag: noindex`, take no Nitro route
  cache and no SWR, and are in no sitemap.
- The pages call only the `/api/v1/frameworks` routes, with the session the
  reader uses; responses are never stored in the browser or shared state beyond
  the page.
- With `SERVER_FRAMEWORKS` off every route answers 404, so every page renders
  the fixed 404. The law search shows the corpus control only after
  `GET /api/v1/frameworks` has answered 200 for this tab; the header has no
  frameworks link until the flag is on in production.

## Tests

Fixtures use invented frameworks only: `fw-alpha` `2.1` (`licensed`) and
`fw-beta` `1.0` (`free`), with synthetic text. Tests cover: each page for a
reduced, a mixed and a full access set; that a reduced control's HTML and
payload hold no `title_original`, body, note, table or snippet field and show
the fixed line; that a missing title shows citation and kind only; that an edge
without evidence renders identically whether evidence was withheld or absent;
handles with spaces, parentheses, `/` and `#` in the citation; the 302,
`no-store` and `noindex` headers; the fixed 404 when the API answers 404;
`access_unavailable`; continuations and their expiry errors; and the responsive
sweep at 320, 390 and 1366 in both themes.
