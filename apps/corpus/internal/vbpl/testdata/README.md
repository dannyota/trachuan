# VBPL fixtures recorded 2026-11-09 (primary, curl at 1 request per second)

Headers: `Origin: https://vbpl.vn`, `Referer: https://vbpl.vn/`, User-Agent
trachuan/0.0. Presigned query strings removed from files-\*.json.

- list-agencies-62-908-page1.json: POST /doc/all, agencies 62 and 908, page 1,
  size 5, issueDate desc.
- 67878 (22/2014/TT-NHNN, legacy numeric id): detail, tree (11 Article, 34
  Clause, 17 Point nodes), diagram (types 3, 4, 9), files.
- 245a9a00-d447-11f1-84e4-cfbf13844ed1 (118/2026/VBHN-TT-NHNN, UUID id): detail
  (effStatus null), tree, diagram, files.
- 66518 (04/VBHN-NHNN): detail (effStatus null), empty tree (data []), diagram,
  files.
- 171687 (50/2024/TT-NHNN): detail (references null), diagram with types 1
  (repeals), 3 (legal_basis), 12 (replaces).

## Corpus scope fixtures (`record-scope.sh`, 2026-11-09)

POST /doc/all with `optionDoc: docNum`, `matchMode: exact`,
`agencyLevel: TRUNG_UONG`, no agency filter, unless noted. `data.tokens` echoes
the query.

- lookup-71-2025-QH15.json: unique match, total 1.
- lookup-367-QD-TTg.json: three central documents share the yearless number
  (2009, 2013, 2014); the 2026 decision with that number is not in VBPL.
- lookup-367-QD-TTg-size2-page1..3.json: the same lookup with page size 2;
  `total` stays 3 on every page and page 3 returns an empty `items` list.
- lookup-142-2026-ND-CP.json: complete empty result, total 0.
- lookup-368-2025-ND-CP-cyrillic.json: the number of 185857 with a Cyrillic Er
  (U+0420) in `CР`; the gateway returns total 0, so it does not fold look-alike
  letters.
- list-exact-tai-san-so.json: title search, `matchMode: exact`, the eight
  central keyword agencies; 4 results against 245 under `all_words`.
- list-all-words-thanh-toan-khong-dung-tien-mat.json and
  list-exact-thanh-toan-khong-dung-tien-mat.json: `exact` drops 52/2024/NĐ-CP,
  whose title reads "không dung tiền mặt"; `all_words` ignores diacritics.
- diagram-117031.json and diagram-186058.json, with diagram-171687.json:
  `documentNamesByType` lists outward targets and `documentNamesBySource` lists
  inbound documents. 171687 lists 117031 under type 12 (replaces); 117031 lists
  171687 under source type 12. 186058 lists 171687 under type 10 (amends);
  171687 lists 186058 under source type 10.
- list-central-size5-page1.json and list-central-size5-page2.json: the full
  central listing (`agencyLevel: TRUNG_UONG`, no keyword, no agency), page size
  5; `total` 57951 and `agencyIds` on every item.
