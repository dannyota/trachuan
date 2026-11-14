# Host budgets

Hosting cost, host memory, alarms and disk for the serving host. Request,
pipeline, indexing, retrieval and Temporal limits are in [budgets](budgets.md).

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
rows total 10,940 MB (estimates, not measured peak use), leaving about 12 GB for
the OS, page cache and margin. The server row includes its [caches](caching.md).
PostgreSQL uses `shared_buffers` 4 GiB and `effective_cache_size` 16 GiB so the
active corpus (2.9 GB for vn/20261110_4) stays in memory. JIT is off: on the A1
host it spent 1.2 s compiling the reader's relation query, which runs in 8 ms
without it (measured 2026-11-14). The owner accepted the embedder row (ADR 0009,
0011).

| Process                                        | RSS MB |
| ---------------------------------------------- | ------ |
| embedder (ONNX INT8, Vietnamese_Embedding_v2)  | 2,000  |
| postgres, shared_buffers 4 GiB                 | 4,500  |
| web (Node)                                     | 250    |
| server (Go), with caches and sparse postings   | 2,140  |
| caddy                                          | 50     |
| headroom for activation restore and HNSW build | 2,000  |

Server caches, each a byte-capped least-recently-used map
([caching](caching.md)):

| Cache            | Byte cap | Maximum age        |
| ---------------- | -------- | ------------------ |
| Query vectors    | 256 MB   | 24 hours           |
| Search responses | 1,024 MB | 1 hour             |
| Read items       | 512 MB   | release life, 24 h |
| Document entries | 64 MB    | release life       |
| Topic node pages | 8 MB     | release life       |

A value above one sixteenth of its cache's byte cap is not cached.

Activation refuses to start below 2.5 GB free memory or 10 GB free disk.

Acceptance gates before the first public release, each measured on the host:
search p95 at most 1 s and p99 at most 2 s at 2 requests per second for 5
minutes; embedder RSS at most 2.0 GB after a 5-minute stress at full admission
(Go service: 1.7 GB on the Mac); activation peak memory within the headroom row;
disk under 70% after activation with the previous release kept for rollback.

Search on the host (2026-11-14, 9d44fde3, release vn/20261112_2 with sparse
postings, measured from a client with a 36 ms round trip): one MCP client, 40
distinct questions per class at about 0.9 requests per second, 240 requests, 0
errors. Cold p50 / p95 / p99: full at 20 hits 555 / 640 / 1,281 ms (the slowest
was the run's first request), filtered full 535 / 662 / 678 ms, compact 447 /
577 / 593 ms. Repeats served from the response cache: 120 to 215 ms at p50,
mostly payload transfer (316 KB full, 69 KB compact). Server memory stayed at
335 to 382 MB. The 2-requests-per-second gate is not yet measured: it needs two
clients, because each user is limited to 60 searches a minute.

Embedder measurements on the host: 0.83 GB in a direct ONNX Runtime run (INT8,
2026-11-10). The Go query service (2026-11-11) passes the embedder gate. At
e25b097 it held 1.21 to 1.28 GB through and after the 5-minute stress at full
admission and a 60-second burst of 10 clients, but RSS was 2.35 GB right after
startup and peaked at 2.51 GB in the first minutes while the Go runtime still
held the verified 570 MB model buffer. With `debug.FreeOSMemory` after warmup
(d2e74cb), steady RSS is 1.1 to 1.3 GB: 1.07 GB at ready and for the next
minute, 1.17 GB after a 60-second stress with two clients (799 requests, all
200). Model load still peaks at about 2.4 GB (VmHWM 2,424,064 kB) for under 5
seconds before ready, above the 2,000 MB row. Over loopback HTTP: 92 short
questions one at a time p50 83 ms and p95 131 ms; a 217-token text p50 485 ms;
two clients without pause p50 160 ms, p95 233 ms, p99 273 ms at 12.2 requests
per second; the burst got 429 at once and no 504. Details are in the
[INT8 study](../research/query-int8-2026-11.md#go-service-on-the-a1-host-2026-11-11).

Owner-supplied host measurements from 2026-11-09, across three runs with
four-thread FP32: short-query p50 207–267 ms; long 217-token query p50 1.2–1.7
s. These are direct ONNX Runtime timings, not end-to-end search latency or
full-load acceptance. The earlier
[research note](../research/host-oci-a1-2026-11.md) records one of those runs.
Keep the existing deadlines and release gates; measure contention with the
thread limits in [indexing](budgets.md#indexing) and two active embedding
requests. Current INT8 measurements and their limits are in
[ADR 0009](../adr/0009-dense-embedding-recipe.md).

## Host alarms (proposed)

| Signal                                       | Trigger                 |
| -------------------------------------------- | ----------------------- |
| Filesystem used, boot and data separately    | Above 70% for 5 minutes |
| Memory utilization                           | Above 90% for 5 minutes |
| Instance accessibility/infrastructure status | Nonzero for 1 minute    |
| Host heartbeat or expected metric stream     | Absent for 5 minutes    |

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
