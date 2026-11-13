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
each file a presigned GET URL on `s3-han02.fptcloud.com`, valid 24 hours. A
`GET` of that URL with `Range: bytes=0-0` answers 206 with `ETag`; `HEAD`
answers 403, because the signature covers `GET` only. The list's own `etag`
field is null. Over every VBPL document of `vn/20261112_2` (4,259 documents,
36,541 listed files; [ETag probe](../research/vbpl-etag-2026-11.md)): 36,413
ETags have MD5 form, 95 multipart form, 33 none (30 files without a URL, 3 empty
files that answer 416), 0 another form. Of 32,787 stored copies, 32,701 equal
their MD5 ETag and 0 differ; 83 multipart ETags equal the value computed from
the stored copy, 68 with 16 MiB parts and 15 with 5 MiB parts, and 0 are
unexplained. The ETag pass took 580 seconds at 8 in flight.

An earlier version of this note reused attachments only while their first
network read was younger than an age limit, and treated an unknown origin time
as expired. Run `vn/20261112_1` copied 2,391 attachments of 1,383 documents from
the local cache, which records no origin time, so the next run, `vn/20261112_2`,
had read 803 of its first 3,094 documents in full for age and downloaded 7,592
attachments (3.6 GB) again. Content hashes replace that rule. Across every
stored VBPL fetch bundle (11,224 bundles of 4,577 documents written 2026-11-09
to 11-12, `fetched_file` joined to `artifact`, distinct SHA-256 per item and
file name), 32,794 files appear in two or more bundles under the same document
and name, and exactly one changed content: `vn:vbpl:13205` `13205.xml`, 3,298 to
3,380 bytes on 2026-11-11; no PDF, Word or original-text file changed. The
16,863 attachments (8.65 GB) that runs `vn/20261112_1` and `_2` downloaded for
age include none whose bytes differ from a stored copy of the same document and
name. The age rule never detected a change and cost about 5 GB a run.

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
   error handling are unchanged. A seed whose expand item is done in the same
   run takes both from that expand artifact instead, detail with `viewCount`
   removed as below; they are minutes to hours old, and a change after expand is
   caught by the next run. Such a seed counts in `reused`, `null_date`,
   `date_changed` and `missing_attachment` exactly as a fresh read would.
   Control-sample items always read both from the source, so `control_same`
   keeps comparing against fresh bytes.
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

In a full read every listed attachment is copied from a stored copy or
downloaded under [attachments by content hash](fetch-attachments.md): the
source's ETag, read with a one-byte ranged request, decides; no time does.

## Control

Each run re-reads a deterministic sample in full: the items where the first 8
bytes of `sha256(document_key || 0x00 || rel)`, read as an unsigned integer, are
divisible by the control divisor in [budgets](budgets.md). The sampled item
makes the full read above, with an ETag request for every file, so it downloads
only files whose hash differs or cannot be compared. When the item would
otherwise have reused its baseline, fetch compares the two and records one
outcome:

| Outcome         | Meaning                                                                                                                       |
| --------------- | ----------------------------------------------------------------------------------------------------------------------------- |
| `same`          | `tree.json`, `files.json` and `report.json` byte-identical                                                                    |
| `tree_ids_only` | Trees equal after removing node `key` and `id`; rest identical                                                                |
| `changed`       | Anything else, including a file that does not match its stored copy under [attachments by content hash](fetch-attachments.md) |

A file of a sampled document that is downloaded because its hash differs is
counted `control_file_changed`. Every `changed` outcome is logged at WARN with
the document key and the files that differ. A `changed` outcome does not fail
the run; the primary reads the document and decides whether the reuse rule must
narrow.

## Counts

The run message carries `fetch_reads`: reused; full reads by reason
(`no_baseline`, `invalid_baseline`, `null_date`, `date_changed`,
`missing_attachment`, `control`); the control outcomes; and per attachment of a
full read: copied (`etag_md5`, `etag_multipart`, each from baseline or cache)
and downloaded by reason (`no_candidate`, `etag_differs`, `etag_other_form`,
`etag_unavailable`, `candidate_unverified`).

## Observations and times

Detail and diagram observations are `network`, as today; taken from the same
run's expand artifact they are `adopted` with null `fetched_at` and
`origin_ref`. A reused tree or attachment is recorded with acquisition
`adopted`; its origin is the observation with the same path and content hash in
the baseline's producing commit, whose time it copies, with `origin_ref`
pointing at it. A cache copy keeps a null time. `fetched_file.fetched_at`
follows [fetch bundles](fetch-bundles.md#observation-commit-and-recovery). These
times are audit evidence only. Bundles carry no time, so reuse cannot change a
hash.

## Recipe

Neither the read rule nor the file rule is a fetch recipe input
(`apps/corpus/internal/stage/fetch/fetch.go`, `Recipe`). `files.json` stays the
array of `{name,sha256,size}` and the file sits at `files/<sha256>` whether it
was copied or downloaded; presigned URLs and ETags are never in the bundle. So
bundles keep their bytes and today's baselines stay valid. The fetch recipe does
not change.

## Effect

A run on unchanged sources makes two gateway calls per reused document, four
plus one ETag request per file for a full read, and downloads only files whose
hash differs or cannot be compared. The first live run after the change reports
the counts and the fetch stage time in the research note.

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
- Control: the sample is stable for a `(key, rel)` pair; a prepared baseline
  gives `same`, `tree_ids_only` and `changed`, and a file the fake source
  changed without moving `updatedDate` gives `control_file_changed` and is
  downloaded.
- Baseline row: written only by a committed done item that read the source; a
  gap, a failure, a stale attempt and an adopted acquisition leave it unchanged;
  it survives checkpoint and restore.
- Restart of a run still adopts its own acquisitions with no source call.
- The fetch recipe hash and the bytes of recorded bundles are unchanged, and a
  copied attachment and a downloaded one give byte-identical `files.json`.
- A live run after a baseline run reports its counts in the research note,
  beside the fetch stage time and the ETag pass time.
- Attachment checks are listed in
  [attachments by content hash](fetch-attachments.md#checks).
