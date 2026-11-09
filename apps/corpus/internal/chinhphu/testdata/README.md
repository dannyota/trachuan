# vanban.chinhphu.vn fixtures recorded 2026-11-09 (primary, 1 request per second)

`record.sh` recorded these with User-Agent trachuan/0.0. Cookies
(`ASP.NET_SessionId`, `l`) stayed in memory. Saved pages replace the values of
`__VIEWSTATE`, `__EVENTVALIDATION` and `__VIEWSTATEGENERATOR` with
`REDACTED-<name>`; `evidence.json` records each status and whether `__VIEWSTATE`
changed from the page the request was built from.

Search posts the home page form back to `/` with
`ctrl_191017_163$txtSearchKeyword`, `ctrl_191017_163$drdRecordPerPage`,
`ctrl_191017_163$drdDocOrg` and `ctrl_191017_163$btnSearch`. Paging posts
`__EVENTTARGET=ctrl_191017_163$grvDocument` and `__EVENTARGUMENT=Page$<n>` or
`Page$Last` with the previous page's form fields.

- home.html: the form; its unfiltered grid reports `1 - 50 | 99798`.
- search-ai.html: keyword `trí tuệ nhân tạo`, 50 results requested; 11 rows,
  `1 - 11 | 11`.
- search-number-142-2026-ND-CP.html: keyword `142/2026/NĐ-CP`, cap 500; one row.
- search-number-367-QD-TTg.html: keyword `367/QĐ-TTg`, cap 500; 18 rows,
  including `1367/QĐ-TTg` and `2367/QĐ-TTg`: the keyword matches substrings.
- search-empty.html: no match; the grid shows `Không có thông tin văn bản!` and
  the page info is blank.
- search-provincial-org-16134.html: issuer filter `Chủ tịch UBND tỉnh Phú Thọ`;
  one row whose number ends in a space ("63/2025/QĐ-CTUBND " with a trailing
  space).
- search-truncated-page1.html, -page2.html, -pagelast.html: keyword `ngân hàng`,
  cap 500; page info `1 - 50 | 500`, `51 - 100 | 500`, `451 - 500 | 500`. The
  reported total equals the selected cap, so the match set is truncated.
- search-truncated-page3-stale-tokens.html: `Page$3` posted with page 1's tokens
  after later pages; the site served rows 101 to 150.
- detail-218029.html: 142/2026/NĐ-CP; issuer `Chính phủ`, signed PDF link.
- detail-215941.html: 63/2025/QĐ-CTUBND; issuer `Chủ tịch UBND tỉnh Phú Thọ`.
- detail-missing.html: docid 999999999; HTTP 200 with title text
  `Không tìm thấy văn bản này` and no field table.
- redirect-http.txt: `http://vanban.chinhphu.vn/` answers 301 to HTTPS.
- pdf-142-headers.txt and pdf-142-head.bin: HEAD of the 142/2026/NĐ-CP PDF on
  `datafiles.chinhphu.vn` (`application/pdf`, 3,661,910 bytes,
  `accept-ranges: bytes`) and its first 1,024 bytes from a range request
  (`%PDF-1.4`).
- file-missing.xml: a missing file on `datafiles.chinhphu.vn`; HTTP 404 with an
  S3 `NoSuchKey` XML body.

No challenge or HTTP 403 appeared in these requests; tests build one in code.
