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
verified artifacts that hold its stored source payloads, and every run treats
the key as an ordinary active member read from those payloads.

## The pin

`sources/<cc>.yaml` gains `vbpl.pinned`, frozen in the run snapshot like every
source input. Each entry has:

- `key`: the VBPL document key, for example `vn:vbpl:179989`;
- `number`: its document number;
- `expand` and `fetch`: the SHA-256 of a done expand artifact and a done fetch
  artifact of that key;
- `rel`: the release of the run whose done items produced both;
- `since`: the date the source was first seen without the document.

The primary adds a pin after a run lists a named document as newly withdrawn
(below), choosing that document's latest verified acquisition; the pin is
reviewed like any source-list change. Removing the pin ends it.

## Verification

Before discover lists anything, the run verifies the whole pin set without
network requests:

- keys are unique, and numbers are unique after the shared
  [number normalization](data.md#official-number-normalization-proposed-extension);
- for each pin, both artifacts exist and their manifests and file hashes verify;
  each was produced by a done item of the declared `rel`'s run, of this country,
  for this key, at the expand and fetch stage respectively (the recipes may
  differ from this run's);
- the normalized `number` equals the normalized number in the pinned fetch
  artifact's stored detail.

Any failure records the durable run failure `pin_invalid` and stops the run
before expand. A pin is never skipped silently.

## In a run

Discover gives each pinned key a current direct support tuple of kind `pinned`,
value the key, with the source-list hash and both artifact hashes as its
evidence reference. `pinned` seeds a primary VBPL member like `named`
([membership](membership.md#direct-discovery)), so membership never withdraws
the key while the pin stands, and no state is carried to a later run.

Expand writes the key's observation from the stored `detail.json` and
`diagram.json` of the pinned expand artifact, under the run's frozen expand
recipe. Fetch publishes the stored `detail.json`, `diagram.json`, `tree.json`,
`files.json` and `files/` of the pinned fetch artifact as the key's bundle, with
acquisition origin `pinned` and the pin's `rel` as provenance. For the pinned
key neither stage makes a source request, selects a baseline, probes ETags or
takes a control sample. Stored detail lacks `viewCount`, as every stored detail
does. Later stages, the key's one-hop context, the secondary hand-over and
validity then follow the ordinary rules; context documents the source still
serves are read fresh.

Discover also records two outcomes from the listings it makes anyway:

- `source_pin_live`: a complete listing shows the pinned key again. This is a
  notice, not a gap: it does not gate the release, the run continues on the
  pinned payloads, and the primary removes the pin before the next run.
- `pin_conflict`: another VBPL key admitted on current listing evidence has the
  normalized number of a pin, so the source re-published the document. The run
  records the durable run failure `pin_conflict` before expand, because the
  corpus must not hold two records of one document; the primary removes or
  replaces the pin and starts a new run. A chinhphu candidate never raises it:
  it meets the pinned member through the
  [hand-over table](sources.md#secondary-discovery-and-hand-over).

## What is recorded and shown

The pinned document carries the coverage gap `source_withdrawn_retained` with
`since`, `rel` and both artifact hashes; release acceptance lists it, and
acceptance follows the ordinary gap rules. Build stores `since` and `rel` on the
document, and every MCP document entry of a pinned document, in search and in
read, carries `source_withdrawn` with `since` and `acquired_by_rel`; other
documents omit it. Without it an agent would read the status stamp as fresh.
Index loads the document like any active member; its text comes from the pinned
payloads, the one exception to [index](index.md#inclusion-and-losses)'s rule
against carrying text from an earlier run.

No artifact pruning exists today; any pruning added later must keep the
artifacts every pin names.

## Newly withdrawn named documents

`corpus status` and the run report list first every member with a `named`
support tuple that became `withdrawn` in the run, with its number and latest
verified expand and fetch artifacts, then every `source_pin_live` notice. Each
withdrawn one needs a pin decision before the release is accepted.

## The secondary source

[chinhphu](sources.md#secondary-discovery-and-hand-over) fills central documents
VBPL lacks, and VBPL wins on a proven identity match. A pinned key is an
ordinary active VBPL member, so the hand-over table applies unchanged: a proven
match becomes `source_replaced`, a different verified date is a different
document, and missing or conflicting evidence is `source_identity_ambiguous`.
The pin is not replaced by chinhphu: that would drop the VBPL tree and diagram
relations and change node ids and citations while the verified bytes are
unchanged. The primary may remove the pin and let chinhphu admit the document if
VBPL never returns.

## Data

- Source list: `vbpl.pinned` as above; the discover recipe changes once for the
  new support kind.
- `run_membership_evidence` gains the kind `pinned`, and `run_failure` the codes
  `pin_invalid` and `pin_conflict` ([data](data.md#scope-workshop-contracts)).
- Coverage gap kinds gain `source_withdrawn_retained`; `source_pin_live` is kept
  in the discover report and shown by status, never in `gap`.
- The release `document` row gains nullable `source_withdrawn_since` and
  `source_withdrawn_rel`; the MCP schema gains the optional `source_withdrawn`
  object on document entries.

## Checks

The primary runs these; they are requirements, not results.

- A pinned key absent from listings is active with no source request, its bundle
  equal to the pinned files, the gap, and `source_withdrawn` in search and read.
- A missing or altered artifact, a wrong producing run, stage or key, a number
  mismatch, or a duplicate key or normalized number fails the run with
  `pin_invalid` before expand.
- A pinned key listed live gives a non-gating `source_pin_live` notice; another
  VBPL key with the pin's normalized number fails the run with `pin_conflict`; a
  chinhphu candidate follows the hand-over table.
- Context targets of a pinned key keep support and are read fresh; the key is
  never baseline-selected, ETag-probed or control-sampled.
- Validity rules and corrections apply to the pinned document as to any member.
- A named member that becomes `withdrawn` is listed first in status and report.
- The first pin, `vn:vbpl:179989` at its run-33 (vn/20261113_1) artifacts,
  restores the law and its four context documents in the next run.
