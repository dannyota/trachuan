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

| Path                                             | Content                                                   | API                                                   |
| ------------------------------------------------ | --------------------------------------------------------- | ----------------------------------------------------- |
| `/frameworks`                                    | every framework version of the release                    | `GET /api/v1/frameworks`                              |
| `/frameworks/search`                             | search of the frameworks corpus                           | `POST /api/v1/frameworks/search`                      |
| `/frameworks/{code}/{version}`                   | one version: head, outline, gaps                          | `GET .../versions/{handle}`, `GET .../gaps?versions=` |
| `/frameworks/{code}/{version}/control?citation=` | one control: head, text when readable, mappings, children | `GET .../controls/{handle}`                           |

Search has its own path, so a law search link never becomes a frameworks search.
The query form on `/vn/search` and on `/frameworks/search` carries a two-option
control, "Văn bản pháp luật" / "Legal documents" and "Khung kiểm soát" /
"Control frameworks", that moves between the two paths without the query; the
query text never enters a URL on either.

**URLs and handles.** `{code}` and `{version}` must match the handle grammar of
[results](frameworks-results.md#handles-and-identity), else the page is the
fixed 404. A citation travels in the `citation` query parameter, never as a path
segment: a citation may be `.` or `..`, which URL parsing removes from a path,
while a query value survives as written. `URLSearchParams` decodes it once; the
page checks it against the citation grammar, builds the canonical handle from
code, version and citation with the handle encoding rule, and passes the handle
to the API as one segment with `encodeURIComponent`. Links are built the same
way from returned data: code and version from the part of a returned `handle`
before `#` (the grammar limits that part to `fw:<code>:<version>`, so it needs
no decoding), and the citation from the returned `citation` field. A returned
handle is never decoded or re-encoded. A mapping end without a `handle`
(unresolved) is shown and not linked. `invalid_handle` and `not_found` render
the fixed 404. For a signed-out visit, `next` is the first of these that passes
the `next` rules of [web API](web-api.md#google-login) unchanged: the control
page with its `citation` (as the reader keeps `release`), its version page,
`/frameworks`. A citation or a code the rules refuse (a backslash, a path over
the length limit) therefore falls back one step at a time, and `/frameworks`
always passes.

## What each page shows

**`/frameworks`.** One row per entry of `versions`, in the order of its keys:
code and version (from the version handle), the access label, the account line,
and the counts the response carries (`controls`, `edges_resolved`,
`edges_unresolved`), plus lineage as links to the other versions. Readable
versions add their `sources` (text source and completeness). Totals come from
`totals`. The status carries no name or publisher, so the list shows none; the
version page does. A link opens `/frameworks/search`.

**Version page.** The version head from the read's `version` object (name,
publisher, code, version, access label, account line, lineage), then the outline
as citations and our titles, each a link to its control. For a version the
account cannot read, one neutral sentence says where access comes from: "Quyền
đọc được cấp theo địa chỉ email đã xác minh của tài khoản." / "Access is granted
to the verified email of an account." It offers no contact, request or purchase
flow. The gaps the response returns for the version follow (the server returns
public gaps to everyone and text gaps only to accounts that can read the
version; the page shows what it gets and filters nothing), with their fixed kind
labels and details as served; "Xem thêm" / "Show more" continues with
`next_cursor` and the same `versions` selector, and `cursor_expired` reloads the
gaps from the start with one fixed line.

**Control page.** The head first: citation, kind, path (ancestor citations as
links), our title, status, lineage, access label. Then the units in the order
they arrive:

- **Readable control**: the publisher's title (`title_original`), each body
  paragraph, notes and tables, as the words of the source: Noto Serif, verbatim,
  never shortened or reworded.
- **Reduced control** (`text_withheld: true`): one fixed line, "Nội dung của nhà
  xuất bản cần được cấp quyền mới đọc được. Tài khoản này chưa được cấp quyền."
  / "The publisher's text needs access to read. This account does not have it."
  No length, no paragraph count, no placeholder blocks, no preview and no other
  hint of the text.
- **Title**: our title when present; when absent, the citation and kind alone.
  Never `title_original` in its place, never text from another field.
- **Mappings**: the read's mapping units are the one source of edges on this
  page; the mappings route is not called. One row per edge with the relation
  label and both ends (citation, version, our title; an unresolved end shows its
  business key and no title), the source name, and `evidence` only when the unit
  carries it, verbatim. An edge without evidence shows nothing in its place, so
  withheld and absent evidence render the same.
- **Outline**: the child controls as citations and our titles.

"Xem tiếp" / "Show more" appears only when the item carries `next`, and calls
the same route with it. `invalid_next` or `release_expired` reloads the page
from the start with one fixed line. Any other item `error` (`unit_too_large`,
`invalid_item`) shows the error's fixed message where the units stop, and no
button.

**Search.** The query form, a framework filter (codes from the status), and the
hits. A hit shows citation, kind, our title (or citation and kind alone), the
version name from the response's `frameworks` entry for the hit's version, the
access label when reduced, the mapping count, and a snippet only when the hit
carries one, verbatim in Noto Serif. A lookup hit is labelled "Tra theo số hiệu"
/ "Citation match" instead of a score. Abstention shows the response's fixed
message. "Xem thêm" / "Show more" continues with `next_cursor`; `cursor_expired`
restarts the search with one fixed line. A hit links to its control page.

## Look

The pages wear the family identity of [web visual](web-visual.md) and follow the
reader's layout.

- **Heads.** The version and control pages open with the reader's identity band
  on `mist`: the identifier (`code version`, or the citation) in Roboto Mono,
  then the heading, then one muted meta line. The heading is our title, or the
  version's name, in Plus Jakarta Sans 700; a control without our title shows
  its citation and kind as the heading instead. The meta line joins kind,
  status, the access label and the account line with " · " and never starts or
  ends with a separator.
- **Access label.** Plain muted text with an icon: a closed lock for "Cần cấp
  quyền", an open lock for "Đọc tự do". It is not a stamp: stamps keep their
  shape for validity alone.
- **Reduced statement.** A `mist` notice with the closed lock icon and `ink`
  text, where the publisher's text would start.
- **Lists.** Versions, outline entries, hits, mappings and gaps are rows with
  hairline separators: the identifier as a link in Roboto Mono, then our title
  (or the name) on its own line, then a muted meta line (counts, kind, relation,
  source). Sections have Plus Jakarta Sans headings: "Mục lục" / "Contents",
  "Liên kết" / "Mappings", "Giới hạn dữ liệu" / "Data limits".
- **Faces.** The publisher's title, body, notes, tables, snippets and evidence
  are the words of the source, in Noto Serif; everything ours is in Plus Jakarta
  Sans.

## Words

| Field or state       | vi                                                                                  | en                                                               |
| -------------------- | ----------------------------------------------------------------------------------- | ---------------------------------------------------------------- |
| `access: free`       | Đọc tự do                                                                           | Free to read                                                     |
| `access: licensed`   | Cần cấp quyền                                                                       | Access required                                                  |
| `readable: true`     | Bạn đọc được                                                                        | You can read this                                                |
| `readable: false`    | Bạn chưa được cấp quyền                                                             | You do not have access                                           |
| `access_unavailable` | Tạm thời chưa kiểm tra được quyền đọc; trang đang hiện phần đọc tự do.              | Access could not be checked right now; this shows the free part. |
| counts               | {n} mục kiểm soát · {n} liên kết đã xác định đích · {n} liên kết chưa xác định đích | {n} controls · {n} resolved mappings · {n} unresolved mappings   |
| relation             | tương đương, là một phần của, bao gồm, giao với, liên quan                          | equivalent to, subset of, superset of, intersects, related to    |

| Gap kind                                                     | vi                                            | en                                     |
| ------------------------------------------------------------ | --------------------------------------------- | -------------------------------------- |
| `mapping_unresolved`                                         | Liên kết chưa xác định đích                   | Mapping target not resolved            |
| `mapping_dropped`                                            | Liên kết không đưa vào bản phát hành          | Mapping left out of the release        |
| `title_missing`                                              | Chưa có tên do Tra Chuẩn đặt                  | No Tra Chuẩn title yet                 |
| `ocr_blocked`                                                | Nhận dạng ký tự chưa đạt                      | Character recognition did not pass     |
| `ocr_flagged`                                                | Nhận dạng ký tự đang được xem xét             | Character recognition under review     |
| `text_incomplete`                                            | Nội dung chưa đầy đủ                          | Text incomplete                        |
| `chunk_omission`                                             | Một phần nội dung không được đưa vào tìm kiếm | Part of the text is not searchable     |
| `embedding_input_too_long`, `embedding_input_bytes_exceeded` | Một phần nội dung quá dài để đưa vào tìm kiếm | Part of the text is too long to search |
| any other kind                                               | the kind code as served                       | the kind code as served                |

| State                                            | vi                                               | en                                                        |
| ------------------------------------------------ | ------------------------------------------------ | --------------------------------------------------------- |
| item `unit_too_large`                            | Một phần của mục này quá lớn để hiển thị.        | Part of this item is too large to show.                   |
| item `invalid_item`                              | Không đọc được mục này.                          | This item could not be read.                              |
| reload after `invalid_next` or `release_expired` | Dữ liệu đã được cập nhật; trang hiện lại từ đầu. | The data was updated; the page starts again from the top. |
| `cursor_expired` (search and gaps)               | the existing `cursor_expired` message            | the existing `cursor_expired` message                     |
| `invalid_handle`, `not_found`                    | the fixed 404                                    | the fixed 404                                             |
| `unavailable`, `rate_limited`                    | the existing messages of those codes             | the existing messages of those codes                      |

Names, publishers, versions, citations, our titles, kinds without a label,
statuses without a label, source names and gap details are data: shown as
served, never translated or rewritten. Citations and handles use Roboto Mono.
The site's own sentences follow [web voice](web-voice.md) and never claim more
than the response states. `access_unavailable` is a notice, not an error page.
`retrieval_recipe_status` is not shown.

## Access, caching and the flag

- Every page needs a session: a signed-out visit gets the 302 to `/login?next=`
  of [web](web.md), with `next` built as "URLs and handles" says. Pages answer
  `Cache-Control: no-store` and `X-Robots-Tag: noindex`, take no Nitro route
  cache and no SWR, and are in no sitemap.
- The pages call only the `/api/v1/frameworks` routes, with the session the
  reader uses; responses are never stored in the browser or shared state beyond
  the page.
- With `SERVER_FRAMEWORKS` off every route answers 404, so every page renders
  the fixed 404. The law search shows the corpus control only after
  `GET /api/v1/frameworks` has answered 200 in this tab; the probe runs once per
  tab, and any failure hides the control and shows no error on the law search.
  The header has no frameworks link until the flag is on in production.

## Tests

Fixtures use invented frameworks only: `fw-alpha` `2.1` (`licensed`) and
`fw-beta` `1.0` (`free`), with synthetic text. Tests cover: each page for a
reduced, a mixed and a full access set; that a reduced control's HTML and
payload hold no `title_original`, body, note, table or snippet field and show
the fixed line; that a missing title shows citation and kind only; that an edge
without evidence renders identically whether evidence was withheld or absent;
citations with spaces, parentheses, `/`, `#`, `%`, `\`, and the citations `.`
and `..`, round-tripped through links and the `next` of a signed-out visit, with
each step of the `next` fallback (a backslash citation, an over-long code); the
302, `no-store` and `noindex` headers; the fixed 404 when the API answers 404;
`access_unavailable`; continuations and their expiry errors; and the responsive
sweep at 320, 390 and 1366 in both themes.
