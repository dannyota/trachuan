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
| Total recurring, on demand             | 62    |                                        |
| Total with a 1-year no-upfront savings plan on EC2 | about 45 | estimate; decide after one month of measurement |

Prices are unverified estimates; verify them in the AWS pricing calculator
for ap-southeast-1 before creating the host and record the result here. Data
transfer is assumed under 50 GB per month.

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

Acceptance gates before the first public release, each measured on the host:
search p95 at most 1 s and p99 at most 2 s at 2 requests per second for 30
minutes; embedder RSS at most 2 GB after a 30-minute soak; activation peak
memory within the headroom row; disk under 70% after activation with two
retained releases.

## Request limits

| Limit                                  | Value                                   |
| -------------------------------------- | --------------------------------------- |
| Query length                           | 2,000 characters                        |
| Request body                           | 64 KB                                   |
| `search` per account                   | 60 per minute                           |
| `document` per account                 | 120 per minute                          |
| `taxonomy` depth; nodes per response   | 3; 500                                  |
| `search` limit; `nodes[]`; `doc_types[]`; `issuers[]`; `documents[]` | 50; 20; 10; 10; 20 |
| `quality_gaps` page                    | 200 with cursor                         |
| Document text per response             | 2 MB; larger documents are read by anchor range (`from`, `to`) |
| Embedder admission                     | 2 active, 8 queued, 250 ms max wait, 2 s deadline |
| Login and OAuth endpoints per IP       | 20 per minute                           |
| Public routes per IP (Nitro)           | 120 per minute, served from a 10-minute cache; cache keys limited to known routes and node codes; 4 concurrent renders |
| CIMD fetch                             | 5 s, 64 KB, no redirects, 4 concurrent fetches, 1,000 cached documents for 24 h |
| OAuth clients stored                   | 10,000 total, 10 per client URL, LRU eviction of clients unused for 90 days |

## Retrieval

Dense candidates 50, lexical candidates 50, RRF k 60 (banhmi's measured
settings). Golden-set floors: recall@8 0.90, MRR@8
0.66, current-law precision 0.99, abstention 0.95. Taxonomy floors are in
[taxonomy](taxonomy.md).

## Temporal (self-hosted on the MacBook)

Workflow payloads at most 256 KB; one workflow per release; activities batch
100 documents; the workflow continues-as-new after every stage, so one
history segment holds at most one stage (about 40 activities for 4,000
documents); a segment above 5,000 events fails the run for investigation
instead of growing.
