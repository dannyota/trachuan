# Design

Intended behavior of trachuan. Decisions with alternatives are in the
[ADRs](../adr/README.md); numeric limits in [budgets](budgets.md) and
[host budgets](budgets-host.md).

| Note                                                                             | Owns                                                                                                             |
| -------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------- |
| [system](system.md)                                                              | Success criteria, components, database roles, repository layout                                                  |
| [data](data.md)                                                                  | Corpus and auth data models                                                                                      |
| [workshop data](workshop-data.md)                                                | Workshop tables, scope contracts, relation support and expand observations                                       |
| [findings and reviewed knowledge](knowledge-loop.md)                             | Detectors, findings, the reviewed-knowledge contract, intake                                                     |
| [release evidence](release-evidence.md)                                          | `findings.json`, `identity.jsonl` and `changes.jsonl`: shapes, bindings and the release gate                     |
| [retrieval](retrieval.md)                                                        | Query validation, ranking, validity, subject filter, release pinning, MCP tools, web API                         |
| [taxonomy](taxonomy.md)                                                          | Subject tree import, classification layers, evaluation gate                                                      |
| [pipeline](pipeline.md)                                                          | Write path on Temporal, stages, batch submissions, release artifact                                              |
| [sources](sources.md)                                                            | Vietnam discovery, relation context, secondary source and loss approval                                          |
| [relations from text](relations-text.md)                                         | Provision-level amend, repeal and suspend relations parsed from article text, fail closed                        |
| [scoped amendment lists](relations-text-scoped.md)                               | Version 2 of relations from text: an article lead names the target, its items name provisions                    |
| [normalization](normalization.md)                                                | Dates, numbers, source status mapping and consolidated status derivation                                         |
| [calibration](calibration.md)                                                    | Provisional abstention threshold                                                                                 |
| [text extraction](text-extraction.md)                                            | Provision trees from HTML transcriptions and attached files when the source has no tree                          |
| [clauses and points](text-structure.md)                                          | Clause and point provisions inside accepted articles of parsed text, fail closed                                 |
| [heading-only trees](heading-only-trees.md)                                      | When a tree with heading-only articles is read from its transcription                                            |
| [PDF text layer](pdf-text.md)                                                    | When extract reads a PDF's own text instead of OCR: gate, measured counts, extractor                             |
| [OCR store](ocr-store.md)                                                        | Stored OCR text and cross-check records, the banhmi import and how extract reads them                            |
| [OCR cross-check](ocr-check.md)                                                  | How a hosted model checks stored OCR text against its PDF, verdicts, batches and spend limits                    |
| [OCR cross-check version 3](ocr-check-v3.md)                                     | JPEG pages at 200 dpi, direct requests, version 4 at 150 dpi                                                     |
| [OCR cross-check version 5](ocr-check-v5.md)                                     | Proposed: empty items allowed, coverage field, placeholder items named                                           |
| [fresh OCR requests](ocr-vision.md)                                              | Google Cloud Vision requests for PDFs the OCR store lacks: page images, limits, command                          |
| [batch submissions](batch-submissions.md)                                        | Submission intent log and recovery for external batch services                                                   |
| [scope adoption](scope-adoption.md)                                              | Schema version 3, inventory version 2, build loading by final membership                                         |
| [validity corrections](validity-status.md)                                       | Text rules, manual corrections and review queue for wrong or missing source validity                             |
| [membership](membership.md)                                                      | Support supersession, retention, withdrawal, fetch drift and removal lifetime                                    |
| [pinned documents](pinned-documents.md)                                          | Keep a document the source deleted, read from verified payloads the source list pins                             |
| [source execution](source-execution.md)                                          | Shared source interfaces, finalization, retries and durable pauses                                               |
| [retry of source gaps](source-retry.md)                                          | What a restart retries, rewinds and clears, and the reuse of a run's own acquisitions                            |
| [fetch reuse](fetch-reuse.md)                                                    | What a new run keeps from the previous VBPL read when `updatedDate` is unchanged                                 |
| [attachments by content hash](fetch-attachments.md)                              | Copy or download each VBPL attachment by its source ETag, fail closed                                            |
| [auth](auth.md)                                                                  | Google login, OAuth for MCP, CIMD, revocation, account lifecycle                                                 |
| [accounts](accounts.md)                                                          | Email and password accounts, TOTP, recovery codes, key ring                                                      |
| [control frameworks](frameworks.md)                                              | Framework corpus kind: sources, data model, pipeline, serving, release, downstream products                      |
| [control frameworks: access](frameworks-access.md)                               | Access classes, entitlements, reduced record, the invariant, places for licensed material                        |
| [control frameworks: results](frameworks-results.md)                             | Handles, inputs, search, read, mappings, diagnostics and web routes for frameworks                               |
| [control frameworks: results and the invariant](frameworks-results-invariant.md) | Why each results rule holds the invariant, and its tests                                                         |
| [control frameworks: evaluation](frameworks-evaluation.md)                       | Golden cases, hits, cohorts, measured recipe, release invariant check, floors                                    |
| [control frameworks: release](frameworks-release.md)                             | Bucket, upload, signing workflow, host phase, schema, key and flag, and order for frameworks releases            |
| [control frameworks: build](frameworks-build.md)                                 | Offline build command: steps, artifacts, resume, gaps and signing evidence                                       |
| [account mail](account-mail.md)                                                  | Mail outbox: classes, sender, content                                                                            |
| [web API](web-api.md)                                                            | Google login flow, web sessions, CSRF, `/api/v1` routes, OpenAPI contract                                        |
| [OAuth](oauth.md)                                                                | MCP authorization server: clients, consent, codes, tokens, aboutme import gaps                                   |
| [error reports](error-reports.md)                                                | Agent and web reports of corpus defects, triage, export to reviewed fixes                                        |
| [web](web.md)                                                                    | Routes, pages, caching, routing, CSP, page shell                                                                 |
| [web visual system](web-visual.md)                                               | Colour tokens, type, shape, stamps, logo, motion, contrast table                                                 |
| [web voice](web-voice.md)                                                        | Voice rules, claims, voice and typeface per content field, maker panel                                           |
| [public document summaries](web-public-documents.md)                             | The summary URL with no session, fields, outline and heading approval, availability, caching, head tags, sitemap |
| [framework pages](web-frameworks.md)                                             | Framework list, version, control and search pages, words per field, access labels, caching and the flag          |
| [serving public summaries](web-public-documents-serving.md)                      | Summary rows, public document endpoints, admission and capacity, tests, capacity acceptance                      |
| [discovery](discovery.md)                                                        | Indexable routes, robots.txt, sitemap, head tags, structured data, llms.txt, measurement                         |
| [deployment](deployment.md)                                                      | Host, deploy, secrets, release activation, backups, observability                                                |
| [backups](backups.md)                                                            | pgBackRest image, configuration, schedules, restore proof and results                                            |
| [pull deployment](pull-deployment.md)                                            | Signed release from a tag, host updater, rollback, remote execution                                              |
| [corpus release](corpus-release.md)                                              | Upload, signed corpus manifest, host restore and activation                                                      |
| [corpus release changes](corpus-release-changes.md)                              | The order in which schema versions and recipe changes ship                                                       |
| [budgets](budgets.md)                                                            | Request, pipeline, indexing, retrieval and Temporal limits                                                       |
| [host budgets](budgets-host.md)                                                  | Hosting cost, host memory, alarms and disk                                                                       |
| [host watch](host-watch.md)                                                      | Telegram alerts from the host timer: signals, messages, state and secrets                                        |
| [caching](caching.md)                                                            | Server caches for query vectors, search responses, read items and document entries                               |
| [index](index.md)                                                                | Index scope, build inventory, normalization, local database recovery                                             |
| [chunk](chunk.md)                                                                | Chunk recipe, source spans, omissions and workshop projection                                                    |
| [embedder](embedder.md)                                                          | Embedding HTTP contract, Go query and Python document services, embedding artifacts                              |
| [corpus schema](corpus-schema.md)                                                | Exact shared corpus template contract and schema version                                                         |
| [corpus schema migration](corpus-schema-migration.md)                            | Restore requirements, schema versions and rollback across them                                                   |
| [lexical](lexical.md)                                                            | Shared normalization, hashing, BM25 and statistics                                                               |
| [retrieval recipe](retrieval-recipe.md)                                          | Shared Go API, recipes 2 and 3, validity and evidence shaping                                                    |
| [sibling clause merge](retrieval-siblings.md)                                    | Candidate recipe 3 rule: clause hits of one article merge into one article hit, measured first                   |
| [search results](mcp-results.md)                                                 | Compact search hits, snippets, related pointers, batch `read`, guide flow                                        |
| [evaluation](evaluation.md)                                                      | Cohorts, identity mapping, calibration, metrics and caches                                                       |
| [MRR@8 tolerance](evaluation-mrr-tolerance.md)                                   | The bounded exception to MRR@8 non-regression on a corpus-only release, against a high-water mark                |
| [evaluation fixtures](evaluation-fixtures.md)                                    | Authored case fixtures, the identity map and identity validation                                                 |
| [serialized recipe](retrieval-recipe-serialized.md)                              | Serialized retrieval recipe, hash and checks                                                                     |
| [retrieval performance](retrieval-performance.md)                                | Measured statement costs per search and topic page; one change per cost                                          |
| [sparse postings](sparse-postings.md)                                            | Exact in-memory sparse arm for the active release, with its load-time gate                                       |

Spikes done on 2026-11-09: OAuth clients
([note](../research/oauth-clients-2026-11.md)), the dense model
([ADR 0009](../adr/0009-dense-embedding-recipe.md)), and the Bộ pháp điển join
([note](../research/phapdien-join-2026-11.md)).

Out of scope for the first release: other countries; dossiers, watch lists,
exports; organizations; paid tier; a platform model or reranker; translation;
query history; Coraza or CrowdSec; deployment observer; serving Pháp điển data.
