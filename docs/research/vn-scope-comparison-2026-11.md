# Corpus scope spike (2026-11-09)

Observed contracts for the [corpus scope](../design/sources.md) work, recorded
by the primary on 2026-11-09 from the owner's MacBook at no more than 1 request
per second per host. Not authority: the design and the source list decide. The
fixtures are in `apps/corpus/internal/vbpl/testdata/` (`record-scope.sh`) and
`apps/corpus/internal/chinhphu/testdata/` (`record.sh`).

## VBPL gateway

Number lookups post `optionDoc: docNum`, `matchMode: exact`,
`agencyLevel: TRUNG_UONG` and the number as `keyword` to `/doc/all`.

- The response carries `data.total`, `data.pageNumber`, `data.pageSize`,
  `data.items` and `data.tokens`, which echoes the query.
- Matching is whole-number: `367/QĐ-TTg` returns three documents numbered
  exactly `367/QĐ-TTg` (2009, 2013, 2014) and none numbered `1367/QĐ-TTg`.
- `total` is the same on every page. A page past the end returns an empty
  `items` list with the same `total`. A lookup is complete when every page
  succeeded, the totals agree and the distinct ids equal `total`.
- A missing document is a complete result with `total: 0`.
- The gateway does not fold Cyrillic look-alikes: `368/2025/NĐ-CР` with a
  Cyrillic Er returns 0 while the Latin number returns 185857. Normalize before
  comparing, and send configured numbers in Latin letters.
- Yearless numbers recur: `127/QĐ-TTg` returns one document from 2008, not the
  2021 AI strategy with that number.

Title searches with `matchMode: exact` match the keyword as a contiguous phrase
with its diacritics; `all_words` matches each syllable anywhere and ignores
diacritics (`thuê ngoài` matches titles with `thuế` and `nước ngoài`). Exact
mode loses 52/2024/NĐ-CP, whose VBPL title reads "không dung tiền mặt", and
titles that split a phrase, such as "định danh và xác thực điện tử".

Errors seen on demand were JSON: 404 `{"status":404,...}` for an unknown path,
and 400 with `"success":"false"` (a string) for an unknown provision tree id. No
non-JSON HTTP 200 was reproduced; tests build one in code.

Relation direction: a diagram's `documentNamesByType` lists the document's
outward targets and `documentNamesBySource` lists documents that point to it.
171687 lists 117031 under type 12 (replaces) and 117031 lists 171687 under
source type 12; 186058 lists 171687 under type 10 and 171687 lists 186058 under
source type 10.

The full central listing (`agencyLevel: TRUNG_UONG`, no keyword or agency)
reports 57,951 documents and accepts `pageSize` 1000. Listing items carry no
field classification, and 2,027 of the 3,210 cached details from run vn/20261109
have the field "Chưa phân loại", so VBPL fields cannot define scope.

## vanban.chinhphu.vn

Search is an ASP.NET postback of the home page form to `/`; the fields are in
the fixture README. Observed contracts:

- `ctrl_191017_163$drdRecordPerPage` (50, 100, 200, 500) caps the result set;
  the grid shows 50 rows per page. The page info `a - b | N` reports
  `N = min(matches, cap)`. A search is complete only when `N` is below the cap,
  every page was read and the distinct docids equal `N`; `N` equal to the cap is
  truncation. The unfiltered home grid reports 99,798 documents.
- An empty search shows `Không có thông tin văn bản!` and a blank page info.
  That marker is the recorded zero total.
- Paging posts `__EVENTTARGET=ctrl_191017_163$grvDocument` with
  `__EVENTARGUMENT=Page$<n>` or `Page$Last`.
- `__VIEWSTATE` (58 to 92 KB) and `__EVENTVALIDATION` change on every response.
  A postback built from older tokens still succeeded, so a stale token is not a
  failure signal. Cookies are `ASP.NET_SessionId` and `l`.
- The keyword matches substrings of numbers: `367/QĐ-TTg` returns 18 rows,
  including `1367/QĐ-TTg` and `2367/QĐ-TTg` from many years. Compare normalized
  numbers locally.
- Detail pages are `/?pageid=27160&docid=<n>` with a two-column table:
  `Số ký hiệu`, `Ngày ban hành` (DD-MM-YYYY), `Ngày có hiệu lực`,
  `Loại văn bản`, `Cơ quan ban hành`, `Người ký`, `Trích yếu` and
  `Tài liệu đính kèm` (links). `Cơ quan ban hành` is the issuer evidence:
  `Chính phủ`, `Thủ tướng Chính phủ`, `Quốc hội`, `Bộ Khoa học và Công nghệ`,
  `Ngân hàng Nhà nước Việt Nam` for central documents;
  `Chủ tịch UBND tỉnh Phú Thọ` for a provincial one. The issuer filter lists 401
  bodies, including provincial councils and people's committees.
- A missing docid returns HTTP 200 with `Không tìm thấy văn bản này` and no
  field table.
- Listing numbers can carry trailing spaces ("63/2025/QĐ-CTUBND " with a
  trailing space).
- Attachments are on `datafiles.chinhphu.vn`: HEAD answers `application/pdf`
  with a length and `accept-ranges: bytes`; the first bytes are `%PDF-1.4`.
  Names vary (`142-2026-ndcp.signed.pdf`, `05-bkhcn.pdf`), so a name does not
  prove a signature. A missing file is HTTP 404 with S3 `NoSuchKey` XML. Plain
  HTTP on that host also serves the PDF without a redirect; the client must
  build HTTPS URLs only. `http://vanban.chinhphu.vn/` answers 301 to HTTPS.
- Pages are 170 to 270 KB raw and about 80 KB for a detail. No challenge or HTTP
  403 appeared in about 60 requests; responses carry `server: openresty`.

