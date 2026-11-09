# Plans

Plans describe work that remains. A plan is deleted when its work lands; git history keeps it. States: `planned`, `in progress`, `landed`, `verified`, `blocked`. Findings not fixed at once go to [`backlog.md`](backlog.md).

## Order

|#|Plan|Produces|Depends on|State|
|-|-|-|-|-|
|0|Spikes|Local PostgreSQL with the banhmi Vietnam corpus; research notes `oauth-clients-2026-11`, `embedder-2026-11`, `phapdien-join-2026-11`; ADR 0005 and 0004 History, new ADR 0009|design approved 2026-11-09|landed|
|1|Corpus pipeline core and VBPL ingest|`apps/corpus`: workshop schema, Temporal worker, run commands, checkpoints, stages `discover`, `fetch`, `extract` (VBPL tree), `relate`; CI|Plan 0 landed|landed|
|2|[Index and evaluation](2026-11-09-index-eval.md)|stages `chunk`, `embed`, `build`, `eval`; `apps/embedder`; root `internal/corpusdb`, `internal/retrieval`, `internal/embedding`; legacy and old product cohorts; baseline recipe 1|Plan 1; ADR 0010, 0011|in progress|
|3|Corpus scope|one-hop relation targets, exact number lookups for named documents, keyword list review, retry of source gaps on start (R-14); golden coverage (86 of 96 citations have a document today; 4 of the 5 missing documents are in VBPL, see `docs/research/vn-sources-2026-11.md`)|Plan 1|not written|
|4|Retrieval cases and tuning|development and sealed cases, ADR 0010 trials on cached arms, frozen release-1 recipes|Plan 2|not written|
|5|Taxonomy import and seeds|Pháp điển import, `taxonomy/mapping.yaml`, seeds, subject-filter ordering|Plan 1|not written|
|6|Text extraction|documents without a VBPL tree: HTML transcription, PDF, OCR; `vanban.chinhphu.vn` signed PDFs for central documents VBPL lacks|Plan 1|not written|
|7|Classification|LLM batch classification, submission intent log, taxonomy evaluation gate|Plans 2 and 5|not written|
|8|Server|`apps/server`: auth, OAuth for MCP, retrieval, MCP tools, web API, OpenAPI document|Plan 2 corpus database; Plan 0 OAuth decision|not written|
|9|Web|`apps/web`: public pages, search, reader, account|Plan 8 OpenAPI document|not written|
|10|Host and release|`deploy/`: OpenTofu, host bundle, Quadlet, pgBackRest, signed release manifest and S3 upload, `release activate`, deploy workflows|Plans 1 to 9 images|not written|

Each later plan is written with the writing-plans skill when its dependency lands, against `docs/design/README.md` and the research notes from Plan 0.
