# Budgets and limits

Numeric limits live here and, for the serving host's cost, memory, alarms and
disk, in [host budgets](budgets-host.md). Values marked "estimate" are replaced
by measurement before the first release.

## Request limits

| Limit                                                                     | Value                                                                                                                                                                                                                                                                                                                                      |
| ------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Query length                                                              | 2,000 characters and 256 model tokens                                                                                                                                                                                                                                                                                                      |
| Request body                                                              | 64 KB                                                                                                                                                                                                                                                                                                                                      |
| Server HTTP timeouts                                                      | Headers 5 s and 16 KB; whole request read 10 s; response write 30 s; idle connection 120 s                                                                                                                                                                                                                                                 |
| Auth database query deadline                                              | 2 s; a timeout fails the request closed with 503                                                                                                                                                                                                                                                                                           |
| `search` per account                                                      | 60 per minute                                                                                                                                                                                                                                                                                                                              |
| `read` per account; items per call; handle, anchor and number length      | 120 per minute; 8; 512, 512 and 128 code points                                                                                                                                                                                                                                                                                            |
| `taxonomy` depth; nodes per response                                      | 3; 500                                                                                                                                                                                                                                                                                                                                     |
| `search` limit; `nodes[]`; `doc_types[]`; `issuers[]`; `documents[]`      | 50; 20; 10; 10; 20                                                                                                                                                                                                                                                                                                                         |
| `taxonomy` depth; `q` length; nodes per response; documents per node page | 3 (default 1); 100 code points; 500; 20                                                                                                                                                                                                                                                                                                    |
| `quality_gaps` page                                                       | 200 with cursor                                                                                                                                                                                                                                                                                                                            |
| Web document text per response; MCP `read` per call                       | 2 MB of serialized units per web document page by the `read` rules (leads, provisions and outline entries cut into parts of 1 MB, continuation by `next`). `read`: 40,000 Unicode code points of serialized units across items; any unit above 16,000 code points is split into parts; continuation by `next`                              |
| Embedder admission                                                        | 2 active, 8 queued, 250 ms max wait, 2 s deadline                                                                                                                                                                                                                                                                                          |
| Login and OAuth endpoints per IP                                          | 20 per minute                                                                                                                                                                                                                                                                                                                              |
| Pages per IP (Nitro)                                                      | 120 per minute, public routes served from a 10-minute cache; cache keys limited to known routes and node codes; 4 concurrent renders of any page, excess receive 429 without queueing                                                                                                                                                      |
| Public API per IP (`/api/v1/public/*`)                                    | 60 per minute; responses cacheable for 10 minutes                                                                                                                                                                                                                                                                                          |
| Credential lookup cache                                                   | 10 s, never beyond the credential's expiry                                                                                                                                                                                                                                                                                                 |
| Personal token lifetime                                                   | default 90 days, at most 1 year                                                                                                                                                                                                                                                                                                            |
| Credentials per account                                                   | 20 live personal tokens (beyond: 409 `token_limit`); 50 live web sessions (a new login revokes the oldest); revoked or expired tokens and sessions deleted 7 days after they end; the token list returns live rows and rows ended in the last 7 days, the session list live sessions only, newest first, at most 100                       |
| Error reports                                                             | 20 per account in any 24 hours; 200 open and 1,000 retained per account; 100,000 retained in total, report and export snapshot rows together; reports other than open ones deleted 90 days after their last state change; number segment set per release at most 50,000 segments of at most 32 bytes each                                  |
| CIMD fetch                                                                | 5 s, 64 KB, no redirects, 4 concurrent fetches, 1,000 cached documents for 24 h                                                                                                                                                                                                                                                            |
| OAuth clients stored                                                      | 10,000 total; the least recently used client not in use is evicted ([OAuth](oauth.md#clients))                                                                                                                                                                                                                                             |
| OAuth grants; redirect URIs per client; `state`                           | 20 live grants per account; 1 to 5; 512 bytes                                                                                                                                                                                                                                                                                              |
| OAuth lifetimes                                                           | Pending authorization request 10 min; code 60 s; access token 1 h; refresh family 30 days from the code exchange                                                                                                                                                                                                                           |
| OAuth state kept                                                          | 10 token families per grant (the oldest is revoked); 10,000 unexpired pending requests; `last_used_at` written at most hourly; cleanup every 10 min in batches of 1,000, up to 50 batches per step per run; replay retention 1 day (start events in [OAuth](oauth.md#retention)); grants without an unexpired family retired after 30 days |

## Pipeline

Parser limits: 50 MB per file, 5 minutes per document. PDF tool subprocesses
(planned, [PDF text layer](pdf-text.md)): 1 GiB memory, 256 MB scratch space, 50
MB stdout, 64 KB stderr, 60 seconds per call, 2 subprocesses at a time, and the
document limit above for all calls on one PDF. Text extraction: 50,000
paragraphs and nesting depth 64 per HTML transcription or OCR text; fresh OCR at
most 2,000 pages per document and 20,000 pages per run, above which the run
stops for the owner.

Activity batch 100 items; a heartbeat every 20 seconds while a batch runs, and
the worker lets the SDK hold a heartbeat back for at most a quarter of the
heartbeat timeout (the SDK default of 60 seconds left one late heartbeat between
a busy machine and a timed-out batch). The heartbeat interval must be at most a
quarter of the timeout. Batch start-to-close timeout 1 hour; heartbeat timeout 2
minutes. At most 5 attempts, with retry backoff from 30 seconds to 10 minutes. A
`starting` run row is stale after 2 minutes.

VBPL ingest bounds: API response 20 MB; provision tree depth 32 and 50,000
nodes; at most 3 listing passes per query.

VBPL gateway: at most 16 requests per second and 16 in flight by default
(`CORPUS_VBPL_RPS`, `CORPUS_VBPL_IN_FLIGHT`); request timeout 40 seconds,
counted from the moment the request asks for a slot, so waits for the slot, the
rate limit and the source gate are inside it. The 40 seconds exceed the
gateway's own 30-second cut, so its HTTP 504 arrives and counts as 5xx. HTTP
5xx, a timeout or a transport error gets one retry after 2 seconds; if that
fails the document becomes a `source_unavailable` gap with the cause in the same
attempt, and the next run asks again (owner rule 2026-11-12: one fast retry,
then a gap; measured the same day, two diagrams answered 504 or 500 for hours
and stalled a run for 22 minutes under the earlier three retries and batch
backoff). HTTP 429 is a load signal: 3 retries after 2, 8 and 30 seconds, and it
alone stays retryable at the batch level. Measured 2026-11-09: 10.5 requests per
second at concurrency 8, p50 636 ms, no errors; measured 2026-11-10: 0.11 to
0.36 s per request. The 16 defaults are provisional until a live run records
their error rate. Attachment downloads use a separate pool of 8 in flight
(`CORPUS_VBPL_FILE_IN_FLIGHT`) with no request rate limit, because the
attachment CDN caps throughput per IP (about 4 MB/s, measured by banhmi); a slow
download never holds a gateway slot. Fetch issues a document's detail and
diagram calls concurrently; a full read under
[fetch reuse](fetch-reuse.md#read-rule) then issues the tree and file list calls
concurrently and downloads its attachments concurrently within the file pool.

[Fresh OCR requests](ocr-vision.md): 8 Vision requests in flight, at most 600 a
minute, 500 pages per document, render bound 30 seconds plus 2 per page, scratch
ceiling 256 MB per document as for the cross-check; provisional.

[Fetch reuse](fetch-reuse.md): control divisor 20 (about 5% of documents re-read
in full per run), provisional until three runs report their control outcomes.
ETag requests run in the attachment pool: 8 in flight, a 30-second deadline, no
retry (a failure means a download). Attachment downloads: 30 seconds plus the
listed size at 250 kB/s per attempt, one retry after 2 seconds, then `missing`
with a `missing_attachment` gap
([attachments](fetch-attachments.md#download-deadline-and-failures)).

Stage batches in flight: 8 by default (`CORPUS_BATCHES_IN_FLIGHT`); `fetch` runs
16 (`CORPUS_FETCH_BATCHES_IN_FLIGHT`); `embed` runs at most 2, so one batch
waits in the document embedder queue while the other runs on the GPU.

Attachment hosts: `CORPUS_VBPL_FILE_HOSTS` is a comma-separated allowlist,
default `s3-han02.fptcloud.com`.

### Corpus scope ceilings (proposed, pending measurement)

| Limit                    | Value                                                                                                                                                                                                           |
| ------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Relation expansion       | 1 hop; 5,000 distinct added context ids per run                                                                                                                                                                 |
| Discovery result bound   | 100,000 ids per query; 3 complete passes maximum                                                                                                                                                                |
| Central listing          | Page size 1,000; measured 2026-11-09: 57,951 ids, 58 requests and 86 MB per pass, 129 s for two passes at 1 request/s                                                                                           |
| chinhphu candidates      | 100 unique docids per run, before VBPL exclusion                                                                                                                                                                |
| chinhphu search          | Request cap 500; 100 pages per query per pass; 3 passes maximum                                                                                                                                                 |
| chinhphu requests        | 1 request/s, burst 1, 1 in flight across page and file hosts                                                                                                                                                    |
| chinhphu response        | 5 MB per HTML response; existing 50 MB attachment cap                                                                                                                                                           |
| chinhphu total downloads | 1 GB per client lifetime (one worker process), retries and rejected bodies included; no durable per-run cap yet                                                                                                 |
| chinhphu deadlines       | 30 s page request; PDF request 30 s plus size at 250 kB/s; 5 min per item for page and lookup work, PDF downloads bounded by their own deadline and one retry outside it; at most 3 redirects, each revalidated |
| chinhphu retries         | 1 after 2 s for 5xx, timeout or transport error, then a gap; 429 as the [source error table](fetch-attachments.md#download-deadline-and-failures)                                                               |

These are worker-wide source limits, not per-activity allowances. Retries, HTTP
429 waits and their exhaustion follow the
[source error table](fetch-attachments.md#download-deadline-and-failures);
access denials pause the source. All attempts consume request and byte budgets;
record count, bytes and time in run evidence.

Exceeding a count or byte ceiling fails that scope stage explicitly; never
truncate a listing, expansion or attachment inventory and mark it complete. The
build-document cap still applies after union and exclusions. The context ceiling
is a count bound (up to 250 GB at one maximum-size attachment each), so the
primary measures attachment volume and disk before a full run and stops rather
than raising limits. Secondary payloads add at most 1 GB of wire data per worker
process; no new paid service or hosted model is introduced.

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

Query limits apply to the Go query service. ONNX Runtime's
[thread settings](https://onnxruntime.ai/docs/performance/tune-performance/threading.html)
define intra-op parallelism; sequential execution does not create an inter-op
thread pool.

The shared activity timeout and heartbeat still apply. Cancel work before its
item deadline, leaving retryable infrastructure failures; resource estimates do
not authorize raising ceilings. The primary measures local RSS, disk, build
time, sparse nnz and dump size before accepting these limits. No new paid
service is added. Host latency and memory acceptance remain separate.

## Retrieval

| Recipe parameter                                                                          | Value                                                                                                      |
| ----------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------- |
| Dense / lexical candidate depths                                                          | 50 / 50                                                                                                    |
| RRF k; dense / lexical weights                                                            | 60; 1 / 0.5                                                                                                |
| Validated document-number lexical weight                                                  | 1                                                                                                          |
| ANN pool; ef_search                                                                       | max(400, 24 times dense depth); min(pool, 1,000)                                                           |
| ANN scan settings                                                                         | strict_order; force_custom_plan; max_scan_tuples=20,000; scan_mem_multiplier=1                             |
| Hydration pool                                                                            | First min(union size, 8 times response limit) fused chunks                                                 |
| Per-document cap; backfill; non-current tail                                              | 5; demoted hits in rank order until response full; 2 slots for the non-current group in default mode       |
| Default response limit compact; full; maximum                                             | 20; 8; 50                                                                                                  |
| Evidence text per full hit; compact snippet                                               | 14,000 Unicode code points, then source spans and read pointer; 600 code points including omission markers |
| Relations per full hit or read item; evidence per relation; related pointers per document | 20; 1,000 Unicode code points, omissions explicit; 5                                                       |
| Warnings per hit or read unit                                                             | 10, omissions counted                                                                                      |
| Document-reference parser                                                                 | At most 8 references, 128 runes per reference                                                              |
| Serving database statement deadline                                                       | 2 s; no unbounded exact fallback                                                                           |
| Evaluation exact arm cache depth                                                          | Dense 200; lexical 100                                                                                     |
| Provisional calibration threshold sentinels                                               | -1 (all finite cosines pass); 2 (all abstain)                                                              |

Recipe behavior is in [retrieval recipe](retrieval-recipe.md); the threshold is
derived by [calibration](calibration.md), never assumed to be the historical
Qwen threshold. Golden-set floors have one home in
[testing](../../instructions/testing.md); taxonomy floors in
[taxonomy](taxonomy.md).

## Temporal (self-hosted on the MacBook)

Workflow payloads at most 256 KB; one workflow per release; activities batch 100
documents; the workflow continues-as-new after every stage, so one history
segment holds at most one stage (about 40 activities for 4,000 documents); a
segment above 5,000 events fails the run for investigation instead of growing.
