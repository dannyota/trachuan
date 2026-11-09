# Spikes (throwaway)

Everything under `spikes/` is throwaway code that answers one question each.
Nothing here is imported by `apps/`. Plan 1 (corpus pipeline) and Plan 2
(server) delete this directory when the real code lands. Results live in the
research notes, not here.

| Spike            | Question                                                                                                        | Research note                            |
| ---------------- | --------------------------------------------------------------------------------------------------------------- | ---------------------------------------- |
| `corpus/`        | Restore banhmi's Vietnam corpus into local PostgreSQL                                                           | used by the two spikes below             |
| OAuth            | Do Claude.ai, ChatGPT and Claude Code connect to aboutme's OAuth server? (closed by the owner without a retest) | `docs/research/oauth-clients-2026-11.md` |
| `embedder/`      | Which dense recipe serves Vietnam on ARM64?                                                                     | `docs/research/embedder-2026-11.md`      |
| `phapdien-join/` | How much of the corpus does the 2023 Bộ pháp điển package label?                                                | `docs/research/phapdien-join-2026-11.md` |

Run `make db-up`, then `bash spikes/corpus/restore.sh`, then
`podman exec -i trachuan_postgres_1 psql -U trachuan -d banhmi_vn < spikes/corpus/verify.sql`.
