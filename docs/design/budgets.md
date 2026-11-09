# Budgets and limits

Numeric limits live here. Values marked "estimate" are replaced by measurement
before the first release.

## Hosting cost, monthly, ap-southeast-1

Prices from the AWS Price List API (`aws pricing get-products`), read
2026-11-08, 730 hours per month. Items marked "assumed" were not read from the
API and are verified before the host is created.

| Item                                                      | USD  | Source                           |
| --------------------------------------------------------- | ---- | -------------------------------- |
| EC2 `t4g.large` Linux on demand, 0.0848 USD/h             | 61.9 | Price List API                   |
| EBS gp3 60 GB, 0.096 USD/GB-month                         | 5.8  | Price List API                   |
| Public IPv4, 0.005 USD/h                                  | 3.7  | assumed (AWS public IPv4 charge) |
| S3 releases and backups, 50 GB plus requests              | 2    | assumed                          |
| Cloudflare DNS for trachuan.vn, Free plan                 | 0    | owner's plan                     |
| CloudWatch alarms and logs                                | 1    | assumed                          |
| Data transfer out, under 50 GB                            | 4    | assumed                          |
| Total, on demand                                          | 78   |                                  |
| EC2 `t4g.large` 1-year reserved, no upfront, 0.0534 USD/h | 39.0 | Price List API                   |
| Total with the reserved instance                          | 55   |                                  |

The 60 USD target holds only with a one-year reserved instance or savings plan.
Decision rule: run on demand for the first month of measurement, then reserve if
`t4g.large` is the right size. `t4g.medium` (4 GB) is rejected on the memory
table below: the embedder, PostgreSQL and the activation headroom do not fit.

Corpus processing is separate and per run: Claude Batch classification (estimate
20 to 40 USD for a full run, under 5 USD for a weekly delta), MacBook
electricity, zero Temporal fees (self-hosted). Record actual cost in each
release manifest.

## Host memory (estimates)

| Process                                        | RSS MB |
| ---------------------------------------------- | ------ |
| embedder (ONNX FP32, Vietnamese_Embedding_v2)  | 2,500  |
| postgres                                       | 1,000  |
| web (Node)                                     | 250    |
| server (Go)                                    | 150    |
| caddy                                          | 50     |
| headroom for activation restore and HNSW build | 2,000  |

Activation refuses to start below 2.5 GB free memory or 10 GB free disk.

Acceptance gates before the first public release, each measured on the host:
search p95 at most 1 s and p99 at most 2 s at 2 requests per second for 30
minutes; embedder RSS at most 2.5 GB after a 30-minute soak; activation peak
memory within the headroom row; disk under 70% after activation with two
retained releases.

## Request limits

| Limit                                                                | Value                                                                                                                                                                         |
| -------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Query length                                                         | 2,000 characters and 256 model tokens                                                                                                                                         |
| Request body                                                         | 64 KB                                                                                                                                                                         |
| `search` per account                                                 | 60 per minute                                                                                                                                                                 |
| `document` per account                                               | 120 per minute                                                                                                                                                                |
| `taxonomy` depth; nodes per response                                 | 3; 500                                                                                                                                                                        |
| `search` limit; `nodes[]`; `doc_types[]`; `issuers[]`; `documents[]` | 50; 20; 10; 10; 20                                                                                                                                                            |
| `quality_gaps` page                                                  | 200 with cursor                                                                                                                                                               |
| Document text per response                                           | 2 MB; a response stops at a provision boundary with `next_from`; a provision above 2 MB is split into parts of 1 MB                                                           |
| Embedder admission                                                   | 2 active, 8 queued, 250 ms max wait, 2 s deadline                                                                                                                             |
| Login and OAuth endpoints per IP                                     | 20 per minute                                                                                                                                                                 |
| Public routes per IP (Nitro)                                         | 120 per minute, served from a 10-minute cache; cache keys limited to known routes and node codes; 4 concurrent renders, excess uncached requests receive 429 without queueing |
| Credential lookup cache                                              | 10 s, never beyond the credential's expiry                                                                                                                                    |
| Personal token lifetime                                              | default 90 days, at most 1 year                                                                                                                                               |
| CIMD fetch                                                           | 5 s, 64 KB, no redirects, 4 concurrent fetches, 1,000 cached documents for 24 h                                                                                               |
| OAuth clients stored                                                 | 10,000 total, 10 per client URL, LRU eviction of clients unused for 90 days                                                                                                   |

## Pipeline

Parser limits: 50 MB per file, 5 minutes per document, 500 OCR pages.

Activity batch 100 items; a heartbeat every 20 seconds while a batch runs. Batch
start-to-close timeout 1 hour; heartbeat timeout 2 minutes. At most 5 attempts,
with retry backoff from 30 seconds to 10 minutes. A `starting` run row is stale
after 2 minutes.

VBPL ingest bounds: API response 20 MB; provision tree depth 32 and 50,000
nodes; at most 3 listing passes per query.

VBPL: at most 8 requests per second and 8 in flight by default
(`CORPUS_VBPL_RPS`, `CORPUS_VBPL_IN_FLIGHT`), 8 stage batches in flight,
measured 2026-11-09 (10.5 requests per second at concurrency 8, p50 636 ms, no
errors); request timeout 30 seconds. Retry HTTP 429, HTTP 5xx and timeouts 3
times, after 2, 8 and 30 seconds. The `embed` stage runs at most 2 batches in
flight, so one batch waits in the document embedder queue while the other runs
on the GPU.

Attachment hosts: `CORPUS_VBPL_FILE_HOSTS` is a comma-separated allowlist,
default `s3-han02.fptcloud.com`.

