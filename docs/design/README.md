# Design

Intended behavior of trachuan. Decisions with alternatives are in the
[ADRs](../adr/README.md); numeric limits in [budgets](budgets.md) and
[host budgets](budgets-host.md).

| Note                                                | Owns                                                                                          |
| --------------------------------------------------- | --------------------------------------------------------------------------------------------- |
| [system](system.md)                                 | Success criteria, components, database roles, repository layout                               |
| [data](data.md)                                     | Corpus, auth and workshop data models                                                         |
| [retrieval](retrieval.md)                           | Query validation, ranking, validity, subject filter, release pinning, MCP tools, web API      |
| [taxonomy](taxonomy.md)                             | Subject tree import, classification layers, evaluation gate                                   |
| [pipeline](pipeline.md)                             | Write path on Temporal, stages, batch submissions, release artifact                           |
| [sources](sources.md)                               | Vietnam discovery, relation context, secondary source and loss approval                       |
| [relations from text](relations-text.md)            | Provision-level amend, repeal and suspend relations parsed from article text, fail closed     |
| [scoped amendment lists](relations-text-scoped.md)  | Version 2 of relations from text: an article lead names the target, its items name provisions |
| [normalization](normalization.md)                   | Dates, numbers, source status mapping and consolidated status derivation                      |
| [calibration](calibration.md)                       | Provisional abstention threshold                                                              |
| [text extraction](text-extraction.md)               | Provision trees from HTML transcriptions and attached files when the source has no tree       |
| [clauses and points](text-structure.md)             | Clause and point provisions inside accepted articles of parsed text, fail closed              |
| [heading-only trees](heading-only-trees.md)         | When a tree with heading-only articles is read from its transcription                         |
| [PDF text layer](pdf-text.md)                       | When extract reads a PDF's own text instead of OCR: gate, measured counts, extractor          |
| [OCR store](ocr-store.md)                           | Stored OCR text and cross-check records, the banhmi import and how extract reads them         |
| [OCR cross-check](ocr-check.md)                     | How a hosted model checks stored OCR text against its PDF, verdicts, batches and spend limits |
| [OCR cross-check version 3](ocr-check-v3.md)        | JPEG pages at 200 dpi, direct requests, version 4 at 150 dpi                                  |
| [OCR cross-check version 5](ocr-check-v5.md)        | Proposed: empty items allowed, coverage field, placeholder items named                        |
| [fresh OCR requests](ocr-vision.md)                 | Google Cloud Vision requests for PDFs the OCR store lacks: page images, limits, command       |
| [batch submissions](batch-submissions.md)           | Submission intent log and recovery for external batch services                                |
| [scope adoption](scope-adoption.md)                 | Schema version 3, inventory version 2, build loading by final membership                      |
| [validity corrections](validity-status.md)          | Text rules, manual corrections and review queue for wrong or missing source validity          |
| [membership](membership.md)                         | Support supersession, retention, withdrawal, fetch drift and removal lifetime                 |
| [source execution](source-execution.md)             | Shared source interfaces, finalization, retries and durable pauses                            |
| [retry of source gaps](source-retry.md)             | What a restart retries, rewinds and clears, and the reuse of a run's own acquisitions         |
| [fetch reuse](fetch-reuse.md)                       | What a new run keeps from the previous VBPL read when `updatedDate` is unchanged              |
| [attachments by content hash](fetch-attachments.md) | Copy or download each VBPL attachment by its source ETag, fail closed                         |
| [auth](auth.md)                                     | Google login, OAuth for MCP, CIMD, revocation, account lifecycle                              |
| [web API](web-api.md)                               | Google login flow, web sessions, CSRF, `/api/v1` routes, OpenAPI contract                     |
| [OAuth](oauth.md)                                   | MCP authorization server: clients, consent, codes, tokens, aboutme import gaps                |
| [error reports](error-reports.md)                   | Agent and web reports of corpus defects, triage, export to reviewed fixes                     |
| [web](web.md)                                       | Routes, pages, visual system                                                                  |
| [deployment](deployment.md)                         | Host, deploy, secrets, release activation, backups, observability                             |
| [budgets](budgets.md)                               | Request, pipeline, indexing, retrieval and Temporal limits                                    |
| [host budgets](budgets-host.md)                     | Hosting cost, host memory, alarms and disk                                                    |
| [caching](caching.md)                               | Server caches for query vectors, search responses, read items and document entries            |
| [index](index.md)                                   | Index scope, build inventory, normalization, local database recovery                          |
| [chunk](chunk.md)                                   | Chunk recipe, source spans, omissions and workshop projection                                 |
| [embedder](embedder.md)                             | Embedding HTTP contract, Go query and Python document services, embedding artifacts           |
| [corpus schema](corpus-schema.md)                   | Exact shared corpus template contract and schema version                                      |
| [lexical](lexical.md)                               | Shared normalization, hashing, BM25 and statistics                                            |
| [retrieval recipe](retrieval-recipe.md)             | Shared Go API, recipe 1, validity and evidence shaping                                        |
| [search results](mcp-results.md)                    | Compact search hits, snippets, related pointers, batch `read`, guide flow                     |
| [evaluation](evaluation.md)                         | Cohorts, identity mapping, calibration, metrics and caches                                    |
| [evaluation fixtures](evaluation-fixtures.md)       | Authored case fixtures, the identity map and identity validation                              |
| [serialized recipe](retrieval-recipe-serialized.md) | Serialized retrieval recipe, hash and checks                                                  |

Spikes done on 2026-11-09: OAuth clients
([note](../research/oauth-clients-2026-11.md)), the dense model
([ADR 0009](../adr/0009-dense-embedding-recipe.md)), and the Bộ pháp điển join
([note](../research/phapdien-join-2026-11.md)).

Out of scope for the first release: other countries; dossiers, watch lists,
exports; organizations; paid tier; a platform model or reranker; translation;
query history; Coraza or CrowdSec; deployment observer; serving Pháp điển data.
