# Budgets and limits

Numeric limits live here. Values marked "estimate" are replaced by measurement
before the first release.

## Hosting cost, monthly, ap-southeast-1 (estimates, 2026-11-08)

| Item                                   | USD   | Note                                   |
| -------------------------------------- | ----- | -------------------------------------- |
| EC2 `t4g.large` on demand              | 49    | measure; a savings plan cuts about 30% |
| EBS gp3 60 GB                          | 5     |                                        |
| Public IPv4                            | 4     |                                        |
| S3 releases and backups, 50 GB         | 2     |                                        |
| Route 53 zone and queries              | 1     |                                        |
| CloudWatch alarms and logs             | 1     |                                        |
| Total recurring                        | 62    | target at or below 60 after sizing     |

Corpus processing is separate and per run: Claude Batch classification
(estimate 20 to 40 USD for a full run, under 5 USD for a weekly delta),
MacBook electricity, zero Temporal fees (self-hosted). Record actual cost in
each release manifest.

## Host memory (estimates)

| Process          | RSS MB |
| ---------------- | ------ |
| embedder (INT8)  | 1,500  |
| postgres         | 1,000  |
| web (Node)       | 250    |
| server (Go)      | 150    |
| caddy            | 50     |
| headroom for activation restore and HNSW build | 2,000 |

Activation refuses to start below 2.5 GB free memory or 10 GB free disk.

## Request limits

| Limit                                  | Value                                   |
| -------------------------------------- | --------------------------------------- |
| Query length                           | 2,000 characters                        |
| Request body                           | 64 KB                                   |
| `search` per account                   | 60 per minute                           |
| `document` per account                 | 120 per minute                          |
| `taxonomy` depth, `search` limit       | 3; 50                                   |
| Document text per response             | 2 MB, else paged by provision           |
| Embedder admission                     | 2 active, 8 queued, 250 ms max wait, 2 s deadline |
| Login and OAuth endpoints per IP       | 20 per minute                           |
| Public routes per IP                   | 120 per minute, served from cache       |
| CIMD fetch                             | 5 s, 64 KB, no redirects, 1,000 cached documents |

## Retrieval

Dense top 50, lexical top 50, RRF k 60. Golden-set floors: recall@8 0.90, MRR@8
0.66, current-law precision 0.99, abstention 0.95. Taxonomy floors are in
[taxonomy](taxonomy.md).

## Temporal (self-hosted on the MacBook)

Workflow payloads at most 256 KB; one workflow per release; activities batch
100 documents; history must stay under 10,000 events per workflow, else the
batch size grows.
