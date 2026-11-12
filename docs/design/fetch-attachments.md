# Attachments by content hash

Proposed. How a VBPL fetch item that makes a full read decides, file by file,
whether to copy an attachment it already holds or download it.
[Fetch reuse](fetch-reuse.md) owns when a full read happens, the baseline and
the control sample; [fetch bundles](fetch-bundles.md) owns the bundle and its
observations. Measured facts are in [fetch reuse](fetch-reuse.md#measured).

## Rule

A file's identity is its name in the file list and its content, which the source
states as an ETag. In a full read, for each listed file with a URL and a listed
size within the file limit:

1. Ask the source for its ETag: one `GET` of the presigned URL with
   `Range: bytes=0-0`, in the attachment pool (its in-flight limit and attempt
   deadline in [budgets](budgets.md)). The body is one byte and is discarded.
   The request goes through the same admission gate as a download: a 403 or a
   challenge pauses the source as for any request
   ([source pause](source-execution.md#source-pause)), and an admission refusal
   or a cancellation ends the item as it would a download; neither falls back to
   a download. A missing ETag, any other status than 206 (a 200 means the range
   was ignored; its body is not read), a transport failure or a timeout means
   the file is downloaded. The presigned URL comes from the file list read
   moments earlier in the same item, well inside its 24-hour validity.
2. Candidates are the baseline bundle's file with the same name and the local
   cache's entry for the same document id and name, in that order. A name that
   occurs more than once in the file list or in the baseline has no candidate. A
   candidate whose size differs from the listed size is skipped.
3. The first rule that matches a candidate decides; the ETag is compared without
   its surrounding quotes:
   1. Recorded: `source_etag` holds this document, name and exact ETag string,
      in any form, for the candidate's SHA-256. Copy.
   2. MD5 form, exactly 32 lowercase hexadecimal characters: the candidate's MD5
      equals it. Copy and record the ETag.
   3. Multipart form, 32 lowercase hexadecimal characters, `-` and a part count
      `n`: for each part size of 5, 8, 16, 32 and 64 MiB that splits the
      candidate's size into exactly `n` parts, the S3 multipart ETag of the
      candidate (the MD5 of the concatenated binary MD5s of its parts, then
      `-n`) equals it. Copy and record the ETag.
   4. Otherwise download the file and record the ETag of that download.
4. A copy is accepted only from its own bytes: the copy pass computes SHA-256,
   MD5 and, under rule 3, the multipart ETag, and the copy stands only when the
   SHA-256 equals the candidate's name and, under rule 2 or 3, the recomputed
   value equals the source ETag (under rule 1 the SHA-256 is the recorded one).
   Otherwise the stored rows for that SHA-256 are corrected or removed, the case
   is counted as `candidate_unverified` and the file is downloaded. The
   `file_md5` and `source_etag` rows only select candidates; they never stand in
   for the bytes.

Recording binds an ETag to bytes, never to a guess. The workshop table
`source_etag` (`document_key`, `name`, `etag`, `sha256`; primary key on the
first three) gets a row after a match under rule 2 or 3, or from the `ETag`
header of the download response whose bytes were stored, when that header is an
MD5 equal to the bytes' MD5 or any other form. The ranged response's ETag alone
is never recorded, because the file can change between that request and a
download; an MD5-form download ETag that differs from the bytes' MD5 records
nothing and is counted as `etag_download_differs`. Rows are never updated: a new
ETag for the same name is a new row, so a file that changes back to an earlier
version matches its earlier row. With these rows, a file whose ETag is neither
MD5 nor a known multipart shape costs one download, once, and is copied in every
later run.

MD5s of stored files live in the workshop table `file_md5` (`sha256` primary
key, `md5`). Every download and every cache or baseline copy computes MD5 and
SHA-256 in the same pass over the bytes and writes the row. A stored file
without a row is read once when it is first a candidate, computing both digests;
its SHA-256 must equal its name, and then the row is written. No separate
backfill is needed: the first run after the change fills the rows for the files
it compares, about 5.4 GB of reading once. Both tables travel in the workshop
dump.

Copied files keep today's observations: `adopted` with `origin_ref` from the
baseline, `cache` from the local cache. The file list's `updatedAt` no longer
decides anything; the content hash does.

Trust: the ETag is the source's own statement about bytes it serves, and MD5
(plain or multipart) is not collision resistant. A source able to craft
collisions could make fetch keep older bytes under an unchanged name; the same
source can serve any bytes on a download, so this adds no new trust, only the
residual risk of a forged collision, which is accepted.

## Checks

The primary runs these; they are requirements, not results.

- Attachments in a full read, one fixture per rule: a recorded ETag of any form
  copies; an MD5 ETag equal to the baseline's or the cache's MD5 copies and
  records; a multipart ETag computed from the candidate at each listed part size
  copies and records, and one with no consistent part size downloads; any other
  ETag downloads once, records the download's ETag and copies in the next run; a
  missing ETag, a non-206 answer and a failed ranged request download without a
  row; a size mismatch, a duplicate name and a candidate whose SHA-256 fails
  download; each is counted.
- `source_etag`: never written from the ranged response alone; an MD5 download
  ETag that differs from the bytes records nothing; a changed-then-reverted file
  matches its earlier row; survives checkpoint and restore.
- `file_md5`: written by downloads and copies, filled on first comparison for a
  stored file without a row, survives checkpoint and restore.
- ETag requests: a 403 pauses the source with the gate on, as a download does;
  an admission refusal and a cancellation end the item without a download.
- A poisoned `file_md5` or `source_etag` row (wrong digest for a SHA-256) is
  caught by the copy pass, counted, corrected and followed by a download, also
  after checkpoint and restore.
