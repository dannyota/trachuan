# Plans

Plans describe work that remains. A plan is deleted when its work lands; git history keeps it. States: `planned`, `in progress`, `landed`, `verified`, `blocked`. Findings not fixed at once go to [`backlog.md`](backlog.md).

## Order

|#|Plan|Produces|Depends on|State|
|-|-|-|-|-|
|0|[Spikes](2026-11-09-spikes.md)|Local PostgreSQL with the banhmi Vietnam corpus; research notes `oauth-clients-2026-11`, `embedder-2026-11`, `phapdien-join-2026-11`; ADR 0005 and 0004 History, new ADR 0009|design approved 2026-11-09|landed|
|1|Corpus pipeline|`apps/corpus`: workshop schema, Temporal worker, stages through `build` and `eval`, signed release to S3|Plan 0 landed: ADR 0009 recipe, taxonomy mapping, backlog items for classify|not written; next|
|2|Server|`apps/server`: auth, OAuth for MCP, retrieval, MCP tools, web API, OpenAPI document|Plan 1 release in a local database; Plan 0 OAuth decision|not written|
|3|Web|`apps/web`: public pages, search, reader, account|Plan 2 OpenAPI document|not written|
|4|Host and release|`deploy/`: OpenTofu, host bundle, Quadlet, pgBackRest, `release activate`, CI and deploy workflows|Plans 1 to 3 images|not written|

Each later plan is written with the writing-plans skill when its dependency lands, against `docs/design/README.md` and the research notes from Plan 0.
