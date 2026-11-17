# Control frameworks: results and the invariant

Why each rule of [results](frameworks-results.md) holds the
[invariant](frameworks-access.md#the-invariant), and the tests that prove it.

## How each rule follows it

| Rule                                                       | Why                                                              |
| ---------------------------------------------------------- | ---------------------------------------------------------------- |
| Authenticate first; only the entitlement lookup falls back | no unauthenticated caller reaches any result                     |
| Queries filtered by projection, not after                  | reads and timing depend only on visible data                     |
| Public-field index for unreadable versions                 | hits, order, scores, abstention ignore body text                 |
| Cursors and `next` bound to `access_set_version`           | a page never continues under another access set                  |
| Reduced units built from public fields only                | read part boundaries do not depend on text                       |
| Edge ends always reduced; evidence by dependency set       | no edge carries text the caller may not read                     |
| No withholding indicator on edges                          | presence of private evidence is not observable                   |
| Gap and count classification in the query                  | diagnostics of unreadable versions are public only               |
| Text-independent errors and `access_set_version`           | no error or version value signals licensed text                  |
| Parameters set without licensed body text                  | a recipe change from private text cannot move a reduced response |
| Gap keys and edge tuples from public data                  | private gaps or evidence cannot renumber or reorder public rows  |
| One handle per route                                       | no route authorizes one identity and reads another               |
| `no-store`, partitioned caches                             | no cache crosses access sets                                     |

## Tests

The invariant suite of [evaluation](frameworks.md#evaluation) runs every
operation here (search, lookup, read with paging, mappings, status, gaps, every
route, cache hits and misses) with a reduced, a mixed and a full access set. It
changes the body, original title, notes, tables and private evidence of a
synthetic licensed version and asserts byte-identical responses, cursors, `next`
tokens, `access_set_version` and errors for every caller who cannot read that
version. A second case rebuilds the synthetic release with changed licensed text
and asserts identical recipe parameters, gap keys and edge order. It also
records, per request, which tables and indexes were read for each version, and
fails when a request reads any private structure of a version the caller cannot
read.
