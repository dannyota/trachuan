# relationSQL at production size

Observed 2026-11-12 on this MacBook (PostgreSQL 18.6, parallel workers off)
against the local release `corpus_vn_20261111_4` (schema 4): 3,841 documents,
298,500 provisions, 19,141 relations, all `vbpl_diagram` and all document-level
(no `source_provision_id` or `target_provision_id`). Relations per document: p90
17, maximum 943. Query: `relationSQL` in `internal/retrieval/hydrate_tree.go` at
main 0bce570 (after the relation warnings work), run with
`EXPLAIN (ANALYZE, BUFFERS)` and `$2 = 21`.

## Plans

The incident branch's `OR` join is a BitmapOr over
`relation_source_document_id_idx` and `relation_target_document_id_idx`, then a
bitmap heap scan; the inherited-warning branch uses
`relation_target_document_id_idx` and its own `LIMIT`; the final relation row is
an index scan on `relation_pkey` per selected id. No sequential scan on
`relation` and no nested loop over the whole table.

| Id set                                                                   | Groups | Relation rows selected   | `relation_pkey` lookups | Execution |
| ------------------------------------------------------------------------ | ------ | ------------------------ | ----------------------- | --------- |
| 20 articles of the most related document (943 relations)                 | 20     | 943 per group            | 18,860                  | 91 ms     |
| one article from each of the 20 most related documents (16 had articles) | 16     | 330 per group on average | 5,261                   | 37 ms     |

## Finding

The incident branch has no per-group bound: every group fetches all of its
document's relations, with evidence, before `shapeRelations` keeps 20 and sets
`relations_omitted`. Ordering that branch by the recipe relation order with
`LIMIT RelationCap+1` (tried on a branch) cut the two cases to 35 ms and 17 ms
with 21 lookups per group, but broke search-hit warnings: hit warnings and
`warnings_omitted` are taken from the fetched rows, so a cap on the mixed rows
undercounts omitted warnings and lets earlier kinds (`cites`, `guided_by`,
`implements`, `legal_basis`) displace warning rows, which
[search results](../design/mcp-results.md#warnings) forbids. The change was
dropped. Bounding this branch needs warning rows selected and counted on their
own, as read units already do; the backlog records it.
