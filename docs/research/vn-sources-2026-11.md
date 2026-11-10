# Vietnam legal sources (2026-11-09)

Checked with a browser and the public APIs on 2026-11-09 to decide which sources
the corpus should read. Not authority; the pipeline design and the source list
decide.

## Findings

| Source                                          | Owner                 | What it is                                                                                                                    | Verdict                                                      |
| ----------------------------------------------- | --------------------- | ----------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------ |
| VBPL gateway `vbpl-bientap-gateway.moj.gov.vn`  | Ministry of Justice   | National legal database: metadata, validity status, provision tree, relations, attachments                                    | Primary, keep                                                |
| Cổng Pháp luật quốc gia `phapluat.gov.vn`       | Ministry of Justice   | National law portal launched in 2025; its JSON API (`/api/legal-documents`) lists 174,407 documents with the same ids as VBPL | Same database as VBPL; not a separate source                 |
| Hệ thống văn bản Chính phủ `vanban.chinhphu.vn` | Government Office     | Central government documents with signed PDFs on `datafiles.chinhphu.vn`                                                      | Secondary, only for documents VBPL lacks                     |
| Công báo `congbao.chinhphu.vn`                  | Government Office     | Official gazette (PDF issues)                                                                                                 | Drop: its search returned 0 results for known documents      |
| State Bank site                                 | State Bank of Vietnam | Circulars of one issuer                                                                                                       | Drop: every State Bank document in the golden set is in VBPL |
| Bộ pháp điển `phapdien.moj.gov.vn`              | Ministry of Justice   | Codification                                                                                                                  | Taxonomy only (ADR 0004), not text                           |

Evidence:

- The portal's ids are VBPL ids: 09/2020/TT-NHNN is `144532` on both, which is
  corpus key `vn:vbpl:144532`. Newer documents use time-based UUIDs on both.
- Four of the five golden documents missing from run vn/20261109 exist in VBPL
  and the gateway serves their detail and provision tree: 71/2025/QH15
  (`179989`), 368/2025/NĐ-CP (`185857`), 58/2021/NĐ-CP (`148075`) and
  18/2018/TT-NHNN (`130588`, expired). They are missing because discovery did
  not list them, not because of the source.
- 142/2026/NĐ-CP (30/04/2026, detailing the Law on Artificial Intelligence) is
  not in VBPL; `vanban.chinhphu.vn` has it with a signed PDF.
- Number lookups on the portal API need `searchOptions: 2`; option 1 searches
  text and returns documents that only cite the number.
- The web front-ends of `vbpl.vn` and `phapluat.gov.vn` return 403 to
  non-browser clients; the gateway API answers the pipeline's client.

## The banhmi cache against the dropped sources

banhmi read four sources: VBPL 3,937 documents, `vanban` 216, `sbv_hanoi` 70,
`congbao` 6. 132 non-VBPL documents are not in run vn/20261109. Looking each
number up in VBPL (after fixing Cyrillic look-alike letters and missing years in
banhmi's numbers) leaves:

- `vanban`: 84 of 105 are in VBPL and only outside the discovery scope; the rest
  are recent central documents VBPL lacks (for example 142/2026/NĐ-CP,
  174/2026/NĐ-CP, 228/2026/NĐ-CP, 353/2025/NĐ-CP, 05/2026/TT-BKHCN,
  33/2026/QĐ-TTg) and four provincial decisions, which VBPL holds at the local
  level and are out of scope.
- `sbv_hanoi`: its circulars are in VBPL; the other 19 are State Bank decisions:
  administrative-procedure notices, policy rate decisions, corrections, and
  2345/QĐ-NHNN on online payment security (repealed by 2872/QĐ-NHNN; Circular
  50/2024/TT-NHNN now governs).
- `congbao`: its one document VBPL lacks, 33/2026/QĐ-TTg, is on `vanban`.

## Artificial intelligence

VBPL holds two central documents with "trí tuệ nhân tạo" in the title, the Law
on Artificial Intelligence 134/2025/QH15 and 53/2026/TT-NHNN; both are in the
corpus. `vanban.chinhphu.vn` lists eleven, including 142/2026/NĐ-CP,
33/2026/QĐ-TTg (high-risk AI systems), 05/2026/TT-BKHCN (national AI ethics
framework), 367/QĐ-TTg (law implementation plan), 804/QĐ-TTg (datasets),
1671/QĐ-TTg and 1528/QĐ-TTg (strategy and workforce). VBPL title search with
`matchMode: exact` also cuts noise: "tài sản số" falls from 1,176 hits under
`all_words` to 7.

## Recommendations

1. Keep VBPL as the only primary source. Fix coverage in discovery: fetch
   relation targets one hop out, look up named documents by exact number
   (`optionDoc: docNum`, `matchMode: exact`), and switch keyword searches to
   `matchMode: exact` (corpus scope plan).
2. Add `vanban.chinhphu.vn` as the secondary source for central documents VBPL
   lacks, with source `chinhphu`, starting with the AI documents above; a
   document VBPL later publishes moves to VBPL by number. Text comes from the
   signed PDF in the text extraction plan.
3. Drop Công báo and `sbv_hanoi`. Do not add `sbv.gov.vn` now; State Bank
   decisions that are not law stay out until a case needs them.
