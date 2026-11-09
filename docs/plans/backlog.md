# Backlog

One line per item: date, area, finding, source.

- 2026-11-08, taxonomy, ask Cục Kiểm tra văn bản QPPL (Bộ Tư pháp, 024.62739660) about API or dump access to the current codification, design/README.md spikes.
- 2026-11-08, auth, wait for the aboutme refactor of oauthsrv, mcpapi and Google login before copying them, design/README.md spikes.
- 2026-11-08, repo, choose a license (banhmi used Apache-2.0, aboutme AGPL-3.0).
- 2026-11-08, pipeline, Temporal workflow versioning policy and history size check before the first full run, ADR 0008.
- 2026-11-09, classify, seeds cover only 25.3% of primary articles; size the LLM batch and the held-out set for the other 75%, docs/research/phapdien-join-2026-11.md.
- 2026-11-09, classify, let consolidated-text articles inherit seeds through `consolidates` relations (4,648 of 8,963) with their own confidence and a held-out slice, same note finding 2.
- 2026-11-09, taxonomy import, key package articles by load sequence; MAPC and (đề mục, label) repeat, same note finding 4.
- 2026-11-09, auth, verify ChatGPT and Claude.ai connectors, refresh and revocation against traquydinh's own OAuth before release; aboutme proved only Claude Code, docs/research/oauth-clients-2026-11.md.
