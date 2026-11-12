# Subject taxonomy

Two axes: a subject tree for narrowing, and the structure tree of each document
for citing. This note owns the subject tree and its classification. Decision:
[ADR 0004](../adr/0004-subject-taxonomy-from-phap-dien.md). Facts about the
source: [research note](../research/phap-dien-2026-11-08.md).

## Import mapping from the 2023 codification package

| Package element                                  | Level                                            | Code form               | Rule                                                                                                                                                                                                             |
| ------------------------------------------------ | ------------------------------------------------ | ----------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| chủ đề (`jdChuDe`, 45)                           | 1                                                | `22`                    | STT as code                                                                                                                                                                                                      |
| đề mục (`jdDeMuc`)                               | 2                                                | `22.3`                  | chủ đề STT plus đề mục STT; when that code is taken, the next free number; entries marked "loại bỏ" import as `retired`; a đề mục with no placed article is imported but not served until it holds an assignment |
| phần, when the đề mục has phần                   | 3                                                | `22.3.p1`               | MAPC order                                                                                                                                                                                                       |
| chương                                           | 3, or 4 under a phần                             | `22.3.c2`, `22.3.p1.c2` | MAPC order                                                                                                                                                                                                       |
| mục inside a chương                              | 4, or dropped when the chương is already level 4 | `22.3.c2.m1`            | a dropped mục is recorded in the mapping file as flattened into its chương                                                                                                                                       |
| tiểu mục inside a mục                            | never a node                                     | none                    | flattened into its mục; recorded in the mapping file                                                                                                                                                             |
| mục or tiểu mục without a chương or mục above it | never a node                                     | none                    | folded into its nearest coded ancestor (the đề mục or a phần); recorded in the mapping file                                                                                                                      |
| custom node                                      | 3 or 4                                           | `22.3.x1`               | declared in `taxonomy/custom.yaml` with name and one-sentence description, under an active package node or another custom node                                                                                   |

Codes are stable identifiers, not display paths. `taxonomy/mapping.yaml` maps
each package node GUID (`ID` in `jdAllTree`, `Value` in `jdChuDe` and `jdDeMuc`)
to its code. A later import looks up the GUID first and keeps the code even when
the position changed; a GUID not in the file receives a new code, and a code is
never reused. A new code is its parent's code, the kind letter and one more than
the highest number of any code ever recorded in the mapping with exactly that
prefix, whatever the status or current position of that code's node; a chủ đề or
đề mục number follows the STT rule in the table, falling back to the same scan.
A GUID missing from a later import marks its node `retired`. A GUID that keeps
an active node changes only its level, parent and ordinal, never its code. A
GUID whose node becomes flattened retires its code and is recorded as flattened.
A retired code never returns to service: a GUID whose node is active again after
its code was retired, because it went missing, was flattened or was marked
removed, receives a new code. A retired entry is history: the import that
retires it records it a last time and later imports never change it, so a GUID
holds at most one active code and any number of retired ones. A đề mục marked
removed that holds no active code is recorded once as retired; the nodes inside
it are neither coded nor flattened, so codes they held retire.
`taxonomy_version` is the hash of the mapping file, the custom file and the
overrides file.

## Inputs

The package is an input artifact, never committed: `BoPhapDienDienTu.zip`,
SHA-256 `dc2fc7d14e208b551e3e7667e911dcfd24d8f246a7e24d3d24ad7507cd4b5ff5`, from
`https://raw.githubusercontent.com/definev/phapdien/ef75c3edb63fa52d979ad55cec7cbe37d6944ced/data/BoPhapDienDienTu.zip`
(33,893,115 bytes), fetched into `work/phapdien/` by a script that checks the
hash; its HTML and JavaScript are read as data, never run. Tests use a small
synthetic package with the same file shapes and freshly written text. Three
files are committed under `taxonomy/`:

- `mapping.yaml`: the package SHA-256, then one entry per package node with its
  GUID, code, level, kind (`chude`, `demuc`, `phan`, `chuong`, `muc`), parent
  code, name as last imported, status and, when active, its package ordinal
  among its active siblings, and one entry per flattened node (a dropped mục or
  a tiểu mục) naming the code it folds into. `corpus taxonomy import` rewrites
  it from the package under the code rules above, deterministically, and never
  reuses a code; a person reviews the diff and commits it.
- `custom.yaml`: custom nodes, each with code, parent code, name and a
  one-sentence description; empty until a subject needs one.
- `overrides.yaml`: manual entries, each with document key, provision anchor,
  node code, `exclude` and a reason; empty until reviewed.

