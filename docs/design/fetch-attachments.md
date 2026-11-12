# Attachments by content hash

Proposed. How a VBPL fetch item that makes a full read decides, file by file,
whether to copy an attachment it already holds or download it, and what a failed
download costs. [Fetch reuse](fetch-reuse.md) owns when a full read happens, the
baseline and the control sample; [fetch bundles](fetch-bundles.md) owns the
bundle. Measured facts are in [fetch reuse](fetch-reuse.md#measured).

## Rule

A file is the same file when its name and its content hash are the same. The
source states the hash: the ETag of the object behind the file list's presigned
URL. No time decides anything.

In a full read, for each listed file with a URL and a listed size within the
file limit:

1. Ask the source for the ETag: one `GET` of the presigned URL with
   `Range: bytes=0-0`, in the attachment pool ([budgets](budgets.md)). The one
   byte is discarded. The request passes the same admission gate as a download,
   so a 403 or a challenge pauses the source
   ([source pause](source-execution.md#source-pause)).
2. Candidates are the baseline bundle's file with the same name, then the local
   cache's entry for the same document id and name. A name listed twice has no
   candidate; a candidate whose size differs from the listed size is skipped.
3. Copy the candidate when the ETag, without its quotes, equals a hash computed
   from the candidate's bytes:
   - MD5 form, 32 lowercase hexadecimal characters: the candidate's MD5.
   - Multipart form, 32 lowercase hexadecimal characters, `-` and a part count
     `n`: the S3 multipart ETag of the candidate (the MD5 of the concatenated
     binary MD5s of its parts, then `-n`) for a part size of 16 MiB or 5 MiB
     that splits the candidate's size into exactly `n` parts.
4. Otherwise download the file: no candidate, the hash differs, the ETag has
   another form or is missing, or the ranged request did not answer 206. Each
   reason is counted.

The copy pass reads the candidate once and computes SHA-256 with the hash it
needs; the copy stands only when the SHA-256 equals the candidate's file name
and the computed hash equals the ETag. A stored row never stands in for the
bytes.

The workshop table `file_hash` (`sha256` primary key, `md5`, `etag_16`,
`etag_5`, the two multipart values null for files that fit one part) caches
those hashes so a later run compares without reading the file. Every download
and copy writes its row in the same pass; a stored file without a row is read
once when it is first a candidate. The table travels in the workshop dump and is
only a cache: a row that disagrees with the bytes is corrected by the copy pass
and the file is downloaded.

A file whose ETag matches neither form is downloaded in every run. The measured
sample has none; if a run reports them in number, the rule gains the part size
that explains them, not a second mechanism.

MD5 is not collision resistant. The ETag is the source's own statement about
bytes it serves, and the same source can serve any bytes on a download, so the
rule adds no trust in the source beyond what a download already has.

## Download deadline and failures

Owner rule (2026-11-12): a source error costs at most one fast retry, then a
gap. One document or one file never holds or fails a stage. The rule covers
every source the pipeline reads, VBPL and chinhphu, with one table:

| What the source did                                                  | Retry           | Then                                                                            |
| -------------------------------------------------------------------- | --------------- | ------------------------------------------------------------------------------- |
| Timeout, transport error, HTTP 5xx, non-JSON 200 from a JSON call    | once, after 2 s | gateway or page call: gap `source_unavailable`; attachment: listed in `missing` |
| Any other unexpected status, a body whose size differs from the list | none            | the same gap or `missing`                                                       |
| HTTP 404 on an attachment                                            | none            | listed in `missing`, as today                                                   |
| HTTP 429                                                             | see below       | the item stays pending and its batch retries: a load signal                     |
| HTTP 403 or a challenge                                              | none            | [source pause](source-execution.md#source-pause): the item fails, the run stops |
| Valid JSON with invalid fields, oversize response                    | none            | `invalid_source_data`, `response_too_large`, as today                           |
| A body that is not the declared kind (an invalid PDF, a wrong type)  | none            | the existing source-data gap of [fetch bundles](fetch-bundles.md), not missing  |

An item ends `failed` or retries its batch only for an error of our own side
(disk, database, a cancelled activity), for HTTP 429 and for a source pause. Any
other source outcome not listed here is treated as the second row, so no new
source behaviour can reach the batch retry.

HTTP 429, every client: before each retry wait the later of the scheduled
backoff (2, 8, then 30 seconds) and a valid `Retry-After`; at most 3 retries.
The waits of one request sum to at most 200 seconds, and each attempt keeps its
own deadline, the size formula below for a download. When the next wait does not
fit in what is left of the 200 seconds, or the third retry answers 429 again,
the item returns pending to its batch; it never becomes a gap or a timeout for
that reason. A `Retry-After` above 10 minutes is a `source_unavailable` gap, as
today.

- Deadline per download attempt: 30 seconds plus the listed size at 250 kB/s, so
  20 MB gets 110 seconds and the 50 MB file limit 230 seconds; an unknown size
  counts as the limit. Eight downloads at 250 kB/s are 2 MB/s together, half the
  4 MB/s per address the file host allows. The fixed 30 seconds this replaces
  failed four PDFs of 15 to 21 MB five times each in run `vn/20261112_2`.
  chinhphu downloads use the same formula in place of their fixed 120 seconds.
- A retry starts from the first byte.
- A file listed in `missing` gives the document gap `missing_attachment` with
  the file name as its detail, exactly as a 404 does today, and the item commits
  done with its other files. The nonempty `missing` list makes the next run read
  the document in full.
- The cause of a failure (`timeout`, `transport`, `http_<status>`, `size`) is
  counted in `fetch_reads` and logged with the document key and file name. It is
  not stored in the bundle or the gap, so `report.json`, the fetch recipe and
  the gaps that adoption and rewind rebuild from the bundle do not change.
- A failed ETag request is not retried: the file is downloaded.

## Checks

The primary runs these; they are requirements, not results.

- One fixture per outcome of the rule: an MD5 ETag equal to the baseline's MD5
  copies with no download; the same from the local cache; a multipart ETag
  computed from the candidate with 16 MiB parts and with 5 MiB parts copies; a
  differing MD5, a multipart ETag no listed part size explains, another form, a
  missing ETag, a non-206 answer and a failed ranged request download; a size
  mismatch and a duplicate name download. Each outcome is counted.
- A copied and a downloaded attachment give byte-identical `files.json` and
  bundle.
- `file_hash`: written by downloads and copies, filled on first comparison,
  survives checkpoint and restore; a wrong row is corrected and the file is
  downloaded.
- An ETag request answered 403 pauses the source with the gate on.
- Deadline and failures: a 20 MB fixture served at 250 kB/s finishes inside its
  deadline; one fixture per row of the table, for a VBPL gateway call, a VBPL
  attachment and a chinhphu page and PDF, ends in the stated outcome after the
  stated number of tries; a download that fails twice records `missing`, a
  `missing_attachment` gap and a done item holding the other files, and the same
  gap is produced again on adoption; no source outcome leaves an item `failed`
  or retries a batch except HTTP 429 and a source pause.
- A live run on unchanged sources downloads no attachment whose ETag has MD5 or
  multipart form, and reports the count and size of files of other forms.
