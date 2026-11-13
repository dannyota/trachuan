# Source withdrawal retention

Proposed. A source can delete the record of a document that is still law. On
2026-11-13 VBPL stopped listing `vn:vbpl:179989` (Law 71/2025/QH15, named in
`sources/vn.yaml`) and answers its detail with HTTP 400
`invalid.document.entity.not.found`. Under
[membership](membership.md#direct-discovery) it became `withdrawn` and left the
release, and its four relation-context documents became `scope_removed` because
their only support was its relations. This note keeps such a document from its
last verified acquisition, decided once at discover, and marks it everywhere an
agent sees it.

## Qualification

Discover decides retention where the member-level fallback of
[membership](membership.md#direct-discovery) would record `withdrawn`, before
expand freezes membership. A VBPL member is retained instead when all hold:

1. The run's frozen source list has a `named` entry whose lookup listed this key
   in the most recent earlier complete lookup. A member whose named entry left
   the source list is withdrawn as today, and an accepted removal is never
   revived.
2. The membership predecessor holds a verified acquisition of the key: its done
   expand artifact and done fetch artifact, with manifests and file hashes that
   verify in the artifact store and the recorded `fetch_sha256`. Missing or
   unverifiable artifacts mean no retention.
3. No member admitted on current evidence in this run has an equal normalized
   number, whatever its issuer or date evidence. Two retention candidates with
   an equal normalized number are both withdrawn. When the source re-publishes a
   document under a new key, the new key wins.

Only listing absence triggers retention. A key the listings still show but whose
detail, diagram or attachments fail keeps the
[source-error table](fetch-attachments.md#download-deadline-and-failures): 403,
429, retryable failures and `missing` attachments behave as today. A withdrawal
noticed after expand froze membership changes nothing in that run; the next
run's discover decides.

Relation context is not retained on its own. A retained member's adopted diagram
supports its one-hop targets like any active seed, so context documents the
source still serves are fetched fresh, whatever other support they have. A
context document whose own record is gone is withdrawn as today; retention never
chains.

## Adoption

The member stays `active`. Expand adopts the predecessor's verified expand
artifact for the key and requests nothing for it; fetch adopts the verified
fetch artifact the same way. Both references are recorded at discover and frozen
with the run. Later stages run as for any active member under the run's frozen
recipes: unchanged inputs reuse artifacts through the usual input hashes, and
changed recipes rebuild them from the adopted inputs. Artifacts that an active
retention references are kept; artifact pruning must skip them.

Build treats the adopted detail as raw source metadata and applies the current
normalization, [validity rules and corrections](validity-status.md) as for any
member, so a later replacement event or correction still applies.
[Index](index.md#inclusion-and-losses) loads the document; this is the one
exception to "do not carry stale text from a previous run".

## Event, recheck and recovery

A withdrawal event is identified by the key and the run that first retained it.
Each run's discover repeats the named lookup and, for a retained key only, one
detail request; expand and fetch request nothing for it. Recovery is a complete
named lookup that lists the key and a detail request that succeeds, both in one
run; that run fetches fresh and ends the event. A later loss starts a new event
with a new first run.

The document carries the coverage gap `source_withdrawn_retained`, detail: first
run, first-seen date, retained acquisition run and `fetch_sha256`, this run's
listing and detail evidence. Release acceptance lists every such gap. The owner
accepts an event once; acceptance carries only within the same event, like the
[removal lifetime](membership.md#removal-lifetime).

## What an agent sees

Every MCP document entry of a retained document, in search and in read, carries
`source_withdrawn` with `since` (first-seen date) and `acquired` (the retained
acquisition's fetch date); other documents omit the field. Text, `text_source`
and validity are served as built. Without the field an agent would read the
status stamp as fresh.

## The secondary source

[chinhphu](sources.md#secondary-discovery-and-hand-over) fills central documents
VBPL lacks, and VBPL wins on a proven identity match. A retained member keeps
that precedence: a chinhphu candidate with the same identity becomes
`source_replaced` as today, and its key is named in the gap detail. It does not
take over: the switch would drop the VBPL tree and diagram relations and change
node ids and citations while the verified bytes are unchanged. The owner can
remove the named entry, which ends the retention by rule 1.

## Data

A workshop table `run_source_retention` (run, document key, first run,
first-seen date, adopted expand artifact, adopted fetch artifact, listing and
detail evidence references) holds one row per retained member per run. Discover
writes it in the transaction that records membership; expand, fetch, build and
release acceptance read it. The membership support state stays `retained`, the
disposition `active`. The MCP schema gains the optional `source_withdrawn`
object on document entries.

## Checks

The primary runs these; they are requirements, not results.

- A named member absent from complete listings with a verified acquisition stays
  active: no source request for it in expand or fetch, one detail recheck, the
  gap, and `source_withdrawn` in search and read. Without a verified acquisition
  it is withdrawn.
- A member whose named entry left the source list is withdrawn; an accepted
  removal stays removed.
- A current member with an equal normalized number withdraws the old key; two
  candidates with one number are both withdrawn.
- A listed key with a failing detail, a 403, a 429 or a missing attachment keeps
  today's outcome; a withdrawal seen only at fetch changes nothing in that run.
- Context targets of a retained member keep support and fetch fresh; a context
  document whose record is gone is withdrawn.
- Recovery needs a listing and a successful detail in one run; a later loss is a
  new event that needs its own acceptance.
- A later replacement event or correction changes the retained document's
  validity in build.
- Pruning keeps artifacts an active retention references.
