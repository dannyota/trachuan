# Plans

Plans describe work that remains. A plan is deleted when its work lands; git history keeps it. States: `planned`, `in progress`, `landed`, `verified`, `blocked`. Findings not fixed at once go to [`backlog.md`](backlog.md).

## Order

|#|Plan|Produces|Depends on|State|
|-|-|-|-|-|
|0|Spikes (deleted when Plan 1 starts)|Local PostgreSQL with the banhmi Vietnam corpus; research notes `oauth-clients-2026-11`, `embedder-2026-11`, `phapdien-join-2026-11`; ADR 0005 and 0004 History, new ADR 0009|design approved 2026-11-09|landed|
|1|[Corpus pipeline core and VBPL ingest](2026-11-09-corpus-ingest.md)|`apps/corpus`: workshop schema, Temporal worker, run commands, checkpoints, stages `discover`, `fetch`, `extract` (VBPL tree), `relate`; CI|Plan 0 landed|planned|
|2|Index and release|stages `chunk`, `embed`, lexical vectors, taxonomy import and seeds, `build`, shared retrieval package, `eval`, signed `release` to S3|Plan 1; hybrid study `docs/research/retrieval-hybrid-2026-11.md`|not written|
|3|Text extraction|documents without a VBPL tree: HTML transcription, PDF, OCR; secondary sources|Plan 1|not written|
|4|Classification|LLM batch classification, submission intent log, taxonomy evaluation gate|Plan 2|not written|
|5|Server|`apps/server`: auth, OAuth for MCP, retrieval, MCP tools, web API, OpenAPI document|Plan 2 corpus database; Plan 0 OAuth decision|not written|
|6|Web|`apps/web`: public pages, search, reader, account|Plan 5 OpenAPI document|not written|
|7|Host and release|`deploy/`: OpenTofu, host bundle, Quadlet, pgBackRest, `release activate`, deploy workflows|Plans 1 to 6 images|not written|

Each later plan is written with the writing-plans skill when its dependency lands, against `docs/design/README.md` and the research notes from Plan 0.
