# Pinned documents

Proposed. A source can delete the record of a document that is still law. On
2026-11-13 VBPL stopped listing `vn:vbpl:179989` (Law 71/2025/QH15, named in
`sources/vn.yaml`) and answers its detail with HTTP 400
`invalid.document.entity.not.found`. Under
[membership](membership.md#direct-discovery) it became `withdrawn` and left the
release, and its four relation-context documents became `scope_removed` because
their only support was its relations.

Automatic retention would need state carried between runs: proof of the last
listing, acquisition references, event continuity and recovery. This note uses
an explicit, reviewed pin instead: the source list names the document and the
verified artifacts that hold its source payloads, and every run treats the key
as an ordinary active member read from those payloads.

## The pin

`sources/<cc>.yaml` gains `vbpl.pinned`, frozen in the run snapshot like every
source input. Each entry has:

- `key`: the VBPL document key, for example `vn:vbpl:179989`;
- `number`: its document number;
- `expand` and `fetch`: the SHA-256 of a verified expand artifact and a verified
  fetch artifact of that key from an earlier run;
- `rel`: the release of the run that produced them;
- `since`: the date the source was first seen without the document.

The primary adds a pin after a run reports a named document as newly withdrawn
(below), choosing that document's latest verified acquisition, and the pin is
reviewed like any source-list change. Removing the pin ends it.

## In a run

Before discover lists anything, the run verifies every pin: both artifacts exist
in the artifact store, their manifests and file hashes verify, their recorded
stage and item key match the pin, and the fetch artifact's document number
matches `number`. Any failure records the durable run failure `pin_invalid` and
stops the run before expand. A pin is never skipped silently.

Discover admits each pinned key with a current support tuple of the new rule
kind `pinned` (value: the key). The key is then an ordinary active VBPL member:
membership never withdraws it while the pin stands, and no state is carried to a
later run. For a pinned key the VBPL client answers detail, diagram, tree and
attachment requests from the raw payloads in the pinned artifacts and makes no
network request; every other key is unaffected. Expand and fetch process those
payloads under the run's frozen recipes, so relations, membership of its one-hop
context, the secondary hand-over and validity follow the ordinary rules. Context
documents the source still serves are read fresh.

Two observations are reported every run, from listings discover makes anyway:

- `source_pin_live`: a complete listing shows the pinned key again. The run
  continues on the pinned payloads; the primary removes the pin before the next
  run.
- `pin_conflict`: a member admitted on current listing evidence has the same
  normalized number as a pin under another key, so the source re-published the
  document. The run records the durable run failure `pin_conflict` before
  expand, because the corpus must not hold two records of one document; the
  primary removes or replaces the pin and starts a new run.

## What is recorded and shown

The pinned document carries the coverage gap `source_withdrawn_retained` with
`since`, `rel` and both artifact hashes. Release acceptance lists every such gap
and every `source_pin_live` notice. Build stores `since` and `rel` on the
document, and every MCP document entry of a pinned document, in search and in
read, carries `source_withdrawn` with `since` and `acquired_by_rel`; other
documents omit it. Without it an agent would read the status stamp as fresh.
Index loads the document like any active member; text comes from the pinned
payloads, which is the one exception to [index](index.md#inclusion-and-losses)'s
rule against carrying text from an earlier run.

No artifact pruning exists today; any pruning added later must keep the
artifacts every pin names.

## Newly withdrawn named documents

Each run lists, first in its status and in its report, every member with a
`named` support tuple that became `withdrawn` in that run, with its number and
latest verified expand and fetch artifacts. Each needs a pin decision before the
release is accepted.

## The secondary source

[chinhphu](sources.md#secondary-discovery-and-hand-over) fills central documents
VBPL lacks, and VBPL wins on a proven identity match. A pinned key is an
ordinary active VBPL member, so a chinhphu candidate with the same identity
becomes `source_replaced` as today. The pin is not replaced by chinhphu: that
would drop the VBPL tree and diagram relations and change node ids and citations
while the verified bytes are unchanged. The primary may still remove the pin and
let chinhphu admit the document if VBPL never returns.

## Data

- Source list: `vbpl.pinned` as above; the discover recipe changes once for the
  new rule kind.
- `run_membership_evidence` rule kinds gain `pinned`
  ([data](data.md#scope-workshop-contracts)).
- `run_failure` codes gain `pin_invalid` and `pin_conflict`.
- Coverage gap kinds gain `source_withdrawn_retained` and `source_pin_live`.
- The release `document` row gains nullable `source_withdrawn_since` and
  `source_withdrawn_rel`; the MCP schema gains the optional `source_withdrawn`
  object on document entries.

## Checks

The primary runs these; they are requirements, not results.

- A pinned key absent from listings is active, with no network request for it,
  byte-identical payloads read from its artifacts, the gap, and
  `source_withdrawn` in search and read.
- A missing or tampered pinned artifact, a wrong key or a wrong number fails the
  run with `pin_invalid` before expand.
- A pinned key listed live is reported as `source_pin_live`; another key with
  the same normalized number fails the run with `pin_conflict`.
- Context targets of a pinned key keep support and are read fresh; a chinhphu
  candidate with the same identity stays `source_replaced`.
- Validity rules and corrections apply to the pinned document as to any member.
- A named member that becomes `withdrawn` is listed first in status and report.
- The first pin, `vn:vbpl:179989` at its verified acquisition, restores the law
  and its four context documents in the next run.
