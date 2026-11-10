# Budgets and limits

Numeric limits live here. Values marked "estimate" are replaced by measurement
before the first release.

## Hosting cost, monthly, ap-singapore-1

USD list prices read 2026-11-09; 730 hours/month, before tax. Estimate assumes
one production host, no other usage of the quoted tenancy allowances, 50 GB
Standard Object Storage and 100,000 requests/month. These are usage estimates,
not a bill or a promise that the owner's remaining allowances are unused. The A1
zero-cost rows use the paid price list; actual tenancy eligibility remains
unverified until billing confirms it.

| Item                              | Monthly USD                 | Basis and source                                                  |
| --------------------------------- | --------------------------- | ----------------------------------------------------------------- |
| A1, 4 OCPU and 24 GB              | 0.00                        | 2,920 OCPU-hours and 17,520 GB-hours; [paid A1 allowance][prices] |
| Boot, 50 GB Balanced              | 0.00 eligible; 2.125 billed | [200 GB combined allowance][free]; [0.0425/GB-month][block-price] |
| Data block volume, 60 GB Balanced | 0.00 eligible; 2.55 billed  | Same allowance and rate                                           |
| Standard Object Storage, 50 GB    | 1.02                        | (50 minus 10 free) × [0.0255/GB-month][object-price]              |
| Object requests, 100,000          | 0.017                       | (100,000 minus 50,000 free) / 10,000 × [0.0034][object-price]     |
| Internet outbound, 50 GB          | 0.00                        | [First 10 TB/month free in APAC][prices]                          |
| Reserved public IPv4              | 0.00                        | [Oracle public-IP pricing][ip-price]                              |
| Vault, Monitoring, Notifications  | 0.00                        | Within [included allowances][free] below                          |
| Cloudflare DNS, Free plan         | 0.00                        | Owner's existing plan                                             |
| Total with eligible free volumes  | 1.04                        | Rounded from 1.037                                                |
| Total with both volumes billed    | 5.71                        | Rounded from 5.712                                                |

[prices]: https://www.oracle.com/cloud/price-list/
[free]:
  https://docs.oracle.com/en-us/iaas/Content/FreeTier/freetier_topic-Always_Free_Resources.htm
[block-price]:
  https://blogs.oracle.com/cloud-infrastructure/elastic-performance-with-the-next-generation-block-storage-on-oracle-cloud
[object-price]: https://www.oracle.com/cloud/storage/archive-storage/faq/
[ip-price]:
  https://blogs.oracle.com/cloud-infrastructure/release-announcement-reserved-public-ips-are-now-available-on-oracle-cloud-infrastructure

Oracle's [price list][prices] explicitly gives **paid tenancies** 3,000 A1
OCPU-hours and 18,000 GB-hours/month across VM, bare metal and container
instances. The [Free Tier guide][free] instead says **all tenancies** get 1,500
OCPU-hours and 9,000 GB-hours. Both were re-read 2026-11-10; the sources
disagree. Keep the paid-price-list estimate, but the tenancy's actual allowance
is unverified until billing shows it. [Upgrading retains free
resources][upgrade] does not resolve which allowance applies.

If the smaller allowance applies, the modeled compute charge is 26.98/month:
1,420 excess OCPU-hours × 0.01 plus 8,520 excess GB-hours × 0.0015. Total cost
would be 28.02 with eligible volumes or 32.69 with both volumes billed. If no A1
allowance remains, [list rates][a1-price] add 55.48/month at 730 hours.

[upgrade]: https://www.oracle.com/cloud/free/faq/
[a1-price]: https://www.oracle.com/cloud/compute/arm/

Free boot/block storage requires the home region and available eligibility;
confirm whether Singapore is the home region before budgeting zero. Balanced
uses 10 VPUs/GB: 0.0255 storage + 0.017 performance per GB-month. Do not assume
the measurement VM and another full-size production VM both fit the allowance.
The paid-account Standard-tier free amount is 10 GB, not the free-only account's
pooled 20 GB. Budget all retained versions, WAL and checkpoints in the 50 GB;
increase the estimate after measuring retention growth.