`taxonomy_version` is the SHA-256 of the three files' bytes, each prefixed with
its length, in the order mapping, custom, overrides; it enters the build input
hash. A build refuses a package whose hash differs from the one in
`mapping.yaml`. Package nodes take their names from the package as recorded in
`mapping.yaml`, so a retired node keeps its last name, and have an empty
description; custom nodes carry their own. Package article text and notes are
read only to place articles and are never stored or served (ADR 0004).

## Classification layers

Precedence and replacement within a node family are defined in [data](data.md)
under the corpus rules.

1. Seed. An article whose VBPL ItemID and article anchor match a placed article
   in the package receives that node down to its deepest imported level,
   `confidence = 1.0`. The ItemID comes from the first link in the article's
   note that points to `vbpq-toanvan.aspx` with an `ItemID`, skipping other
   links; it matches the release document whose key is `vn:vbpl:<ItemID>`, of
   any class. The article number is the one after `Dieu_` in that link, else the
   first `(Điều <n>` in the note, and matches the provision anchored `dieu-<n>`
   in that document. An article listed twice yields one assignment. A document
   that is not in the release is expected and yields nothing; a document found
   without the article yields a coverage gap `seed_article_missing` naming the
   document and the node. Number-only and consolidation matches are not seeds.
2. LLM in batches over public legal text, after `chunk` and `embed` have run:
   - One request per document (title, scope article, table of contents, VBPL
     field) yields level-2 nodes.
   - One request per chapter with all its articles yields assignments for each
     article. Candidate nodes are the top 10 by cosine between the article's
     chunk vectors and node description vectors embedded with the same dense
     recipe, searched across the whole tree.
   - Output is JSON with a set of codes and a confidence per code; an article
     may receive several nodes in different families. The threshold per level is
     calibrated on the labeled set to reach precision at least 0.85, then frozen
     in the release; below it, no assignment and a gap "unassigned".
3. Manual overrides in `taxonomy/overrides.yaml`. An override names an active
   package node at level 2 or deeper or a custom node; any other code fails
   `corpus taxonomy check` and the build. An override whose document or anchor
   is not in the release yields a coverage gap `override_target_missing` naming
   the document and the node, and nothing else.

## Serving

A node is served when it is `active` and its subtree holds at least one
effective assignment; any other code is unknown to every read path and filter.
Assignments sit on the provision they name (seeds on articles). A provision's
effective nodes are resolved per family along its ancestry: in each family, the
nearest provision, itself first and then each ancestor, that holds any entry in
that family decides, applying the [data](data.md) precedence at that provision;
an `exclude` there removes the family. So a clause inherits its article's nodes
unless the clause itself carries an entry in that family, and a manual entry or
exclusion on a clause wins over a seed on its article. Effective nodes drive
hits, filters and counts alike.

Each node carries its sibling `ordinal`: active package siblings in package
order, then custom siblings in file order, then retired siblings in code order;
and the counts of distinct provisions with an effective node in its subtree and
of their documents. The build computes both. It places a retired node under its
recorded parent at the parent's level plus one and leaves out a retired node
that would fall below level 4. Every mapping and custom node reaches a chủ đề
through its parents in at most four steps; any other chain, including a cycle,
fails `corpus taxonomy check` and the build. A release whose assignments come
only from seeds and overrides serves them without the classification floors
below, which gate the LLM layer; it must still pass the filter gate in
[evaluation](evaluation.md). The LLM layer's candidate text for a node is,
versioned with the classification recipe, the names of its ancestors and itself
joined from the root, followed by its description when it has one.

## Evaluation gate

Before a release may publish assignments, `corpus eval` measures on a labeled
set of seeded articles:

Labels are sets, so every metric is micro precision and recall over (article,
node) pairs. The same set tunes prompts and thresholds and gates the release;
every prompt or threshold change reruns it.

| Slice                                            | Metric                                              | Floor |
| ------------------------------------------------ | --------------------------------------------------- | ----- |
| 300 seeded articles, labels masked               | precision and recall at level 2                     | 0.90  |
| same                                             | precision and recall at level 3                     | 0.80  |
| same                                             | precision and recall at the deepest level           | 0.75  |
| 100 articles with labels in two or more families | precision and recall over all pairs                 | 0.80  |
| every slice                                      | candidate recall at 10 (true node among candidates) | 0.95  |
| 100 post-2023 articles labeled by the owner      | precision and recall at level 2                     | 0.85  |
| same                                             | precision and recall at the deepest level           | 0.70  |
| 50 articles that belong to custom nodes          | precision and recall of custom nodes                | 0.80  |

Every level that a release serves must have a floor in this table; a new level
or slice is added here before it is served.

The report also lists assignment counts per node, unassigned counts, and the
diff of node changes against the previous release. A failed floor blocks
`release`.
