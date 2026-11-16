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
2. Consolidation inheritance. Article numbers alone do not prove that two
   articles correspond ([corpus schema](corpus-schema.md)), so an unseeded
   article of a consolidated document inherits only from a corresponding
   article: a seeded article with the same `dieu-<n>` anchor in a document it
   `consolidates` whose non-empty `heading` equals its own after Unicode NFC,
   case folding and whitespace collapse. It receives that article's seeds as
   method `inherited` at `confidence = 0.9`. Several corresponding sources give
   the union. Any other case (no such article, an empty or different heading)
   inherits nothing, goes to the LLM layer and is counted. No LLM is involved.
3. LLM in batches over public legal text, after `chunk` and `embed` have run,
   for articles that hold no seed or inherited node:
   - One request per document (title, scope article, table of contents, VBPL
     field) returns a set of at most `F` active level-2 codes, `F` set in the
     classification recipe and enforced by the output schema; a code that is not
     an active level-2 node is dropped and counted. The result is not an
     assignment: it is given as document context to every article request of
     that document and fixes those articles' allowed codes. A document whose
     request fails or keeps no code yields no LLM assignment for its articles
     and gap `unassigned` with detail `invalid_response` or `no_family`. The
     classification artifact keeps each document's validated codes and outcome,
     hashed with the classification recipe and the taxonomy; evaluation reads
     family recall from it and counts an article of a failed or empty document
     as a miss.
   - One request per chapter with all its articles yields assignments for each
     article; a document without chapters is split into groups of up to 12
     articles in document order. The classification recipe sets a maximum input
     and output size per request; a chapter or group over it is split into runs
     of consecutive articles that fit, and an article that alone exceeds it gets
     no LLM assignment and gap `unassigned` with detail `too_long`. An article's
     allowed codes are the active nodes at levels 2 to 4 in the subtrees of its
     document's level-2 codes; each article request carries those subtrees once,
     as code, name and parent code, and the request size bound counts them. A
     level-2 subtree holds at most 91 nodes and 8,708 characters of names on
     `vn/20261115_3`.
   - Article request output is JSON keyed by the request's article anchors, with
     a set of codes and a confidence per code for each; an article may receive
     several nodes in different families. A code outside that article's allowed
     codes or not an active package or custom node, and an anchor not in the
     request, is dropped and counted. A response that is truncated, fails the
     schema or misses an article yields no assignment for those articles and gap
     `unassigned` with detail `invalid_response`. The model, effort and prompt
     are part of the classification recipe. The threshold per level is
     calibrated on the labeled set to reach precision at least 0.85, then frozen
     in the release; below it, no assignment and a gap "unassigned".
4. Manual overrides in `taxonomy/overrides.yaml`. An override names an active
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
fails `corpus taxonomy check` and the build. Each layer is gated by its own rows
below: a release serving `inherited` assignments must pass the inheritance rows,
and one serving `llm` assignments the LLM rows. A release whose assignments come
only from seeds and overrides serves them without the classification floors;
every release that serves nodes must pass the filter gate in
[evaluation](evaluation.md).

## Evaluation gate

Before a release may publish assignments, `corpus eval` measures on a labeled
set of seeded articles:

Labels are sets, so every metric is micro precision and recall over (article,
node) pairs. The same set tunes prompts and thresholds and gates the release;
every prompt or threshold change reruns it.

