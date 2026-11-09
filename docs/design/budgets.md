# Budgets and limits

Numeric limits live here. Values marked "estimate" are replaced by measurement
before the first release.

## Hosting cost, monthly, ap-southeast-1

Prices from the AWS Price List API (`aws pricing get-products`), read
2026-11-08, 730 hours per month. Items marked "assumed" were not read from the
API and are verified before the host is created.

| Item                                             | USD   | Source                                  |
| ------------------------------------------------ | ----- | --------------------------------------- |
| EC2 `t4g.large` Linux on demand, 0.0848 USD/h   | 61.9  | Price List API                          |
| EBS gp3 60 GB, 0.096 USD/GB-month                | 5.8   | Price List API                          |
| Public IPv4, 0.005 USD/h                         | 3.7   | assumed (AWS public IPv4 charge)        |
| S3 releases and backups, 50 GB plus requests     | 2     | assumed                                 |
| Route 53 zone and queries                        | 1     | assumed                                 |
| CloudWatch alarms and logs                       | 1     | assumed                                 |
| Data transfer out, under 50 GB                   | 4     | assumed                                 |
| Total, on demand                                 | 79    |                                         |
| EC2 `t4g.large` 1-year reserved, no upfront, 0.0534 USD/h | 39.0 | Price List API                 |
| Total with the reserved instance                 | 56    |                                         |

The 60 USD target holds only with a one-year reserved instance or savings
plan. Decision rule: run on demand for the first month of measurement, then
reserve if `t4g.large` is the right size. `t4g.medium` (4 GB) is rejected on
the memory table below: the embedder, PostgreSQL and the activation headroom
do not fit.

Corpus processing is separate and per run: Claude Batch classification
(estimate 20 to 40 USD for a full run, under 5 USD for a weekly delta),
MacBook electricity, zero Temporal fees (self-hosted). Record actual cost in
each release manifest.

## Host memory (estimates)

| Process          | RSS MB |
| ---------------- | ------ |
| embedder (ONNX FP32, Vietnamese_Embedding_v2) | 2,500 |
| postgres         | 1,000  |
| web (Node)       | 250    |
| server (Go)      | 150    |
| caddy            | 50     |
| headroom for activation restore and HNSW build | 2,000 |

Activation refuses to start below 2.5 GB free memory or 10 GB free disk.

Acceptance gates before the first public release, each measured on the host:
search p95 at most 1 s and p99 at most 2 s at 2 requests per second for 30
minutes; embedder RSS at most 2.5 GB after a 30-minute soak; activation peak
memory within the headroom row; disk under 70% after activation with two
retained releases.

## Request limits

| Limit                                  | Value                                   |
| -------------------------------------- | --------------------------------------- |
| Query length                           | 2,000 characters and 256 model tokens   |
| Request body                           | 64 KB                                   |
| `search` per account                   | 60 per minute                           |
| `document` per account                 | 120 per minute                          |
| `taxonomy` depth; nodes per response   | 3; 500                                  |
| `search` limit; `nodes[]`; `doc_types[]`; `issuers[]`; `documents[]` | 50; 20; 10; 10; 20 |
| `quality_gaps` page                    | 200 with cursor                         |
| Document text per response             | 2 MB; a response stops at a provision boundary with `next_from`; a provision above 2 MB is split into parts of 1 MB |
| Embedder admission                     | 2 active, 8 queued, 250 ms max wait, 2 s deadline |
| Login and OAuth endpoints per IP       | 20 per minute                           |
| Public routes per IP (Nitro)           | 120 per minute, served from a 10-minute cache; cache keys limited to known routes and node codes; 4 concurrent renders, excess uncached requests receive 429 without queueing |
| Credential lookup cache                | 10 s, never beyond the credential's expiry |
| Personal token lifetime                | default 90 days, at most 1 year            |
| CIMD fetch                             | 5 s, 64 KB, no redirects, 4 concurrent fetches, 1,000 cached documents for 24 h |
| OAuth clients stored                   | 10,000 total, 10 per client URL, LRU eviction of clients unused for 90 days |

## Pipeline

Parser limits: 50 MB per file, 5 minutes per document, 500 OCR pages.
Activity batch 100 documents. Submission polling every 5 minutes;
`deadline_at` 26 hours after submission; `preparing` rows become uncertain
after 10 minutes.

## Disk budget (host, estimates)

| Item                                             | GB  |
| ------------------------------------------------ | --- |
| OS, Podman images, embedder model                | 12  |
| Corpus release (142k chunks, vectors, HNSW, text) | 2.5 each |
| Active plus one retained plus one restoring      | 7.5 |
| Auth database                                    | 1   |
| journald 30 days                                 | 2   |
| pgBackRest spool                                 | 2   |
| Total                                            | 25  |

The 60 GB volume keeps usage under 50% with three release copies; the 70%
rule in the acceptance gates leaves room for one unexpected release.

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
