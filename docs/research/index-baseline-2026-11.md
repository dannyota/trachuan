# Index baseline for vn/20261110_2 (2026-11-10)

First full run through `chunk`, `embed`, `build` and `eval`, scored under
[ADR 0010](../adr/0010-retrieval-evaluation-for-agents.md) with retrieval
recipe 1. Not authority. Run `vn/20261110_2` on the MacBook: PostgreSQL 18,
pgvector 0.8.7, documents embedded in FP16 on MPS, queries in FP32 on ONNX CPU.
The numbers come from `corpus eval` on the restored database at commit
`22750c3`; `report.json` SHA-256 `5e5ec41e…04c06b7c`. `release_eligible` is
false by design and `quality_pass` is false.

## Corpus

| Item                 | Value                                                                     |
| -------------------- | ------------------------------------------------------------------------- |
| Documents            | 2,872 (12 more excluded as fetch gaps)                                    |
| Provisions           | 146,292                                                                   |
| Chunks               | 96,750, none rejected by the embedder                                     |
| Relations            | 14,311                                                                    |
| Coverage gaps loaded | 18,061                                                                    |
| Lexical terms        | 7,645 distinct, 31 colliding; nonzeros per chunk p50 75, p95 116, max 171 |
| Database             | 1,603 MB; indexes 872 MB                                                  |
| Dump                 | 583 MB (`pg_dump` custom format)                                          |

## Quality

Exact and ANN search return the same lists for every case: ANN raw recall 1.0
against exact, no fallback, 0 points lost on every metric.

| Metric (hybrid, limit 8) | Old product        | Legacy             | Floor |
| ------------------------ | ------------------ | ------------------ | ----- |
| recall                   | 0.677 (63 of 93)   | 0.688 (66 of 96)   | 0.90  |
| MRR                      | 0.579              | 0.584              | 0.66  |
| current-law precision    | 1.000 (712 of 712) | 1.000 (736 of 736) | 0.99  |
| abstention               | 0.865 (77 of 89)   | 0.837 (77 of 92)   | 0.95  |

| Arm (old product, limit 8) | recall | MRR   |
| -------------------------- | ------ | ----- |
| hybrid                     | 0.677  | 0.579 |
| dense only                 | 0.656  | 0.526 |
| lexical only               | 0.548  | 0.358 |

Recall at limit 20 is 0.742 for the old product cohort and 0.729 for legacy. The
three rejection cases (queries without diacritics) pass. The provisional
abstention threshold is cosine 0.4812: all 5 negatives abstain and 12 of 84
positives abstain wrongly.

## Coverage

11 of the 30 old product misses at limit 8 cannot be hits: their citations name
4 documents the run does not hold (58/2021/NĐ-CP 5 citations, 142/2026/NĐ-CP 4,
71/2025/QH15 1, 368/2025/NĐ-CP 1). Without them recall would be at most 0.882.
The corpus scope plan adds these documents. 780 documents have no provision tree
and so no chunks; the text extraction plan covers them.

## Evidence size and time

| Item                                       | Value                                                                                  |
| ------------------------------------------ | -------------------------------------------------------------------------------------- |
| Evidence characters per response, limit 8  | p50 13,520, p95 24,531                                                                 |
| Evidence characters per response, limit 20 | p50 37,978, p95 54,027                                                                 |
| Exact search per case                      | p50 277 ms, p95 386 ms                                                                 |
| ANN search per case                        | p50 213 ms, p95 274 ms                                                                 |
| Build stage                                | 7.6 minutes                                                                            |
| Eval stage, 181 cases in both modes        | 2 minutes at machine load 40                                                           |
| Embed stage                                | 1,583 documents in 8 minutes after a resume; the full stage was not timed in one piece |

Timings were taken while other builds used the machine, so they are upper
bounds, not budgets. Peak memory and disk growth were not recorded.

## Restore check

The corpus database was dropped and `corpus eval` restored it from the build
dump. Table counts match (2,872 documents, 146,292 provisions, 96,750 chunks,
14,311 relations, 18,061 gaps, 7,645 terms). `arms.jsonl` and `cases.jsonl` are
byte-identical between the built and the restored database, and the reports
differ only in timings and the `built` or `restored` label.

## Findings fixed during the run

- Hydration read the whole corpus on every call: recursive joins without row
  estimates chose sequential scans, and exact search left index scans disabled
  for the hydration statements. One call took 12 to 49 s under load and
  evaluation failed on its 60 s deadline. Commit `e33f0ca` makes hydration
  index-driven with identical output; exact search p50 fell from 1,449 ms to 277
  ms.
- 3,237 workshop artifact rows held absolute paths under the old checkout name
  and broke build planning after the rename; they were rewritten to
  root-relative paths in the local database.
- Two absent map entries were reported as `mapping_incomplete` because documents
  with the alternate number exist without a provision tree. The rule now needs a
  document with provisions (commit `22750c3`).

## Limitations

- 92 cases, 96 citations; the old product cases also calibrate the provisional
  threshold, so abstention is optimistic.
- Legacy numbers use the product recipe and are not a reproduction of banhmi's
  93.8% recall on its own corpus.
- Development and sealed cases, tuning, taxonomy filters and the Go query
  embedder are later plans.
- The run embeds documents in FP16; the earlier run `vn/20261109` used FP32 and
  a different chunk recipe, so the two are not a controlled comparison.
