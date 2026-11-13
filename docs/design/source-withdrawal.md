# Source withdrawal retention

Proposed. A source can delete the record of a document that is still law. On
2026-11-13 VBPL stopped listing `vn:vbpl:179989` (Law 71/2025/QH15, named in
`sources/vn.yaml`) and answers its detail with HTTP 400
`invalid.document.entity.not.found`. Under
[membership](membership.md#direct-discovery) the document became `withdrawn` and
left the release, and its four relation-context documents became `scope_removed`
because their only support was its relations. This note keeps such a document in
the corpus from its last verified acquisition and marks it.

## Which documents

A VBPL member qualifies when, in this run, it would otherwise become `withdrawn`
or lose its fetch to a not-found answer, and all of these hold:

1. It has a `named` tuple from the frozen source list, current or from the most
   recent earlier run that held it; or it is relation context supported only by
   relations of a qualifying named member.
2. An earlier run holds a verified acquisition of it: a done fetch artifact and
   a done expand artifact whose manifests and file hashes verify in the artifact
   store, with the recorded `fetch_sha256`.
3. No active VBPL member in this run has the same identity (normalized number,
   issuer and issue date, [identity comparison](data.md#identity-comparison)).
   When VBPL re-publishes the document under a new id, the new id wins and the
   old one is withdrawn as today, so the corpus never holds two records of one
   document.

A not-found answer is the VBPL response with HTTP 400 and `messageCode`
`invalid.document.entity.not.found` on detail, diagram or a listed attachment,
or an absence that the [membership rules](membership.md#direct-discovery) would
record as `withdrawn`. Any other failure stays a retryable source gap under the
[source-error table](fetch-attachments.md#download-deadline-and-failures). A
member that meets none of the conditions keeps today's outcome. Fail closed:
without a verified earlier acquisition there is nothing to retain, and the
document is withdrawn.

## What is kept

The member stays `active`. Discover records its named tuple as `retained` with
the reason `source_withdrawn`. Expand adopts the detail and diagram observations
of the verified acquisition; fetch adopts its bundle, the way done items adopt
acquisitions on resume. Extract, relate, chunk and embed then reuse their
artifacts byte for byte. The adopted diagram keeps supporting its
relation-context documents, which VBPL still serves and fetch reads fresh; a
context document that also answers not-found qualifies under rule 1.

The document carries the coverage gap `source_withdrawn_retained` with detail:
the date the source was first seen withdrawing it (carried from run to run), the
run and `fetch_sha256` of the retained acquisition, and the not-found evidence
of this run. Its text, status and validity are those of the retained
acquisition; the gap tells an agent that the source no longer serves the record
and since when. [Index](index.md#inclusion-and-losses) loads it like any active
member; this is the one exception to "do not carry stale text from a previous
run".

## Each run

Every run repeats the named lookup and the detail request. When the source lists
and serves the document again, the retention ends: fetch reads it fresh and the
gap disappears. While it lasts, release acceptance lists every
`source_withdrawn_retained` gap with its first-seen date. Acceptance follows the
[removal lifetime](membership.md#removal-lifetime) rule: the owner accepts each
withdrawal event (document key and first-seen date) once, and later runs carry
that acceptance until the event ends. A new withdrawal after the source served
the document again is a new event.

## The secondary source

[chinhphu](sources.md#secondary-discovery-and-hand-over) fills central documents
VBPL lacks, and VBPL wins when both hold one identity. A retained VBPL member
keeps that precedence: a chinhphu candidate with the same identity becomes
`source_replaced` as today, and its key is recorded in the gap detail as a live
source of the same document. It does not take over, because the switch would
drop the VBPL tree and diagram relations, change node ids and citations of a
document whose verified bytes are unchanged, and make the corpus follow a source
outage. The owner can still remove the named entry; the member is then withdrawn
under the current rules.

## Checks

The primary runs these; they are requirements, not results.

- A named member absent from listings with a not-found detail and a verified
  earlier acquisition stays active with byte-identical downstream artifacts and
  the gap; without a verified acquisition it is withdrawn.
- Its relation-context documents keep their support; one that also answers
  not-found is retained, one that is still served is read fresh.
- A new VBPL id with the same identity withdraws the old id; no retention.
- A not-named member and any other error code keep today's outcome.
- The first-seen date carries across runs; the gap ends when the source serves
  the document again; acceptance carries per event.
- A chinhphu candidate with the same identity stays `source_replaced`, named in
  the gap detail.
- The next run reports every retained document and its first-seen date.
