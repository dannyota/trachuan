# Reuse of unchanged VBPL reads across runs

Proposed. A new run reads every VBPL document in full although almost nothing
changed since the previous run. This note lets fetch keep the tree, the file
list and the attachments of a document whose `updatedDate` did not move. It
extends [fetch bundles](fetch-bundles.md) and the
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
   files when every condition holds. Otherwise make the full read of today.
3. Publish the bundle from the fresh detail and diagram and the reused or fresh
   remainder. Equal bytes give the equal manifest hash and the existing
   artifact.

Conditions for reuse, all required:

- The whole baseline bundle verifies against its manifest, and its `detail.json`
  names the same document.
- The fresh `data.updatedDate` is a non-null string and equals the baseline's
  byte for byte. Null on either side is a full read.
- The baseline `report.json` has an empty `missing` list, so a late attachment
  is found. `too_large` entries do not block reuse.
- The oldest origin time among the reused files is known and not older than the
  reuse age limit in [budgets](budgets.md).
- The item is not in this run's control sample.

A failed verification is logged with run, key and reason and becomes a full
read; storage that cannot be read at all stays an infrastructure error.

In a full read for any reason other than the control sample or the age limit,
the existing attachment cache rule also accepts the baseline bundle as a cache:
an attachment whose document id, name and size match a baseline file with a
known origin time no earlier than the gateway's `updatedAt` for that file is
copied once its sha256 verifies. Documents with a null `updatedDate` then cost
four gateway calls and no download. A file whose baseline origin time is unknown
or older than the age limit is never copied, from the baseline or from the local
cache: an expired document downloads every attachment again.

What stays fresh in every run: the detail, with status, dates and `effTo`, and
the diagram, with all relations. What can be stale: a tree, file list or
attachment that the source changed without moving `updatedDate`. The age limit
bounds that, and the control measures it.

## Control

Each run re-reads a deterministic sample in full, with real downloads: the items
where the first 8 bytes of `sha256(document_key || 0x00 || rel)`, read as an
unsigned integer, are divisible by the control divisor in [budgets](budgets.md).
The full read is the item's result. When the item would otherwise have reused
its baseline, fetch compares the two and records one outcome:

| Outcome         | Meaning                                                        |
| --------------- | -------------------------------------------------------------- |
| `same`          | `tree.json`, `files.json` and `report.json` byte-identical     |
| `tree_ids_only` | Trees equal after removing node `key` and `id`; rest identical |
| `changed`       | Anything else: the rule would have kept stale content          |

Every `changed` outcome is logged at WARN with the document key and the files
that differ. The run message carries the fetch counts: reused, full reads by
reason (`no_baseline`, `null_date`, `date_changed`, `missing_attachment`, `age`,
`invalid_baseline`, `control`) and the three control outcomes. A `changed`
outcome does not fail the run; the primary reads the document and decides
whether the rule must narrow. The age limit makes every document a full read at
least once per limit, whatever the sample chose.

## Observations and times

Detail and diagram observations are `network`, as today. A reused tree or
attachment is recorded with acquisition `adopted`. Its origin is the observation
with the same path and content hash in the baseline's producing commit: the new
row copies that observation's time and points `origin_ref` at it. Chains keep
the first network time, so the age limit counts from the real read. An unknown
origin time stays null and fails the age condition. `fetched_file.fetched_at`
follows the existing rule in
[fetch bundles](fetch-bundles.md#observation-commit-and-recovery). Bundles still
carry no time, so reuse cannot change a hash.

## Effect

Expected for a run one day after its baseline, from the counts above (3,832 VBPL
documents, divisor 20):

| Group                               | Documents | Gateway calls | Downloads |
| ----------------------------------- | --------- | ------------- | --------- |
| Reused                              | ~3,440    | 2 each        | 0         |
| Null `updatedDate`, date moved, gap | ~200      | 4 each        | few       |
| Control sample                      | ~190      | 4 each        | ~1,450    |
| Total                               | 3,832     | ~8,450        | ~1,500    |

About 55% of today's gateway calls and 5% of today's downloads. The first run
under a new fetch recipe, and the first run after the age limit, is a full pass.

Known cost left in place: a diagram-only change (37 documents in the measured
day) changes the fetch manifest, and [chunk](chunk.md#stage-and-artifact) binds
`fetch_sha256`, so those documents repeat extract, chunk and embed although only
relate needs the new diagram. Narrowing the chunk input is a separate change.

## Checks

The primary runs these; they are requirements, not results.

- `viewCount`: two details that differ only in the counter give identical stored
  bytes and manifest; expected-byte fixtures for the member first, in the
  middle, last and alone, with and without whitespace, duplicated (the case
  above) and with an escaped key; a detail without the member is unchanged.
- Reuse: with an equal `updatedDate` the fake source sees exactly two calls and
  no download, and the bundle equals the full-read bundle byte for byte.
- Each blocking condition alone forces the full read: no baseline, null date on
  either side, moved date, nonempty `missing`, age over the limit, unknown
  origin time, corrupted baseline file, other recipe hash.
- A full read with a null date takes matching attachments from the baseline and
  downloads one whose gateway `updatedAt` is later than its origin time.
- A document past the age limit downloads every attachment: the fake source
  serves new bytes under an unchanged name, size and `updatedAt`, and the bundle
  holds the new bytes.
- Two runs commit byte-identical bundles at different times; a third run that
  reuses takes time and `origin_ref` from the later producing commit, also after
  checkpoint and restore.
- Control: the sample is stable for a `(key, rel)` pair, makes real downloads,
  and reports `same`, `tree_ids_only` and `changed` for prepared baselines.
- Baseline row: written only by a committed done item that read the source; a
  gap, a failure, a stale attempt and an adopted acquisition leave it unchanged;
  it survives checkpoint and restore.
- Restart of a run still adopts its own acquisitions with no source call.
- A live run after a baseline run reports its counts in the research note,
  beside the fetch stage time.
