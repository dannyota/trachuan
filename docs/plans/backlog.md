# Backlog

One line per item: date, area, finding, source.

- 2026-11-08, taxonomy, ask Cục Kiểm tra văn bản QPPL (Bộ Tư pháp, 024.62739660) about API or dump access to the current codification, design/README.md spikes.
- 2026-11-08, auth, wait for the aboutme refactor of oauthsrv, mcpapi and Google login before copying them, design/README.md spikes.
- 2026-11-08, repo, choose a license (banhmi used Apache-2.0, aboutme AGPL-3.0).
- 2026-11-09, classify, seeds cover only 25.3% of primary articles; size the LLM batch and the held-out set for the other 75%, docs/research/phapdien-join-2026-11.md.
- 2026-11-09, classify, let consolidated-text articles inherit seeds through `consolidates` relations (4,648 of 8,963) with their own confidence and a held-out slice, same note finding 2.
- 2026-11-09, taxonomy import, key package articles by load sequence; MAPC and (đề mục, label) repeat, same note finding 4.
- 2026-11-09, auth, verify ChatGPT and Claude.ai connectors, refresh and revocation against traquydinh's own OAuth before release; aboutme proved only Claude Code, docs/research/oauth-clients-2026-11.md.
- 2026-11-09, retrieval, measure hybrid recall (Vietnamese_Embedding_v2 dense plus BM25, RRF) against banhmi's 93.8% before release 1, docs/research/embedder-2026-11.md.
- 2026-11-09, deploy, measure Vietnamese_Embedding_v2 ONNX FP32 latency and RSS on the target Graviton host in the acceptance gates, ADR 0009.
- 2026-11-09, retrieval, revisit Harrier-0.6B with a full-corpus run if hybrid results leave a gap; it led MRR@8 on the screening corpus, ADR 0009.
- 2026-11-09, deploy, check t4g burstable CPU credits under sustained embedder load in the host acceptance gates; credit exhaustion would raise query latency, final review of the spikes branch.
- 2026-11-09, pipeline, relation targets outside the discovery scope are gaps during ingest (banhmi held 1,108 context documents); decide one-hop fetching before the index plan, docs/plans/2026-11-09-corpus-ingest.md.
- 2026-11-09, pipeline, add checkpoints to S3 in addition to the external disk, docs/plans/2026-11-09-corpus-ingest.md.
