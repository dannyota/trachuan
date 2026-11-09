# Design

Intended behavior of trachuan. Decisions with alternatives are in the
[ADRs](../adr/README.md); numeric limits in [budgets](budgets.md).

| Note                        | Owns                                                                                     |
| --------------------------- | ---------------------------------------------------------------------------------------- |
| [system](system.md)         | Success criteria, components, database roles, repository layout                          |
| [data](data.md)             | Corpus, auth and workshop data models                                                    |
| [retrieval](retrieval.md)   | Query validation, ranking, validity, subject filter, release pinning, MCP tools, web API |
| [taxonomy](taxonomy.md)     | Subject tree import, classification layers, evaluation gate                              |
| [pipeline](pipeline.md)     | Write path on Temporal, stages, batch submissions, release artifact                      |
| [auth](auth.md)             | Google login, OAuth for MCP, CIMD, revocation, account lifecycle                         |
| [web](web.md)               | Routes, pages, visual system                                                             |
| [deployment](deployment.md) | Host, deploy, secrets, release activation, backups, observability                        |
| [budgets](budgets.md)       | Cost, memory, request limits, retrieval and Temporal bounds                              |

Spikes done on 2026-11-09: OAuth clients
([note](../research/oauth-clients-2026-11.md)), the dense model
([ADR 0009](../adr/0009-dense-embedding-recipe.md)), and the Bộ pháp điển join
([note](../research/phapdien-join-2026-11.md)).

Out of scope for the first release: other countries; dossiers, watch lists,
exports; organizations; paid tier; a platform model or reranker; translation;
query history; Coraza or CrowdSec; deployment observer; serving Pháp điển data.