Submission polling every 5 minutes; `deadline_at` 26 hours after submission;
`preparing` rows become uncertain after 10 minutes.

## Indexing

Recipe 1 baseline values below implement the
[study](../research/retrieval-hybrid-2026-11.md#reproduce-before-tuning). New
operational limits are proposed ceilings, not measured throughput claims.
Exceeding a build ceiling fails explicitly; it never drops rows to fit.

| Limit or parameter                                        | Value                                                           |
| --------------------------------------------------------- | --------------------------------------------------------------- |
| Chunk rough body target; minimum substantive content      | 512 rough tokens; 20 normalized runes                           |
| Prefix title and each structural heading; number          | 220 runes each; 128 runes                                       |
| Document input cap, including special tokens              | 2,048 model tokens (ADR 0009)                                   |
| Document embedding batch                                  | At most 16 texts, in chunk ordinal order                        |
| Document HTTP body; admission                             | 1 MiB; 1 active, 1 queued, 30 s maximum queue wait              |
| Document request deadline; client timeout                 | 120 s total; 125 s                                              |
| Query batch; HTTP body; client timeout                    | 1 text; existing 64 KB request cap; 2 s                         |
| Query ONNX threads                                        | 2 intra-op, 1 inter-op, sequential execution                    |
| Vector validation                                         | Finite float32; absolute L2 norm error at most 0.001            |
| Lexical dimensions; k1; b                                 | 1,048,576; 1.2; 0.75                                            |
| Sparse nonzeros per chunk                                 | At most 16,000; overflow fails build, no pruning                |
| HNSW construction                                         | m=16, ef_construction=64, cosine; no sparse index               |
| Build concurrency                                         | 1 database build/restore per worker, 1 SQL index worker         |
| Build maximum documents; provisions; inserted chunks      | 10,000; 2,000,000; 500,000                                      |
| Build distinct normalized terms                           | 2,000,000                                                       |
| Build copy/update batch                                   | 2,000 rows; stream vectors rather than retain all               |
| Build artifact file read                                  | At most 256 MiB per JSON file, checked before decoding          |
| Build memory ceiling; maintenance_work_mem                | 8 GiB worker RSS; 512 MiB PostgreSQL                            |
| Build/restore free disk before start; minimum during work | 30 GiB; 10 GiB (`CORPUS_DISK_START_GIB`, `CORPUS_DISK_MIN_GIB`) |
| Build item deadline; dump/restore command deadline        | 50 minutes; 15 minutes each within the item deadline            |
| Eval item deadline; ad-hoc exact query deadline           | 50 minutes; 60 s per query                                      |
| Eval artifact/cache total                                 | 2 GiB; fail explicitly if exceeded                              |

The shared activity timeout and heartbeat still apply. Cancel work before its
item deadline, leaving retryable infrastructure failures; resource estimates do
not authorize raising ceilings. The primary measures local RSS, disk, build
time, sparse nnz and dump size before accepting these limits. No new paid
service is added. Host latency and memory acceptance remain separate.

## Disk budget (host, estimates)

| Item                                              | GB       |
| ------------------------------------------------- | -------- |
| OS, Podman images, embedder model                 | 12       |
| Corpus release (142k chunks, vectors, HNSW, text) | 2.5 each |
| Active plus one retained plus one restoring       | 7.5      |
| Auth database                                     | 1        |
| journald 30 days                                  | 2        |
| pgBackRest spool                                  | 2        |
| Total                                             | 25       |

The 60 GB volume keeps usage under 50% with three release copies; the 70% rule
in the acceptance gates leaves room for one unexpected release.

## Retrieval

| Recipe 1 parameter                          | Value                                                                          |
| ------------------------------------------- | ------------------------------------------------------------------------------ |
| Dense / lexical candidate depths            | 50 / 50                                                                        |
| RRF k; dense / lexical weights              | 60; 1 / 0.5                                                                    |
| Validated document-number lexical weight    | 1                                                                              |
| ANN pool; ef_search                         | max(400, 24 times dense depth); min(pool, 1,000)                               |
| ANN scan settings                           | strict_order; force_custom_plan; max_scan_tuples=20,000; scan_mem_multiplier=1 |
| Hydration pool                              | First min(union size, 8 times response limit) fused chunks                     |
| Per-document cap; backfill                  | 3; demoted hits in rank order until response full                              |
| Default response limit; maximum             | 8; 50                                                                          |
| Evidence text per hit                       | 14,000 Unicode code points, then source spans and read pointer                 |
| Relations per hit; evidence per relation    | 20; 1,000 Unicode code points, omissions explicit                              |
| Document-reference parser                   | At most 8 references, 128 runes per reference                                  |
| Serving database statement deadline         | 2 s; no unbounded exact fallback                                               |
| Evaluation exact arm cache depth            | Dense 200; lexical 100                                                         |
| Provisional calibration threshold sentinels | -1 (all finite cosines pass); 2 (all abstain)                                  |

Recipe behavior is in [retrieval recipe](retrieval-recipe.md); the threshold is
derived by [calibration](evaluation.md#provisional-calibration), never assumed
to be the historical Qwen threshold. Golden-set floors have one home in
[testing](../../instructions/testing.md); taxonomy floors in
[taxonomy](taxonomy.md).

## Temporal (self-hosted on the MacBook)

Workflow payloads at most 256 KB; one workflow per release; activities batch 100
documents; the workflow continues-as-new after every stage, so one history
segment holds at most one stage (about 40 activities for 4,000 documents); a
segment above 5,000 events fails the run for investigation instead of growing.