For the Vault configuration in [deployment](deployment.md#secrets), budget at
most 150 secrets. Keep Monitoring within 500 million ingestion and 1 billion
retrieval points/month, Notifications within 1 million HTTPS and 1,000 email
deliveries/month. No paid log export is assumed. If all A1, volume, Standard
storage and request allowances are consumed elsewhere, this usage costs about
61.46/month, before excess outbound or observability usage. The 60 USD target
therefore needs usage alerts, not a reserved instance. Domain registration,
taxes and GitHub/GHCR usage beyond the owner's existing plan are excluded;
verify those separately before enabling deploy automation.

Corpus processing is separate and per run: Claude Batch classification (estimate
20 to 40 USD for a full run, under 5 USD for a weekly delta), MacBook
electricity, zero Temporal fees (self-hosted). Record actual cost in each
release manifest.

## Host memory (estimates)

For the [host selected in ADR 0006](../adr/0006-single-host-oci-podman.md), the
rows total 4,650 MB, leaving about 19 GB for the OS, page cache and unallocated
margin; this is not measured peak consumption. The INT8 allocation follows the
accepted query precision in ADR 0009 and query service in ADR 0011.

| Process                                        | RSS MB |
| ---------------------------------------------- | ------ |
| embedder (ONNX INT8, Vietnamese_Embedding_v2)  | 1,200  |
| postgres                                       | 1,000  |
| web (Node)                                     | 250    |
| server (Go)                                    | 150    |
| caddy                                          | 50     |
| headroom for activation restore and HNSW build | 2,000  |

Activation refuses to start below 2.5 GB free memory or 10 GB free disk.

Acceptance gates before the first public release, each measured on the host:
search p95 at most 1 s and p99 at most 2 s at 2 requests per second for 30
minutes; embedder RSS at most 1.2 GB after a 30-minute soak (0.83 GB measured
for the INT8 model in a direct run); activation peak memory within the headroom
row; disk under 70% after activation with two retained releases.

Owner-supplied host measurements from 2026-11-09, across three runs with
four-thread FP32: short-query p50 207–267 ms; long 217-token query p50 1.2–1.7
s. These are direct ONNX Runtime timings, not end-to-end search latency or
full-load acceptance. The earlier
[research note](../research/host-oci-a1-2026-11.md) records one of those runs.
Keep the existing deadlines and release gates; measure contention with the
thread limits below and two active embedding requests. Current INT8 measurements
and their limits are in [ADR 0009](../adr/0009-dense-embedding-recipe.md).

## Host alarms (proposed)

| Signal                                       | Trigger                 |
| -------------------------------------------- | ----------------------- |
| Filesystem used, boot and data separately    | Above 70% for 5 minutes |
| Memory utilization                           | Above 90% for 5 minutes |
| Instance accessibility/infrastructure status | Nonzero for 1 minute    |
| Host heartbeat or expected metric stream     | Absent for 5 minutes    |

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

VBPL gateway: at most 16 requests per second and 16 in flight by default
(`CORPUS_VBPL_RPS`, `CORPUS_VBPL_IN_FLIGHT`); request timeout 30 seconds. Retry
HTTP 429, HTTP 5xx and timeouts 3 times, after 2, 8 and 30 seconds. A document
whose gateway calls still return HTTP 5xx after those retries becomes a
`source_unavailable` gap in the same attempt: the stage does not retry it, so a
document the source keeps refusing never holds a batch or the stage close; the
next run fetches it again. HTTP 429 and timeouts that outlast the client retries
stay retryable at the batch level, because they signal load, not a broken
document. Measured 2026-11-09: 10.5 requests per second at concurrency 8, p50
636 ms, no errors; measured 2026-11-10: 0.11 to 0.36 s per request. The 16
defaults are provisional until a live run records their error rate. Attachment
downloads use a separate pool of 8 in flight (`CORPUS_VBPL_FILE_IN_FLIGHT`) with
no request rate limit, because the attachment CDN caps throughput per IP (about
4 MB/s, measured by banhmi); a slow download never holds a gateway slot. Fetch
issues a document's four gateway calls concurrently and downloads its
attachments concurrently within the file pool.

Stage batches in flight: 8 by default (`CORPUS_BATCHES_IN_FLIGHT`); `fetch` runs
16 (`CORPUS_FETCH_BATCHES_IN_FLIGHT`); `embed` runs at most 2, so one batch
waits in the document embedder queue while the other runs on the GPU.

Attachment hosts: `CORPUS_VBPL_FILE_HOSTS` is a comma-separated allowlist,
default `s3-han02.fptcloud.com`.

### Corpus scope ceilings (proposed, pending measurement)

| Limit                    | Value                                                                                                                 |
| ------------------------ | --------------------------------------------------------------------------------------------------------------------- |
| Relation expansion       | 1 hop; 5,000 distinct added context ids per run                                                                       |
| Discovery result bound   | 100,000 ids per query; 3 complete passes maximum                                                                      |
| Central listing          | Page size 1,000; measured 2026-11-09: 57,951 ids, 58 requests and 86 MB per pass, 129 s for two passes at 1 request/s |
| chinhphu candidates      | 100 unique docids per run, before VBPL exclusion                                                                      |
| chinhphu search          | Request cap 500; 100 pages per query per pass; 3 passes maximum                                                       |
| chinhphu requests        | 1 request/s, burst 1, 1 in flight across page and file hosts                                                          |
| chinhphu response        | 5 MB per HTML response; existing 50 MB attachment cap                                                                 |
| chinhphu total downloads | 1 GB per run, including retries and rejected bodies                                                                   |
| chinhphu deadlines       | 30 s page request; 120 s PDF request; 5 min per item                                                                  |
| chinhphu redirects       | At most 3, with each destination revalidated                                                                          |
| chinhphu retries         | 3 after 2, 8 and 30 s for 429, 5xx or transport timeout                                                               |
| chinhphu Retry-After     | Wait at least the supplied delay; above 10 min defer with a gap                                                       |

These are worker-wide source limits, not per-activity allowances. Honor the
later of retry backoff and `Retry-After`; do not hold an activity past its item
deadline to wait. Exhaustion produces a retryable source gap. Access denials
stop the source rather than consuming retries. All attempts consume request and
byte budgets. Record actual request count, bytes and time in run evidence.

Exceeding any count or byte ceiling fails that scope stage explicitly; never
truncate a listing, expansion or attachment inventory and mark it complete. The
existing total build-document cap still applies after union and exclusions. The
context ceiling permits up to 250 GB at one maximum-size attachment each, so it
is a count bound, not a disk estimate. The primary measures attachment volume
and available workshop/checkpoint disk before a full run; stop rather than raise
limits automatically. Secondary payloads add at most 1 GB of wire data per run,
plus stored artifacts and checkpoint copies. No new paid service or hosted model
is introduced. These limits need owner approval after the spike.

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
| Query ONNX threads                                        | 4 intra-op, 1 inter-op, sequential execution                    |
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

Query limits apply to the Go query service. The transitional Python query mode
uses the same query thread settings and client timeout in this table, including
the corpus client's timeout, so evaluation matches serving budgets. Python is
only the parity reference under [ADR 0011](../adr/0011-embedder-services.md),
not a host service or an alternative release runtime. ONNX Runtime's
[thread settings](https://onnxruntime.ai/docs/performance/tune-performance/threading.html)
define intra-op parallelism; sequential execution does not create an inter-op
thread pool.

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

The host uses a 50 GB boot volume and a 60 GB data volume. Keep OS, images and
model on boot; databases and pgBackRest spool on data. Both use Balanced
performance. The existing estimates fit with margin; the 70% acceptance rule
applies to each filesystem, including retained releases.

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