## AI documents

The `trí tuệ nhân tạo` search on `vanban.chinhphu.vn` returns eleven complete
rows. Two are in VBPL; nine are not found by an exact VBPL number lookup with an
equal number and issue date.

| docid  | Number           | Issued     | Issuer                      | VBPL lookup            |
| ------ | ---------------- | ---------- | --------------------------- | ---------------------- |
| 219813 | 53/2026/TT-NHNN  | 30-09-2026 | Ngân hàng Nhà nước Việt Nam | present                |
| 219354 | 1671/QĐ-TTg      | 28-08-2026 | Thủ tướng Chính phủ         | 0                      |
| 219163 | 1528/QĐ-TTg      | 11-08-2026 | Thủ tướng Chính phủ         | 0                      |
| 218658 | 33/2026/QĐ-TTg   | 30-06-2026 | Thủ tướng Chính phủ         | 0                      |
| 218027 | 804/QĐ-TTg       | 06-05-2026 | Thủ tướng Chính phủ         | 0                      |
| 218029 | 142/2026/NĐ-CP   | 30-04-2026 | Chính phủ                   | 0                      |
| 217165 | 05/2026/TT-BKHCN | 10-03-2026 | Bộ Khoa học và Công nghệ    | 0                      |
| 217086 | 367/QĐ-TTg       | 28-02-2026 | Thủ tướng Chính phủ         | 3 others, 2009 to 2014 |
| 216334 | 134/2025/QH15    | 10-12-2025 | Quốc hội                    | present                |
| 210997 | 83/CĐ-TTg        | 23-08-2024 | Thủ tướng Chính phủ         | 0                      |
| 202565 | 127/QĐ-TTg       | 26-01-2021 | Thủ tướng Chính phủ         | 1 other, 2008          |

83/CĐ-TTg is an official telegram (`Công điện`) about AI workforce training; the
earlier research did not name it. Every named number in the design's
`chinhphu.named_numbers` is also found by the keyword search.

## Scope comparison and owner approval

The comparison artifact lives outside Git in the main checkout's
`work/scope-2026-11-09/`: `comparison-v2.json` (SHA-256
`a75a1ff9394da27cd23b37077e87edfad8c0c0bec584f4df50d3b25ad51afa05`) and
`accounting-v2.json` (SHA-256
`b74f55e4d77554e5d6661861f0d165f1a3056ebf49184b63cfc0f46c50e0487e`), with every
raw response beside them; all 428 recorded page hashes verify. They supersede
`comparison.json` (`54224586…`) and `accounting.json` (`e57625f6…`), which
compared the keyword-exact candidate set.

| Measure                                                    | Value                     |
| ---------------------------------------------------------- | ------------------------- |
| Old seed set: State Bank sweep and 26 `all_words` searches | 3,227 ids                 |
| Server-side `exact` searches, 32 terms, eight agencies     | 267 ids                   |
| Central listing, two identical complete passes             | 57,951 ids, 116 requests  |
| Local agency selection {62, 908}                           | 2,519 ids, equal to sweep |
| Local phrase selection beyond the eight agencies           | 285 ids                   |
| New seed set: local selection and 12 named numbers         | 3,074 ids                 |
| Old-only ids                                               | 442                       |
| of which kept as one-hop relation context                  | 33                        |
| of which excluded as syllable-match noise                  | 409                       |
| New-only ids                                               | 289                       |

All 38 VBPL documents in the evaluation identity map are new seeds. The 591 ids
the keyword-exact list first dropped include four golden documents:
88/2019/NĐ-CP, 143/2021/NĐ-CP and 340/2025/NĐ-CP (titles with `ngân hàng`) and
52/2024/NĐ-CP (title typo `dung`). The owner approved, on 2026-11-09, the five
restoring terms and five named numbers now in [sources](../design/sources.md),
the exclusion of the remaining noise, and the switch to local selection, which
is a superset of the approved keyword-exact set. `accounting-v2.json` lists each
old-only id with its disposition, supporting seeds or old queries.

## Expansion measurement

Measured over the 2,731 new seeds without renditions: 2,433 details and diagrams
came from the run vn/20261109 artifacts and 298 were requested live (596
requests, about 29 MB; ten details answered without data, nine of them HTTP 500
as in that run). Their qualified outward edges reach 2,453 distinct targets, of
which 1,111 are not seeds and become relation context, under the 5,000 ceiling.
Only 33 context ids have a cached bundle. At the run vn/20261109 average of 2.45
MB per bundle, the 298 new seeds and 1,078 new context ids add about 3.4 GB of
fetch volume. A new run's `expand` stage reads 5,462 detail and diagram
responses, about 11 minutes at the measured 8 requests per second.

## Offline test seams

Tests never call the network. Later tasks use these seams:

- VBPL keeps `NewWithBase` for an `httptest` server, `WithLimits` and
  `WithBackoffs`; fixture bodies come from `vbpl/testdata`.
- The chinhphu client takes an `http.RoundTripper` that serves the fixture pages
  for the real `https://vanban.chinhphu.vn` and `https://datafiles.chinhphu.vn`
  URLs, so host allowlists and redirect checks run unchanged; a resolver
  function (host to addresses) for the nonpublic address check; a clock and a
  sleep function for backoff and `Retry-After`; and a limits value (rate, burst,
  in flight, pages, page bytes, file bytes, total bytes, redirects) that tests
  reduce.
- Source pause and request admission sit behind a small interface backed by the
  workshop row, so client tests use an in-memory gate and integration tests the
  real one.
- A challenge or HTTP 403 and a non-JSON HTTP 200 are built in test code; no
  live source produced either.
