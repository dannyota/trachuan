# VBPL attachment ETags

Observed 2026-11-12 from this MacBook, after run `vn/20261112_2` was cancelled,
so no other request ran. Evidence for
[attachments by content hash](../design/fetch-attachments.md).

## Method

For each of the 4,259 VBPL documents of `vn/20261112_2`, the probe read the
gateway file list (`doc/minio/buckets/vbpl/folders/<id>/files`) at 8 calls a
second with the browser User-Agent, then sent one `GET` with `Range: bytes=0-0`
to every presigned URL, 8 in flight. It compared each ETag with the stored copy
of the same document and file name from runs `vn/20261112_1` and `_2`: the MD5
for an MD5-form ETag, and the S3 multipart ETag computed at 5, 8, 16, 32 and 64
MiB parts for a multipart one. It recorded counts and timings only, never URLs.

## Results

| Fact                                    | Count or time                                             |
| --------------------------------------- | --------------------------------------------------------- |
| File lists answered 200                 | 4,259 of 4,259                                            |
| File list pass                          | 546 s (p50 0.14 s, p95 0.25 s), paced at 8/s              |
| Files listed                            | 36,541                                                    |
| Listed without a presigned URL          | 30                                                        |
| Ranged requests answered 206            | 36,508                                                    |
| Ranged requests answered 416            | 3, all listed with size 0                                 |
| ETag form MD5                           | 36,413                                                    |
| ETag form multipart (`<md5>-<n>`)       | 95, 5.96 to 42.6 MB                                       |
| No ETag                                 | 33 (the 30 and the 3 above)                               |
| Any other ETag form                     | 0                                                         |
| Stored copies compared                  | 32,787                                                    |
| MD5 ETag equal to the stored copy's MD5 | 32,701                                                    |
| MD5 ETag different                      | 0                                                         |
| Multipart equal at 16 MiB parts         | 68                                                        |
| Multipart equal at 5 MiB parts          | 15                                                        |
| Multipart matching no part size         | 0                                                         |
| Multipart files without a stored copy   | 12                                                        |
| Listed files without a stored copy      | 3,754                                                     |
| ETag pass                               | 580 s at 8 in flight (p50 0.11 s, p95 0.18 s, max 1.17 s) |

The list's own `etag` field was null for every file. No ranged request failed or
timed out.

## Reading

Every stored file with an ETag is explained by its MD5 or by the multipart ETag
at 16 or 5 MiB parts, so those two part sizes are enough. An ETag request costs
about 0.13 s of pool time; a full re-read of every document would spend about 10
minutes on them, while the reuse rule's full reads (about 300 documents and
2,500 files a run) spend about 40 s. Empty files cannot be probed with a
one-byte range and have nothing to download.
