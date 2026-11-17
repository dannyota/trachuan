# Control frameworks: results

This note owns the field-level contract of framework results on `/mcp` and
`/api/v1/frameworks`. The decisions are in
[ADR 0016](../adr/0016-control-frameworks-corpus.md); the fixed serving rules
are in [control frameworks](frameworks.md#serving); access, the reduced record
and the invariant are in [framework access](frameworks-access.md). Examples use
invented frameworks: `fw-alpha` version `2.1` and `fw-beta` version `1.0`.

## Authentication and access

`/mcp` takes bearer credentials as today; `/api/v1/frameworks` takes the web
session and CSRF rules of [web API](web-api.md), never a bearer token. Nothing
is anonymous. Authentication happens first and follows the existing rules: a
credential that cannot be validated, or an unavailable auth store, fails the
request as it does for law.

Only after authentication succeeds is the caller's access set computed
([decision](frameworks-access.md#decision)). If that entitlement lookup alone
fails, the request continues with the empty access set, the top level carries
`access_unavailable: true`, and nothing from it is cached.

`access_set_version` is an HMAC, under a server key, of the release id string
and the sorted list of version handles the caller can read. It depends on no
text and no content digest; two callers with the same readable versions in the
same release get the same value. Every response carries it.

The server key for `access_set_version` and for signing continuations is read
from `SERVER_FRAMEWORKS_KEY_FILE` (at least 32 random bytes), required when
`SERVER_FRAMEWORKS` is on and unused otherwise; a new key makes every earlier
continuation invalid. Law continuations stay as they are.

## Format

Results follow the law [format](mcp-results.md#format). Each response is
computed from the caller's projection: the full fields of readable versions and
the approved public fields of the others. Every read, index lookup, cache entry,
count and cursor for a request is taken from that projection only. A query for a
version the caller cannot read never selects its body, original title, notes,
tables, private evidence or text-classified gaps, even to discard them; the
filter is part of the query, not a step after it. That defines the timing class:
which structures a request reads depends only on the access set and public
fields.

The engine reads each version's index partitions once per release, when it opens
the release and for every version alike: controls with their public fields and
mapping counts, and per index the postings and statistics. They live in server
memory, keyed by version. A request then uses only the partitions of the
versions its projection allows; no request loads, waits on or warms a partition,
so neither the work nor the timing of a request depends on what another caller
can read. Body text, original titles, notes, tables, evidence and text gaps stay
in the database and are read per request under the projection filter, as above.
The partitions are bounded: the engine counts their size while loading (entries
times a fixed per-entry cost plus string bytes), and a release whose partitions
would exceed 256 MiB does not open; frameworks are then `unavailable` and law
serving is untouched. A release is opened in the background when the server
starts or the frameworks pointer changes, never inside a request: until it has
opened, frameworks requests answer `unavailable` at once, and a release whose
open failed is not retried until the pointer changes or the server restarts. The
public-field index keeps one set of statistics across versions; each version's
body index keeps its own. The measured heap is in
[host budgets](budgets-host.md).

## Handles and identity

A control handle is `fw:<code>:<version>#<citation>`, for example
`fw:fw-alpha:2.1#A.5.1`; a version handle is `fw:<code>:<version>`.

- `code` matches `[a-z][a-z0-9]*(-[a-z0-9]+)*`.
- `version` matches `[0-9a-z]+([.+-][0-9a-z]+)*`.
- A stored citation is 1 to 64 printable ASCII characters (0x21 to 0x7E plus
  inner single spaces). In a handle every byte outside `[A-Za-z0-9.()_~-]` is
  percent-encoded in upper-case hex. A handle whose re-encoding differs from
  itself is `invalid_handle`.
- `citation_norm` is the citation upper-cased with spaces removed. Within one
  version, two controls with the same `citation_norm` fail the build, so a
  citation never resolves to two controls.

Versions of one framework have a total order from the registry: `released`
(date), then the registry `ordinal`. "Newest" means last in that order. Registry
aliases are unique across all frameworks and codes (the build fails otherwise),
and an exact code match takes precedence over an alias.

A `read` call holds framework handles only or law handles only; a mix is
`invalid_request`. Handles never encode a release, a database id, a rank or an
access set.

## Inputs

`corpus_status`, `search`, `read`, `quality_gaps` and `mappings` take
`corpus: "frameworks"` instead of `country` (`mappings` has no law form).
Filters: `frameworks[]` (codes) and `versions[]` (version handles). A filter
naming a code or version not in the release is `invalid_request` for every
caller. The active frameworks release is used unless `release` pins one, as for
law; a pinned release that is gone is `release_expired`.

**Citation routing.** The query gate tries `<code or alias> <citation>`
(`FW-ALPHA A.5.1`) before free text. Resolution reads public fields only: it
looks up `citation_norm` in every version the filters allow and returns the
matching controls in version order, newest first, with `lookup: true`, no score
and no abstention. No match falls through to search. For a readable lookup hit,
the snippet is the start of the control's body cut by the snippet rule; it is
built after resolution, for authorized versions only.

## Search

Top level: `release`, `abstain`, `reason`, `message`, `frameworks`, `hits`,
`gaps`, `next_cursor`, `access_set_version` and `access_unavailable` when set.
`frameworks` is keyed by version handle: `code`, `version`, `name`, `publisher`,
`access` (`free`/`licensed`), `readable`, and `lineage` (`supersedes`, `amends`)
as version handles.

**Ranking.** The frameworks retrieval recipe has the law recipe's structure
([retrieval recipe](retrieval-recipe.md)): two arms, one reciprocal-rank fusion
and an abstention threshold. The candidates are the controls of the versions the
filters allow, taken from the body index of a version the caller can read and
from the public-field index (citation, our title, kind) of every other version.
Each arm produces **one** rank list across versions, cut at the recipe's arm
depth:

- **Dense**: cosine between the query vector and the candidate's vectors, by
  exact scan; a control's score is its best chunk (body chunks for readable
  versions, its public-field vector otherwise).
- **Lexical**: only controls with a lexical match. A readable control scores its
  best body chunk's BM25, computed with that version's chunk statistics; a
  reduced control scores its public record with the public-field index's
  statistics. All matches are ordered by that score in one list.

Reciprocal-rank fusion runs once over the two lists, so no version gets a place
by being best within itself. `score` is the fused score; ties break by framework
code, then version order (newest first), then control order, so the order is
total. As in the law recipe, abstention compares the top fused hit's best
permitted cosine with the threshold, not the fused score, so a filter that
leaves one control still abstains on an unrelated query.

BM25 scores from different statistics are not on exactly one scale; the
evaluation measures the effect on the golden cases, and only a measured recipe
change alters the lexical arm. For the invariant the scale does not matter: an
unreadable version contributes only public-field candidates scored with
public-field statistics and public-field vectors, and the body statistics in
play belong only to versions the caller can read, so no unreadable body can move
any rank, score or abstention.

**Parameters.** Every serving parameter (fusion constant, arm depths, abstention
threshold) is set from free versions' text and the public fields of licensed
versions only, never from licensed body text, so a change to licensed text
cannot move a parameter that a reduced caller's response depends on. Evaluation
with full access measures the recipe; it does not set parameters. The values are
recorded in [frameworks](frameworks.md#evaluation) after the first measured run.

**A hit**: `handle`, `citation`, `kind`, `title` (ours), `status`, `path` (the
citations of its ancestors, root first), `score` or `lookup`, `mappings` (count
of edges with this control at either end) and `text_withheld`. A readable hit
adds `title_original` and `snippet`: the body chunk with the best fused rank,
cut by the law [snippet rule](mcp-results.md#snippet-rule) with the control as
its one member. A reduced hit has neither, and `text_withheld: true`.

**Cursors** follow [continuations](#continuations).

## Read

Input: `corpus`, `release`, `items`. An item is `{handle}` (version or control)
or `{next}`. Item errors as in law [read](mcp-results.md#read): `invalid_item`,
`invalid_handle`, `not_found`, `release_expired`, `invalid_next`,
`unit_too_large`; they do not fail the call. `not_found` means the handle names
no control in the release, which is public.

An item emits a fixed sequence of units:

1. **Head**: the public fields (`handle`, `citation`, `kind`, `path`, `title`,
   `status`, `lineage`, `access`, `text_withheld`), for every caller.
2. For a readable control only: `title_original`, then each body paragraph in
   order (a paragraph is a nonempty line of the stored body, without its line
   break), then notes, then tables, one unit each; split field `text`.
3. **Mappings**: edge units as in [mappings](#mappings), in that order; split
   field `evidence` when present, otherwise the unit is atomic.
4. **Outline**: one unit per child control, `{citation, kind, title}`, ours
   only; atomic.

A version item emits its version entry as the head, then its outline. Caps, part
size, the cut rule, `part`/`parts`, `not_read`, `next` and `complete` are the
law rules. `next` binds the release, the item, its mode, `access_set_version`
and the next unit or part. For an unreadable control the sequence contains only
units built from public fields, so its boundaries do not depend on text.

## Mappings

Input: `handle` (edges at one control) or `from` and `to` (version handles:
edges from one version to the other), optional `relation[]`, `limit` and
`cursor`. Output: `release`, `edges`, `next_cursor`, `access_set_version`.

An edge: `from` and `to` ends, `relation` (`equivalent`, `subset`, `superset`,
`intersects`, `related`), `source` (mapping source name) and `resolved`. Edges
are directed as stored; a `handle` query returns edges where the control is
either end. The build rejects two edges with the same (`from`, `to`, `relation`,
`source`), so that tuple is unique; it is the order and the continuation key.
The page size and the evidence size per edge are in [budgets](budgets.md):
evidence longer than its cap is cut by the law cut rule and carries
`evidence_truncated: true`; the full value is in the control's read units.

**Every end is reduced for every caller**: `handle`, `citation`, `version`,
`title` (ours). An unresolved end gives the target's business key (code,
version, citation) and no title.

**Evidence.** Each evidence value carries, from the build, the set of licensed
versions whose text it quotes or paraphrases, whatever source supplied it. An
empty set means public. A value whose set cannot be established fails the build.
The edge carries `evidence` only when the set is empty or the caller can read
every version in it. Otherwise the edge has no `evidence` field and no
withholding indicator, so an edge with private evidence and an edge with none
look the same to that caller.

## Diagnostics and counts

`corpus_status` with `corpus: "frameworks"`: `release`, `access_set_version`,
`retrieval_recipe_status` (`placeholder` or `measured`, also on `quality_gaps`),
and `versions`, keyed by version handle, each with `access`, `readable`,
`controls`, `edges_resolved`, `edges_unresolved` (edges whose `from` end is in
that version, so each edge counts once) and `lineage`. A readable version adds
`sources`: text source and completeness per source file. Totals are sums over
these fields. No chunk, token, vocabulary or byte count is reported.

`quality_gaps` with `corpus: "frameworks"`, optional `versions[]` and `kind`. A
gap's identity is its key (`kind`, version handle, subject citation or business
key, detail) computed from public data for `public` gaps; the build fails on
duplicate keys. Pages are ordered and continued by that key, never by a database
sequence, so adding or removing a private gap cannot renumber or move a public
one. Each gap kind is classified at build: `public` or `text`, as the gap kind
table in [build](frameworks-build.md#gaps-and-signing) fixes it. A caller gets
`public` gaps for every version and `text` gaps only for readable versions; the
query filters by class and version, so unreadable text gaps are never read. A
gap detail names identities and counts and never quotes text, for anyone.

Errors are the law set with fixed messages: `invalid_request`, `invalid_handle`,
`not_found` (read items), `release_expired`, `cursor_expired`, `invalid_next`,
`unit_too_large`, `rate_limited` and `unavailable` (authentication or release
failures). None of them depends on text or on whether a licensed body exists.

## Continuations

Every cursor and `next` is opaque, signed, and carries its operation and these
bindings; any difference is rejected:

| Operation      | Bound                                                                                                     |
| -------------- | --------------------------------------------------------------------------------------------------------- |
| `search`       | release, filters, query, mode, `access_set_version`, last hit position                                    |
| `read`         | release, item, mode, `access_set_version`, next unit or part                                              |
| `mappings`     | release, selector (`handle` or `from`/`to`), `relation[]`, `limit`, `access_set_version`, last edge tuple |
| `quality_gaps` | release, `versions[]`, `kind`, `access_set_version`, last gap key                                         |

For `search`, `mappings` and `quality_gaps`, the error fails the call: a
different release or `access_set_version` gives `cursor_expired`; any other
difference, or a token for another operation, gives `invalid_request`. For
`read`, a `next` is checked per item and its error fails only that item, as in
law: a different release gives `release_expired`, and a different
`access_set_version`, another operation or any other difference gives
`invalid_next`; the other items of the call are read normally.

## Web API

Routes answer `Cache-Control: no-store` and are excluded from the Nitro and CDN
public caches. They use the objects above.

| Route                                      | Body                                  |
| ------------------------------------------ | ------------------------------------- |
| `GET /api/v1/frameworks`                   | `corpus_status` for frameworks        |
| `GET /api/v1/frameworks/versions/{handle}` | read of a version handle              |
| `GET /api/v1/frameworks/controls/{handle}` | read of a control handle              |
| `POST /api/v1/frameworks/search`           | search; CSRF as other POST routes     |
| `GET /api/v1/frameworks/mappings`          | `mappings`, input as query parameters |
| `GET /api/v1/frameworks/gaps`              | `quality_gaps`                        |

`{handle}` is one path segment. The router decodes the segment once; the result
must be a canonical handle of the route's kind, or the request is
`invalid_handle` before any lookup or access decision. No route carries a second
identity that could disagree with the handle. With `SERVER_FRAMEWORKS=false`
every route answers 404 and the tools are absent from `tools/list`.

## Invariant

How each rule above follows the invariant, and the tests that prove it:
[results and the invariant](frameworks-results-invariant.md).
