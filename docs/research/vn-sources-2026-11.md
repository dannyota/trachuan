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

## Recommendations

1. Keep VBPL as the only primary source. Fix coverage in discovery: fetch
   relation targets one hop out, and add exact number lookups for documents
   named by the source list or golden cases (corpus scope plan).
2. Add `vanban.chinhphu.vn` as a secondary source only for central government
   documents absent from VBPL, recorded with source `chinhphu`; its text comes
   from the signed PDF, so it belongs to the text extraction plan.
3. Do not read Công báo or the State Bank site; they add no document VBPL lacks
   in the checked set.
