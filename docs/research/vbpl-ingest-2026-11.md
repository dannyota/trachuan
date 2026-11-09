# First VBPL ingest run (2026-11-09)

The first live run of the corpus pipeline:
`corpus start vn 20261109 --until relate` on the owner's MacBook in Vietnam
against the VBPL gateway, with the banhmi attachment cache. Not authority; the
rules are in [pipeline](../design/pipeline.md) and the limits in
[budgets](../design/budgets.md).

## Outcome

The run ended `paused` after `relate`, generation 3 (two restarts to pick up
fixes). Times are wall clock in UTC+7.

| Stage      | Items                                         | Gaps                                                  | Time                       |
| ---------- | --------------------------------------------- | ----------------------------------------------------- | -------------------------- |
| `discover` | 27 queries, all stable after 2 passes         | none                                                  | 2 min                      |
| `fetch`    | 3,227 expected; 3,210 done                    | 17 `source_unavailable`, 5 `missing_attachment`       | 167 min (see "Throughput") |
| `extract`  | 3,210 done                                    | 780 `no_tree`, 63 `article_sequence`, 16 `empty_text` | under 1 min                |
| `relate`   | 3,210 done; 14,311 relations to 2,778 targets | 6,131 `unknown_target`, 5 `unknown_relation_code`     | under 1 min                |

Provisions extracted: 28,125 articles, 60,792 clauses, 50,608 points, 4,470
chapters, 3,445 sections, 489 parts. Relations by kind: `legal_basis` 8,345,
`implements` 2,028, `cites` 998, `consolidates` 813, `amends_supplements` 813,
`replaces` 670, `repeals` 620.

## Compared with banhmi v049

| Measure                                              | Value                            |
| ---------------------------------------------------- | -------------------------------- |
| Expected documents in Vietnamese (without `vbpqta_`) | 2,884                            |
| of which with a provision tree                       | 2,399                            |
| banhmi primary documents with a VBPL id              | 2,799                            |
| banhmi primary documents in this run's expected set  | 2,663 (95.1%)                    |
| Expected documents banhmi never had                  | 170                              |
| Golden-cited documents (39) in the expected set      | 34, all 34 with a provision tree |

The five golden documents outside the expected set are 134/2025/QH15,
71/2025/QH15, 368/2025/NĐ-CP, 58/2021/NĐ-CP and 142/2026/NĐ-CP; none is listed
by the State Bank sweep or by banhmi's keyword searches over the other central
agencies. banhmi held them through other sources, which the text extraction plan
adds.

## Findings

1. HTTP/2 stalled the gateway. With eight requests in flight Go multiplexed them
   on one HTTP/2 connection and no item finished for minutes; the goroutine dump
   showed every batch inside `http2.Transport.RoundTrip`. With HTTP/2 disabled
   throughput went from about 3 to 46 documents per minute. The client now uses
   HTTP/1.1 with a pool sized to the in-flight limit.
2. A direct measurement (40 detail requests per level) gave 2.6 requests per
   second at concurrency 2 and 10.5 at concurrency 8, p50 about 640 ms, no
   errors: server latency, not the network, bounds throughput. Defaults are now
   8 requests per second and 8 in flight.
3. The banhmi attachment cache (6,631 indexed files, 8.3 GB) served 3,136
   attachments after sha256 verification.
4. The detail endpoint answers HTTP 500 for 17 documents, mostly from 2001 to
   2003 and English renditions, while their tree, diagram and file list
   answer 200. These end as `source_unavailable` gaps.
5. The State Bank sweep lists 343 English renditions (`vbpqta_` ids). They are
   not legal text in Vietnamese; the expected set now excludes them.
6. Keyword searches use `all_words` over title syllables, so `tài sản số` (245
   documents) and `ngân hàng số` (150) match almost any title containing `số`.
   Backlogged for review before the index plan.
7. 780 documents have no provision tree (banhmi parsed 557 primary documents
   from text); they wait for the text extraction plan.

## Costs

Artifacts 7.6 GB (mostly attachments), checkpoint artifact copy 7.6 GB, one
workshop dump 17 MB. The workflow history of the `relate` segment has 226
events, recorded in `apps/corpus/internal/workflow/testdata/histories/` for the
replay test.
