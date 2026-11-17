# Control frameworks: build

A frameworks release is built on the MacBook by one offline command,
`corpus frameworks build <rel>`, not by the Temporal stages of
[pipeline](pipeline.md) and not in the workshop database. The kind is small (a
few thousand controls) and its inputs are local files. The decision is in
[ADR 0016](../adr/0016-control-frameworks-corpus.md); the corpus kind is in
[control frameworks](frameworks.md); every file and database here is licensed
material and stays in the places of
[licensed material](frameworks-access.md#licensed-material-and-where-it-may-be).

## Steps

| Step       | Reads                    | Writes                                                                           |
| ---------- | ------------------------ | -------------------------------------------------------------------------------- |
| `snapshot` | inputs below             | a frozen copy of every input, hash-verified                                      |
| `parse`    | snapshot                 | controls per version (JSON Lines)                                                |
| `relate`   | snapshot, parse          | edges and evidence with dependency sets (JSON Lines)                             |
| `chunk`    | parse                    | body chunks and public-field records (JSON Lines)                                |
| `embed`    | chunk                    | vectors (float32, the law artifact format) and token counts                      |
| `build`    | snapshot through `embed` | the `frameworks_<rel>` database, its `pg_dump`, gap rows, build manifest         |
| `evaluate` | build                    | `report.json`: golden cases, the invariant suite, gap totals by kind and blocker |

**Inputs.** `snapshot` copies into the run, and hashes, everything later steps
consume: the registry, the source files of the versions in the release (through
the hash-checking reader), our paraphrased titles, machine OCR evidence for
scans, the reviewed waivers (`FRAMEWORKS_SOURCE_DIR/trachuan/ocr-waivers.yaml`,
its hash in the snapshot manifest), the golden cases with their review file, the
public floors (`sources/frameworks-floors.yaml`) and the private floors
([evaluation](frameworks-evaluation.md)) and the gap kind table. No later step,
and no resume, reads anything outside the snapshot, so a registry or title
change during a run cannot reach it.

The versions in a release are the registry versions that name a parser. Our
titles are one CSV, `FRAMEWORKS_SOURCE_DIR/trachuan/titles.csv` (`code`,
`version`, `citation`, `title`), outside Git like the waivers. A control of a
`free` version without a row takes the publisher's title as ours, since that
text is public. A control of a `licensed` version without a row has no title and
records a `title_missing` gap; its public-field record then carries citation and
kind only.

**Parse** runs each version's parser ([frameworks](frameworks.md#pipeline)).
Parse errors, a control-count mismatch, an invalid or duplicate citation and an
invalid lineage fail the step; they are not gaps.

**Relate** emits edges by business key with source and relation, and each
evidence value with the set of licensed versions whose text it uses, computed
from the snapshot's access classes. The set is not inferred from the evidence's
words: it holds every licensed version at either end of the edge, which fails
closed, so only evidence on an edge between free versions is public. A target
named in a source file resolves through registry codes and aliases. Its version
token, without a `v`, `Rev` or `Revision` prefix, names the registered version
equal to it, else the registered version that is its major number (`Rev 5.2.0`
names `r5`, `v8.0` names `v8`). An edge whose target version or either citation
is not in a parsed catalog stays unresolved and is counted; this includes a
target version the registry does not list, of a registered framework. Evidence
is kept only when both ends of its edge are versions in the release; other
evidence is not kept, because the release classifies no access for it, and
counts as `mapping_dropped`. A target that names no registered framework, or an
end that is not a valid business key (the schema's code, version and citation
forms), is not an edge: it is counted as `mapping_dropped` for its source and
version, so every stored edge end is a valid business key with a registry code.
A link between two controls of one version is not a mapping edge
([terms](frameworks.md#terms)): the step leaves it out, and it is not a gap.

**Chunk** applies the law split rule (size limits and cut points) to each
control body as one unit. That rule is extracted from the law chunk package into
one exported function both callers use; it is not copied. The extraction is its
own pull request, reviewed by the owner of the law chunk package, with a test
that law chunk output is byte-identical before and after and a test that the law
chunk recipe hash is unchanged, and merged only after the law corpus release in
progress at the time is signed. Until then the frameworks chunk step is written
against that function's intended signature, so nothing else waits. The legal
parts of [chunk](chunk.md) do not apply: no provision tree, node kinds,
minimum-length omission or cross-provision deduplication. A body chunk's context
prefix is framework, version, citation path and our title, never
`title_original`. Every control also gets exactly one public-field record
(citation, kind, our title), never cut.

Record schemas, named in each manifest and versioned with its recipe:

- `controls/1`: `code`, `version`, `citation`, `citation_norm`, `kind`, `parent`
  (citation or null), `ordinal`, `status`, `title` (ours), `title_original`,
  `body`, `notes[]`, `tables[]`, `supersedes`, `amends`.
- `edges/1`: `from` and `to` as `{code, version, citation}`, `relation`,
  `source`, `resolved`, and `evidence` with `requires[]` (version handles; empty
  means public) when present.
- `chunks/1`: `index` (`body` or `public`), `document_key`, `ordinal`,
  `citation`, `chunk_index`, `context_prefix`, `content`.

Embedding uses the law artifact unchanged, one per document key: per framework
version, the body index has `document_key` `fw:<code>:<version>` and the
public-field index `fwp:<code>:<version>`, with `ordinal` strictly increasing
over that version's chunks in control order, then chunk order. `chunks/1` maps
each ordinal back to its control. The `embed` step directory holds one law
artifact per document key and a manifest listing them.

**Embed** reuses the law embedding client and artifact format with the law dense
recipe, through the document embedder ([embedder](embedder.md)).

**Databases.** The build and evaluation databases are `frameworks_<rel>` and
`frameworks_<rel>_eval` on the MacBook's PostgreSQL. Every step that touches
them holds a PostgreSQL advisory lock keyed by the database name for its whole
duration, so two runs with different work directories cannot touch the same
database. Each database carries an ownership marker as its PostgreSQL comment
(`COMMENT ON DATABASE`: the work directory's real path, `<rel>` and the run id),
outside the schema; the dump uses the law dump's `--no-comments` option, so the
marker is in neither the dump nor the ordered export; a step drops and recreates
a database only when the marker names this work directory and `<rel>`, and stops
otherwise.

**Build** recreates `frameworks_<rel>`, applies schema version 1, loads
versions, seals them ([data model](frameworks.md#data-model)), loads controls,
text, chunks, vectors, lexical postings (the law [lexical](lexical.md)
encoding), edges, evidence and gaps, creates indexes, calibrates the abstention
threshold ([evaluation](frameworks-evaluation.md#the-measured-recipe)), writes
the `release` row, then dumps the database. The dump in the build directory is
the release.

**Evaluate** verifies the dump's hash, restores it into a freshly recreated
`frameworks_<rel>_eval`, checks the restore completed, runs on that copy only,
and writes the report the release manifest names. It never evaluates the build
database.

## Artifacts and provenance

Each step writes `FRAMEWORKS_WORK_DIR/<rel>/<step>/` on a local disk of the
MacBook, outside every sync folder: its outputs and a `manifest.json` with the
step's recipe (name, version, parameters), the SHA-256 of each input (snapshot
files or earlier manifests) and of each output. JSON Lines rows are serialized
canonically in a total order per schema: `controls/1` by (code, version order,
`citation_norm`), `edges/1` by (`from`, `to`, `relation`, `source`), `chunks/1`
by (`document_key`, `ordinal`).

The `release` row inside the dump records the recipes and input hashes of
`snapshot` through `embed`. The build manifest (dump SHA-256 and size, build
recipe) and the evaluation report live beside the dump as immutable sidecars,
and the [signed manifest](frameworks.md#release-and-deployment) binds them, so
nothing written after the dump is recorded inside it.

**Determinism.** `snapshot`, `parse`, `relate` and `chunk` outputs are
byte-identical for the same inputs and recipes. `embed` vectors are too, as the
document embedder is deterministic per input. `build` reproduces the database
contents (compared by an ordered export), not the dump's bytes, which carry
`built_at`; the released identity is the exact dump that was evaluated.
`evaluate` reproduces its metrics, not its timings.

## Runs, resume and failure

The command refuses a `FRAMEWORKS_SOURCE_DIR` or `FRAMEWORKS_WORK_DIR` whose
real path lies under a third-party sync folder, naming the folder in its error:
`~/Library/Mobile Documents/` (iCloud Drive), `~/Library/CloudStorage/`
(Dropbox, Google Drive, OneDrive and other File Provider services),
`~/Dropbox/`, `~/Google Drive/`, `~/OneDrive` and its variants, and `~/Desktop/`
and `~/Documents/` (synced when iCloud Desktop and Documents is on). The
prefixes are a first guard; the command also refuses a path under any sync root
the installed clients report (the `path` entries of `~/.dropbox/info.json`) and
a path with an ancestor directory holding a `.dropbox` marker, which covers sync
roots on external drives. The real path is resolved from the value as given,
symlinks before any `..`, and the check applies to every directory the command
reads or writes (source, work, `<rel>`, step and temporary directories), each
checked when first used; all reads and writes then go through the checked real
path, never the original value.

One command runs per `<rel>`: it holds an exclusive lock file
`FRAMEWORKS_WORK_DIR/<rel>/.lock` for the whole run. Under the lock it first
removes leftover `.tmp-*` directories in that `<rel>` directory. A step writes
into `FRAMEWORKS_WORK_DIR/<rel>/.tmp-<step>-<run id>` on the same filesystem,
syncs its files and the directory, then renames it to `<step>`. A step directory
is therefore whole or absent, and never modified afterwards.

A rerun validates each existing step directory (manifest present, output hashes
match, recorded inputs and recipe equal to the current ones) and skips it. A
directory that fails validation, or whose inputs or recipe changed, stops the
run with the step named; the operator removes that directory and every later
one, or starts a new `<rel>`. A failed step, including disk full, leaves only
its temporary directory, removed on the next run, so the rerun starts at that
step.

Temporary files of every process go under `FRAMEWORKS_WORK_DIR` (`TMPDIR` is set
there). The command disables core dumps for itself and its children.

## Gaps and signing

There is no gap acceptance. Steps record gaps; `build` writes them as
`quality_gap` rows. Every kind is in one table, with its class
([results](frameworks-results.md#diagnostics-and-counts)) and whether it blocks
signing; a kind not in the table fails `build`. Changing the table is a recipe
change.

| Kind                                                         | Class    | Blocks                                     |
| ------------------------------------------------------------ | -------- | ------------------------------------------ |
| `mapping_unresolved`                                         | `public` | no                                         |
| `mapping_dropped`                                            | `public` | no                                         |
| `title_missing`                                              | `public` | yes for a `licensed` version, no otherwise |
| `ocr_blocked`                                                | `text`   | yes                                        |
| `ocr_flagged`                                                | `text`   | no                                         |
| `text_incomplete`                                            | `text`   | no                                         |
| `chunk_omission`                                             | `text`   | no                                         |
| `embedding_input_too_long`, `embedding_input_bytes_exceeded` | `text`   | no                                         |

`report.json` names the release id, the dump SHA-256, the build manifest
SHA-256, the recipe hashes of `snapshot` through `embed`, of `build` and of
`evaluate` and, from the second release on, the previous signed report
(`previous: {rel, report_sha256}`) with a `previous_diff` of every gating
comparison. It carries `quality_pass`, true only when: the evaluated copy came
from that dump; no blocking gap exists; the retrieval recipe status is
`measured`; the invariant suite passed; the golden-case floors, once set, are
met; and from the second release every gating comparison against the previous
report passed. The uploader and signing workflow refuse a frameworks release
unless `quality_pass` is true, the dump, build manifest and report hashes match
the files, the report's release id and its `snapshot` through `embed` recipes
equal the release row's, its `build` and `evaluate` recipes equal those in the
build and evaluation manifests, and its previous report is the active frameworks
release's report. They read no workshop state.