| Slice                                                  | Reference         | Metric                                                                               | Floor |
| ------------------------------------------------------ | ----------------- | ------------------------------------------------------------------------------------ | ----- |
| 300 seeded articles, labels masked                     | seeded, masked    | precision and recall at level 2                                                      | 0.90  |
| same                                                   | seeded, masked    | precision and recall at level 3                                                      | 0.80  |
| same                                                   | seeded, masked    | precision and recall at the deepest level                                            | 0.75  |
| 100 unseeded articles labeled with every family        | model-adjudicated | precision and recall over all pairs                                                  | 0.80  |
| every LLM slice                                        | as the slice      | family recall (each true node's level-2 ancestor among the document request's codes) | 0.95  |
| 100 post-2023 articles                                 | model-adjudicated | precision and recall at level 2                                                      | 0.85  |
| same                                                   | model-adjudicated | precision and recall at the deepest level                                            | 0.70  |
| 50 articles that belong to custom nodes                | model-adjudicated | precision and recall of custom nodes                                                 | 0.80  |
| 100 inherited consolidated articles                    | model-adjudicated | precision at level 2 (inheritance layer)                                             | 0.95  |
| same                                                   | model-adjudicated | precision at level 3 (inheritance layer)                                             | 0.90  |
| same                                                   | model-adjudicated | precision at the deepest level (inheritance layer)                                   | 0.85  |
| the seeded articles with seeds in two or more families | seeded            | precision and recall over all pairs, reported                                        | none  |

Every level that a release serves must have a floor in this table; a new level
or slice is added here before it is served.

A model-adjudicated reference is not owner-labeled: Codex `gpt-6-astra` labels
the articles independently, blind to the classifier's output and candidates, and
the coordinator reads 20 of the post-2023 labels and 10 of the multi-family
labels before either slice gates. Seeds almost never span two families, because
the package places each article once (17 of 13,027 seeded articles on
`vn/20261115_2`), so the multi-family slice is model-adjudicated: 100 unseeded
articles from documents the classifier will see, half drawn from documents whose
document-level request returns two or more level-2 codes and half at random,
each labeled with every family that applies. The 17 seeded multi-family articles
are reported without a floor. The coordinator's corrections are applied, then
the set is frozen with a hash like the seeded set; the slice gates only against
that frozen set. The custom-node slice does not apply while `custom.yaml` is
empty. Seeds land on the original documents the package links, almost never on
consolidated texts, so no consolidated article has an independent label to
compare with (run `vn/20261115_2`: 0 testable articles). The inheritance slice
is therefore model-adjudicated like the post-2023 one: Codex `gpt-6-astra`
labels 100 inherited consolidated articles blind to the inherited nodes, the
coordinator reads 20, and the set is frozen with a hash; with fewer than 100
inherited articles the slice is unavailable. Build measures this slice against
that frozen set, before it writes assignments; until the set exists the slice is
unavailable: when the slice is unavailable or any inheritance row fails, build
writes no `inherited` assignment and records why in the taxonomy stats, so the
release serves seeds and overrides only; `corpus eval` reports the same rows.

The seeded labeled set is
`apps/corpus/internal/evaluation/testdata/<cc>/taxonomy-labels.json`: for each
article its document key, anchor, the SHA-256 of its own text and the seeded
node codes, with the release it was drawn from. It is drawn once by a fixed seed
from the primary articles that hold seeds, at most two articles per document,
and frozen; its SHA-256 enters the evaluation input like the identity map. It
holds the 300 masked articles. Each model-adjudicated set (multi-family,
post-2023, inheritance, custom nodes) is its own file beside it,
`taxonomy-<slice>.json`, with the same fields, the labeling model and the draw
rule, frozen with its own SHA-256 after the coordinator's reading; the
custom-node set is labeled for the custom nodes only, so it cannot exist before
`custom.yaml` has entries. The inheritance set's SHA-256, or the empty string
while it does not exist, enters the build input hash as well, because build
reads it; evaluation uses the same file. Every entry is checked against the
release it scores: the document and anchor are present, the provision's own text
has the recorded SHA-256, and every code is an active node. An entry that fails
counts as a miss in its slice, so the denominator never shrinks; when more than
5 percent of a set's entries fail, the slice is unavailable until the set is
re-adjudicated and frozen again. A redraw is a reviewed change, never automatic.

The report also lists assignment counts per node, unassigned counts, and the
diff of node changes against the previous release. A failed floor blocks
`release`.
