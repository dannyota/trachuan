# Subject taxonomy

Two axes: a subject tree for narrowing, and the structure tree of each document
for citing. This note owns the subject tree and its classification. Decision:
[ADR 0004](../adr/0004-subject-taxonomy-from-phap-dien.md). Facts about the
source: [research note](../research/phap-dien-2026-11-08.md).

## Import mapping from the 2023 codification package

| Package element                | Level                                            | Code form               | Rule                                                                                                                                                                                                             |
| ------------------------------ | ------------------------------------------------ | ----------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| chủ đề (`jdChuDe`, 45)         | 1                                                | `22`                    | STT as code                                                                                                                                                                                                      |
| đề mục (`jdDeMuc`)             | 2                                                | `22.3`                  | chủ đề STT plus đề mục STT; when that code is taken, the next free number; entries marked "loại bỏ" import as `retired`; a đề mục with no placed article is imported but not served until it holds an assignment |
| phần, when the đề mục has phần | 3                                                | `22.3.p1`               | MAPC order                                                                                                                                                                                                       |
| chương                         | 3, or 4 under a phần                             | `22.3.c2`, `22.3.p1.c2` | MAPC order                                                                                                                                                                                                       |
| mục inside a chương            | 4, or dropped when the chương is already level 4 | `22.3.c2.m1`            | a dropped mục is recorded in the mapping file as flattened into its chương                                                                                                                                       |
| tiểu mục inside a mục          | never a node                                     | none                    | flattened into its mục; recorded in the mapping file                                                                                                                                                             |
| custom node                    | 3 or 4                                           | `22.3.x1`               | declared in `taxonomy/custom.yaml` with name and one-sentence description                                                                                                                                        |

Codes are stable identifiers, not display paths. `taxonomy/mapping.yaml` maps
each package node GUID (`ID` in `jdAllTree`, `Value` in `jdChuDe` and `jdDeMuc`)
to its code. A later import looks up the GUID first and keeps the code even when
the position changed; a GUID not in the file receives the next free code at its
level, and a code is never reused. A GUID missing from a later import marks its
node `retired`. `taxonomy_version` is the hash of the mapping file, the custom
file and the overrides file.

## Classification layers

Precedence and replacement within a node family are defined in [data](data.md)
under the corpus rules.

1. Seed. An article whose VBPL ItemID and article anchor match a placed article
   in the package receives that node down to its deepest imported level,
   `confidence = 1.0`.
2. LLM in batches over public legal text, after `chunk` and `embed` have run:
   - One request per document (title, scope article, table of contents, VBPL
     field) yields level-2 nodes.
   - One request per chapter with all its articles yields assignments for each
     article. Candidate nodes are the top 10 by cosine between the article's
     chunk vectors and node description vectors embedded with the same dense
     recipe, searched across the whole tree.
   - Output is JSON with a set of codes and a confidence per code; an article
     may receive several nodes in different families. The threshold per level is
     calibrated on the development half of the held-out set to reach precision
     at least 0.85, then frozen in the release; below it, no assignment and a
     gap "unassigned".
3. Manual overrides in `taxonomy/overrides.yaml`.

## Evaluation gate

Before a release may publish assignments, `corpus eval` measures on a held-out
set that is sealed before prompt tuning:

Labels are sets, so every metric is micro precision and recall over (article,
node) pairs. The set is split in half; the development half tunes prompts and
thresholds, the sealed half is scored once per release.

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
