# Backlog

One line per item: date, area, finding, source.

- 2026-11-08, taxonomy, ask Cục Kiểm tra văn bản QPPL (Bộ Tư pháp, 024.62739660) about API or dump access to the current codification, design/README.md spikes.
- 2026-11-08, auth, wait for the aboutme refactor of oauthsrv, mcpapi and Google login before copying them, design/README.md spikes.
- 2026-11-09, classify, seeds cover only 25.3% of primary articles; size the LLM batch and the held-out set for the other 75%, docs/research/phapdien-join-2026-11.md.
- 2026-11-09, classify, let consolidated-text articles inherit seeds through `consolidates` relations (4,648 of 8,963) with their own confidence and a held-out slice, same note finding 2.
- 2026-11-09, taxonomy import, key package articles by load sequence; MAPC and (đề mục, label) repeat, same note finding 4.
- 2026-11-09, auth, verify ChatGPT and Claude.ai connectors, refresh and revocation against trachuan's own OAuth before release; aboutme proved only Claude Code, docs/research/oauth-clients-2026-11.md.
- 2026-11-09, retrieval, measure hybrid recall (Vietnamese_Embedding_v2 dense plus BM25, RRF) against banhmi's 93.8% before release 1, docs/research/embedder-2026-11.md.
- 2026-11-09, deploy, measure Vietnamese_Embedding_v2 ONNX FP32 latency and RSS on the target Graviton host in the acceptance gates, ADR 0009.
- 2026-11-09, retrieval, revisit Harrier-0.6B with a full-corpus run if hybrid results leave a gap; it led MRR@8 on the screening corpus, ADR 0009.
- 2026-11-09, deploy, check t4g burstable CPU credits under sustained embedder load in the host acceptance gates; credit exhaustion would raise query latency, final review of the spikes branch.
- 2026-11-09, pipeline, relation targets outside the discovery scope are gaps during ingest (banhmi held 1,108 context documents); the corpus scope plan fetches one hop, docs/plans/2026-11-09-corpus-ingest.md.
- 2026-11-09, pipeline, add checkpoints to S3 in addition to the external disk, docs/plans/2026-11-09-corpus-ingest.md.
- 2026-11-09, pipeline, `corpus start` does not retry items whose only gaps are `source_unavailable` (or `invalid_source_data` from a gateway response); reset them and delete those gap rows on start, and treat a non-JSON 200 body as retryable, final review R-14.
- 2026-11-09, scope, VBPL `all_words` title search splits keywords into syllables: `tài sản số` and `ngân hàng số` match almost any title containing `số`; the corpus scope plan reviews the keyword list, live run 20261109.
- 2026-11-09, scope, 10 of 96 golden citations name 5 documents absent from run 20261109: 18/2018/TT-NHNN, 58/2021/NĐ-CP and 71/2025/QH15 are relation targets (one hop fetches them); 142/2026/NĐ-CP and 368/2025/NĐ-CP need the keyword or agency list, corpus scope plan.
