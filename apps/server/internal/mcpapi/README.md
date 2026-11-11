# Cache wiring

Create one `mcpapi.NewCaches(clock)` for the server. Set `Config.Caches`,
`retrieval.Options.Documents = caches.Documents` and
`retrieval.Options.Sequences = caches.Sequences`. Nil options disable caching.

Call `caches.ActivateRelease(release)` on the first active pointer observation
and every change, including clearing the pointer with an empty string. Deliver
these notifications in the resolver's pointer observation order, before lending
leases. Do not activate from a request finishing or an old lease returning.
Notifications must also cover a changed pointer whose engine fails to open.
Old leases can finish, but their reads and writes cannot enter the new caches.

Create one `embedcache.NewVectors(clock)` and keep it across releases. Wrap the
query client with `embedcache.New(client, verifiedQueryRecipe, vectors)` before
opening retrieval engines. Create a new wrapper when the verified query recipe
changes; it can share `vectors`. The wrapper forwards `Recipe` and never calls
it during an embedding lookup.

Export each cache's `Stats()` under its fixed cache name. `Hits`, `Misses` and
`Evictions` are cumulative; `Bytes` is the current charged size. Expiry and
purge removals count as evictions. Do not attach keys or account labels.

Caps use decimal MB. Document and read caches retain serialized immutable
metadata. Read sequences retain the parts already computed while paging;
unvisited units use the existing splitter when needed. Relation entries retain
one existing SQL window per item and reuse it only at that window's starting
unit. Other positions compute the original bounded query and replace the window
after a successful transaction. No cache miss loads extra relation pages.
