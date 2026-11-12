# Reuse of unchanged VBPL reads across runs

Proposed. A new run reads every VBPL document in full although almost nothing
changed since the previous run. This note lets fetch keep the tree, the file
list and the attachments of a document whose `updatedDate` did not move, and
copies any attachment whose name and content hash equal a stored copy instead of
downloading it again. It extends [fetch bundles](fetch-bundles.md) and the
[cache rule](pipeline.md#source-ingest-rules); [retry](source-retry.md) still
owns reuse inside one run. Only VBPL is covered; chinhphu fetch is unchanged.

## Measured

Runs `vn/20261110_6` and `vn/20261111_2`, one day apart, same scope:

| Fact                                                    | Count              |
| ------------------------------------------------------- | ------------------ |
| Documents fetched in both runs                          | 3,833              |
| Bundles byte-identical                                  | 3,767              |
| Changed bundles examined (VBPL)                         | 57                 |
| of those, only `data.viewCount` or the diagram differs  | 52                 |
| of those, `data.updatedDate` moved                      | 1 (with `effTo`)   |
| Trees changed while `updatedDate` stayed                | 3, node ids only   |
| File lists or attachments changed without `updatedDate` | 0                  |
| Details with null `updatedDate` (both runs)             | 382 of 7,297, 5%   |
| Bundles with a missing attachment                       | 20 of 7,297        |
| Gateway calls in one fetch pass                         | 15,328 (4 per doc) |
| Attachment downloads in one fetch pass                  | 29,199, 5.4 GB     |
| Attachments taken from the local cache                  | 3,582              |
| Fetch stage time, fresh pass                            | 21 min             |

The three changed trees differ only in node `key` and `id` values that the
source issued again; their text is identical. Node ids are used only inside one
bundle (extract links parents to children); nothing asks the source by node id.

The gateway offers no cheaper signal. Detail, tree, diagram and file list
responses carry `cache-control: no-cache, no-store`, no `ETag` and no
`Last-Modified` (probed 2026-11-12), so conditional requests do not exist. The
listing has an `updatedDate` key, null in all 463,684 stored listing items. The
detail call is the only place to read it.

Attachments do carry a content hash (probed 2026-11-12). The file list gives
each file a presigned GET URL on `s3-han02.fptcloud.com`, valid 24 hours
(`X-Amz-Expires=86400`). A `GET` of that URL with `Range: bytes=0-0` answers 206
with `ETag` and `Last-Modified`; `HEAD` answers 403, because the signature
covers `GET` only. The list's own `etag` field is null. For `vn:vbpl:171687` the
ETags of all 8 files equal the MD5 of the stored copies, the 8.5 MB original PDF
included. <!-- corpus-wide counts: filled from the full probe -->

An earlier version of this note reused attachments only while their first
network read was younger than an age limit, and treated an unknown origin time
as expired. Run `vn/20261112_1` copied 2,391 attachments of 1,383 documents from
the local cache, which records no origin time, so the next run, `vn/20261112_2`,
had read 803 of its first 3,094 documents in full for age and downloaded 7,592
attachments (3.6 GB) again. Content hashes replace that rule.

One day and two runs is a small sample. The [control](#control) exists because
of that.

## Stored detail drops the view counter

`data.viewCount` is a page counter. It changed about 30 bundles in one day and
with them their chunk and embedding inputs, for no legal content.

Fetch removes the `viewCount` member of the top-level `data` object from the
detail bytes before hashing and storing them. The document is not decoded and
encoded again; every byte outside the deleted span stays as the source sent it.
The deleted span is exact:

- When a comma follows the member's value, after optional whitespace: from the
  opening quote of the key through that comma. Whitespace after the comma stays.
- Otherwise, when a comma precedes the key (the member is last): from that comma
  through the last byte of the value. Whitespace before the comma and after the
  value stays.
- Otherwise (sole member): from the opening quote of the key through the last
  byte of the value.

A key matches when its decoded string equals `viewCount`, so an escaped spelling
matches too. Matching members of `data` are removed one at a time, leftmost
first, each by the rules above applied to the bytes left by the previous
removal: `{"data":{"x":0,"viewCount":1 ,"viewCount":2}}` becomes
`{"data":{"x":0}}`. A `viewCount` anywhere else, a detail without the member and
a `data` that is not an object leave the bytes unchanged. Malformed JSON is a
source data error, as today. The observation hash and size describe the stored
bytes. The VBPL part of the fetch recipe becomes `vbpl-fetch/2`.

Cost, once: every VBPL fetch manifest changes, so the next run repeats extract,
chunk and embed for all documents. This ships first, as its own change, and the
baseline rule below starts from bundles written under the new recipe.

## Baseline

The baseline of a document is its most recently committed VBPL fetch bundle
under the same fetch recipe hash, from any run of the same country.

- The workshop table `fetch_baseline`, keyed by
  `(document_key, fetch_recipe_hash)`, references the artifact and the commit
  that produced it: run, item key, generation and attempt. That tuple selects
  one observation set, because identical bundles from several runs share an
  artifact but not their observations. The successful done-item commit of a VBPL
  fetch item writes it in the same transaction, after the generation, attempt
  and source-pause checks; the last commit wins. An item that adopted its own
  run's acquisition writes nothing. A gap or failure writes nothing and keeps
  the older baseline.
- The table travels in the workshop dump. A missing row means a full read.
- A baseline is never inferred from a directory without a committed item.

Inside one run [acquisitions](source-retry.md#acquisitions) come first: an item
with an acquisition adopts it and makes no source call. The baseline is read
only when the item reads the source.

## Read rule

For each VBPL item that reads the source:

1. Read detail and diagram, two gateway calls, as today. Identity checks and
   error handling are unchanged.
2. Reuse the baseline's `tree.json`, `files.json`, `report.json` and attachment
   files when every condition holds:
   - the whole baseline bundle verifies against its manifest, and its
     `detail.json` names the same document;
   - the fresh `data.updatedDate` is a non-null string and equals the baseline's
     byte for byte (null on either side is a full read);
   - the baseline `report.json` has an empty `missing` list, so a late
     attachment is found (`too_large` entries do not block reuse);
   - the item is not in this run's control sample.
3. Otherwise make a full read: tree and file list, two more gateway calls, then
   each listed attachment by the rule below.
4. Publish the bundle from the fresh detail and diagram and the reused or fresh
   remainder. Equal bytes give the equal manifest hash and the existing
   artifact.

A failed verification is logged with run, key and reason and becomes a full
read; storage that cannot be read at all stays an infrastructure error. No
condition reads a time: origin times stay recorded in the observations and
nothing else uses them.

## Attachments by content hash

A file's identity is its name in the file list and the MD5 of its content. In a
full read, for each listed file with a URL and a listed size within the file
limit:

1. Ask the source for its ETag: one `GET` of the presigned URL with
   `Range: bytes=0-0`, in the attachment pool (its in-flight limit and attempt
   deadline in [budgets](budgets.md)). The body is one byte and is discarded.
2. The ETag is a content hash only when, without its surrounding quotes, it is
   exactly 32 lowercase hexadecimal characters. A missing ETag, any other form
   (an S3 multipart ETag ends in `-<parts>`), a status other than 206, or a
   failed request means the file is downloaded.
3. Candidates are the baseline bundle's file with the same name and the local
   cache's entry for the same document id and name, in that order. A name that
   occurs more than once in the file list or in the baseline has no candidate. A
   candidate whose size differs from the listed size is skipped.
4. A candidate whose MD5 equals the ETag is copied once its SHA-256 verifies, as
   today. Otherwise the file is downloaded. After a download, the file's MD5 is
   compared with an MD5-form ETag; a difference is counted as
   `etag_download_differs` and the downloaded bytes are kept.

MD5s of stored files live in the workshop table `file_md5` (`sha256` primary
key, `md5`), which travels in the workshop dump. Every download and every cache
or baseline copy computes MD5 and SHA-256 in the same pass over the bytes and
writes the row. A stored file without a row is read once when it is first a
candidate, computing both digests; its SHA-256 must equal its name, and then the
row is written. No separate backfill is needed: the first run after the change
fills the rows for the files it compares, about 5.4 GB of reading once.

Copied files keep today's observations: `adopted` with `origin_ref` from the
baseline, `cache` from the local cache. The file list's `updatedAt` no longer
decides anything; the content hash does.

Trust: the ETag is the source's own statement about bytes it serves, and MD5 is
not collision resistant. A source able to craft collisions could make fetch keep
older bytes under an unchanged name; the same source can serve any bytes on a
download, so this adds no new trust, only the residual risk of a forged
collision, which is accepted.

## Control

Each run re-reads a deterministic sample in full: the items where the first 8
bytes of `sha256(document_key || 0x00 || rel)`, read as an unsigned integer, are
divisible by the control divisor in [budgets](budgets.md). The sampled item
makes the full read above, with an ETag request for every file, so it downloads
only files whose hash differs or cannot be verified. When the item would
otherwise have reused its baseline, fetch compares the two and records one
outcome:

| Outcome         | Meaning                                                                |
| --------------- | ---------------------------------------------------------------------- |
| `same`          | `tree.json`, `files.json` and `report.json` byte-identical             |
| `tree_ids_only` | Trees equal after removing node `key` and `id`; rest identical         |
| `changed`       | Anything else, including a file whose ETag differs from the stored MD5 |

Per file it counts `control_file_same` (ETag equals the stored MD5),
`control_file_changed` (it differs) and `control_file_unverified` (no usable
ETag). Every `changed` outcome is logged at WARN with the document key and the
files that differ. A `changed` outcome does not fail the run; the primary reads
the document and decides whether the reuse rule must narrow.

## Counts

The run message carries `fetch_reads`: reused; full reads by reason
(`no_baseline`, `invalid_baseline`, `null_date`, `date_changed`,
`missing_attachment`, `control`); the control outcomes and per-file control
counts; and per attachment of a full read: copied from the baseline, copied from
the cache, and downloaded by reason (`no_candidate`, `etag_differs`,
`etag_missing`, `etag_not_md5`, `etag_request_failed`, `candidate_unverified`),
with `etag_download_differs`.

## Observations and times

Detail and diagram observations are `network`, as today. A reused tree or
attachment is recorded with acquisition `adopted`; its origin is the observation
with the same path and content hash in the baseline's producing commit, whose
time it copies, with `origin_ref` pointing at it. A cache copy keeps a null
time. `fetched_file.fetched_at` follows
[fetch bundles](fetch-bundles.md#observation-commit-and-recovery). These times
are audit evidence only. Bundles carry no time, so reuse cannot change a hash.

## Recipe

Neither the read rule nor the file rule is a fetch recipe input
(`apps/corpus/internal/stage/fetch/fetch.go`, `Recipe`), and a copied file gives
the same name, SHA-256, size, path and URL in `files.json` as a download, so
bundles keep their bytes and today's baselines stay valid. The fetch recipe does
not change.

## Effect

<!-- filled from the full probe and run vn/20261112_2 -->

Known cost left in place: a diagram-only change (37 documents in the measured
day) changes the fetch manifest, and [chunk](chunk.md#stage-and-artifact) binds
`fetch_sha256`, so those documents repeat extract, chunk and embed although only
relate needs the new diagram. Narrowing the chunk input is a separate change.

## Checks

The primary runs these; they are requirements, not results.

- `viewCount`: two details that differ only in the counter give identical stored
  bytes and manifest; expected-byte fixtures for the member first, in the
  middle, last and alone, with and without whitespace, duplicated and with an
  escaped key; a detail without the member is unchanged.
- Reuse: with an equal `updatedDate` the fake source sees exactly two calls and
  no download, and the bundle equals the full-read bundle byte for byte; an old
  baseline (any age, unknown origin times included) is reused.
- Each blocking condition alone forces the full read: no baseline, null date on
  either side, moved date, nonempty `missing`, corrupted baseline file, other
  recipe hash, control sample.
- Attachments in a full read: an ETag equal to the baseline's MD5 copies it with
  no download; equal to the cache's MD5 copies from the cache; a different ETag,
  a missing ETag, a multipart ETag, a non-206 answer and a failed ranged request
  each download, and each is counted; a size mismatch and a duplicate name
  download; a candidate whose SHA-256 fails downloads.
- `file_md5`: written by downloads and copies, filled on first comparison for a
  stored file without a row, survives checkpoint and restore.
- Control: the sample is stable for a `(key, rel)` pair; a prepared baseline
  gives `same`, `tree_ids_only` and `changed`, and a file the fake source
  changed without moving `updatedDate` gives `control_file_changed` and is
  downloaded.
- Baseline row: written only by a committed done item that read the source; a
  gap, a failure, a stale attempt and an adopted acquisition leave it unchanged;
  it survives checkpoint and restore.
- Restart of a run still adopts its own acquisitions with no source call.
- The fetch recipe hash and the bytes of recorded bundles are unchanged.
- A live run after a baseline run reports its counts in the research note,
  beside the fetch stage time and the ETag pass time.
