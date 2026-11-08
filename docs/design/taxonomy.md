# Subject taxonomy

Two axes: a subject tree for narrowing, and the structure tree of each document
for citing. This note owns the subject tree and its classification.
Decision: [ADR 0004](../adr/0004-subject-taxonomy-from-phap-dien.md). Facts
about the source: [research note](../research/phap-dien-2026-11-08.md).

## Import mapping from the 2023 codification package

| Package element                   | Level | Code form            | Rule                                                                 |
| --------------------------------- | ----- | -------------------- | -------------------------------------------------------------------- |
| chủ đề (`jdChuDe`, 45)            | 1     | `22`                 | STT as code                                                          |
| đề mục (`jdDeMuc`)                | 2     | `22.3`               | chủ đề STT plus đề mục STT; entries marked "loại bỏ" import as `retired` |
| phần or chương inside a đề mục    | 3     | `22.3.c2`            | from `jdAllTree` MAPC order; phần without chương becomes level 3     |
| mục inside a chương               | 4     | `22.3.c2.m1`         | optional; a chương without mục is a leaf                             |
| custom node                       | 3 or 4 | `22.3.x1`           | declared in `taxonomy/custom.yaml` with name and one-sentence description |

Codes are stable identifiers, not display paths. A node that disappears from a
later import becomes `retired` and keeps its code. A node that moves keeps its
code and changes `parent_id`. `taxonomy_version` names the mapping table and
custom file revision used by a release.

## Classification layers

Precedence: `manual` over `phapdien_seed` over `llm`.

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
   - Output is JSON with codes and confidence. Below the recorded threshold, no
     assignment and a gap "unassigned".
3. Manual overrides in `taxonomy/overrides.yaml`.

## Evaluation gate

Before a release may publish assignments, `corpus eval` measures on a held-out
set that is sealed before prompt tuning:

| Slice                                                   | Metric                                  | Floor |
| ------------------------------------------------------- | --------------------------------------- | ----- |
| 300 seeded articles, labels masked                      | level-2 agreement                       | 0.90  |
| same                                                    | level-4 agreement where a level 4 exists | 0.75  |
| same                                                    | candidate recall at 10                  | 0.95  |
| 100 post-2023 articles labeled by the owner             | level-2 precision and recall            | 0.85  |
| 50 articles that belong to custom nodes                 | recall of the custom node               | 0.80  |

The report also lists assignment counts per node, unassigned counts, and the
diff of node changes against the previous release. A failed floor blocks
`release`.
