# Plans

Plans describe work that remains. A plan is deleted when its work lands; git
history keeps it. States: `planned`, `in progress`, `landed`, `verified`,
`blocked`. Findings not fixed at once go to [`backlog.md`](backlog.md).

## Order

|#|Plan|Produces|Depends on|State|
|-|-|-|-|-|
|0|Spikes|Local PostgreSQL with the banhmi Vietnam corpus; research notes `oauth-clients-2026-11`, `embedder-2026-11`, `phapdien-join-2026-11`; ADR 0005 and 0004 History, new ADR 0009|design approved 2026-11-09|landed|
|1|Corpus pipeline core and VBPL ingest|`apps/corpus`: workshop schema, Temporal worker, run commands, checkpoints, stages `discover`, `fetch`, `extract` (VBPL tree), `relate`; CI|Plan 0 landed|landed|
|2|[Index and evaluation](2026-11-09-index-eval.md)|stages `chunk`, `embed`, `build`, `eval`; `apps/embedder`; root `internal/corpusdb`, `internal/retrieval`, `internal/embedding`; legacy and old product cohorts; baseline recipe 1|Plan 1; ADR 0010, 0011|landed|
|3|[Corpus scope](2026-11-09-corpus-scope.md) and [corpus schema 3](2026-11-10-corpus-schema-3.md)|one-hop relation targets; exact number lookups for named documents; keyword searches in exact match mode; secondary source `chinhphu` (vanban.chinhphu.vn) discovery and fetch for central documents VBPL lacks, starting with the AI documents; retry of source gaps on start (R-14); evidence `docs/research/vn-sources-2026-11.md`|Plan 1|in progress|
|4|Go query embedder|`apps/queryembed` (ADR 0011): ONNX Runtime CPU, INT8 per-channel queries (ADR 0009), through the Go binding, `daulet/tokenizers`, pinned native assets, `ort` build tag and CI job; parity gate against the Python query mode; delete that mode and rename `apps/embedder` to `apps/docembed`; rebuild and evaluate the release with the Go service; latency on the target host in budgets|Plan 2|landed|
|5|[Retrieval cases and tuning](2026-11-11-cases-tuning.md)|development and sealed cohorts with sealed isolation, owner spot checks, trial runner over cached arms, finalist against baseline, history-question policy, release-1 recipe decision|Plans 2, 4 and 12; ADR 0010 gates of 2026-11-11|planned|
|6|Taxonomy import and seeds|Pháp điển import, `taxonomy/mapping.yaml`, seeds, subject-filter ordering|Plan 1|not written|
|7|[Text extraction](2026-11-11-text-extraction.md)|documents without a VBPL tree: HTML transcription, PDF, OCR; `vanban.chinhphu.vn` signed PDFs for central documents VBPL lacks|Plan 1|in progress|
|8|Classification|LLM batch classification, submission intent log, taxonomy evaluation gate|Plans 2 and 6|not written|
|9a|[Server core and MCP read path](2026-11-11-server-core.md)|`apps/server`: auth schema and store, personal bearer tokens, credential middleware, release resolver and engine cache, per-account limits, MCP tools `guide`, `corpus_status`, `search`, `read`, `quality_gaps` (ADR 0013), command and runbook|Plans 2, 4 and 12|planned|
|9b|Web login and web API|Google OIDC login, web sessions, CSRF, `/api/v1` search, read and documents, account endpoints, personal token management, OpenAPI document and generated client|Plan 9a|not written|
|9c|OAuth for MCP|aboutme `oauthsrv` and `mcpapi` import after review, CIMD fetching, DCR, RFC 9728 metadata, ChatGPT and Claude.ai connector acceptance, agent-in-the-loop check of ADR 0013|Plan 9a|not written|
|10|Web|`apps/web`: public pages, search, reader, account|Plan 9b OpenAPI document|not written|
|11|Host and release|`deploy/`: OpenTofu, host bundle, Quadlet, pgBackRest, signed release manifest and S3 upload, `release activate`, deploy workflows|Plans 1 to 10 images|not written|
|12|Compact results and read|`internal/retrieval`: hit path and best member, snippets, document entries and relation pointers, `Engine.Present` (compact and full), `Engine.Read` (outline, provision, range, relations, parts, `next` tokens, input bounds); eval compact diagnostic and size gate (ADR 0013); evidence `docs/research/compact-results-2026-11.md`|Plan 2; ADR 0013|landed|
|13|Agent error reports|Owner request 2026-11-11: an MCP tool and a web form through which users' agents report wrong text, wrong validity, broken citations or missing documents (handle, kind, short note that must not carry the user's question); an auth-database table and a triage queue; accepted reports feed `sources/vn-corrections.yaml`, OCR fixes and identity-map review; design first, with the ADR 0002 privacy rules|Plan 9a (MCP), Plan 9b (web form)|not written|

Each later plan is written with the writing-plans skill when its dependency
lands, against `docs/design/README.md` and the research notes from Plan 0.
