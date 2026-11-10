@AGENTS.md

## Claude Code specifics

- The primary session runs as `manager` (`.claude/settings.json`); duties are in AGENTS.md "Agents".
- Dispatch Codex: `scripts/codex-agent.sh [--model <m>] [--effort <e>] [--search] [--resume <session-id>] <role> <worktree> <brief-file> <report-file>`. The script reads model, effort, sandbox and developer instructions from `<worktree>/.codex/agents/<role>.toml`, redirects stdin from `/dev/null`, and saves the worker's final message to `<report-file>`.
- Codex workers run offline unit checks in the sandbox: the launcher makes the Go build and lint caches writable, allows loopback listeners and sets `GOPROXY=off`; integration tests and proofs stay with the primary. Workers mount `.codex/` read-only; the primary edits role files.
- When a worker stops with a question, answer in a short file and continue it with `--resume <session id from its log>` instead of a fresh run; copy the report first, because `-o` overwrites it. Put standing answers for small ambiguities in the brief.
- Write briefs with a quoted heredoc (`<<'EOF'`); point workers at exact files, not whole dependencies.
- Probe and fixture-recording scripts record what the source returns and stop only on real errors (transport failure, non-2xx), never on valid but unexpected shapes such as nullable fields or an HTTP 200 with an error envelope.
- When a reviewer keeps finding variants of one edge case, close the class with one fail-closed rule in the design instead of patching each variant.
- Low-risk slices (read-only paths, decode fixes, docs) get one review round, and low findings are fixed without a separate confirmation; writes, security and release paths keep the full process.
- Keep briefs, reports and temporary files in the scratchpad, outside the repository. Findings not fixed at once become lines in `docs/plans/backlog.md`.
- Local checks are allowed on this MacBook: `make pre-push` before every push; `make test` and `make eval` as the brief requires. Run long jobs with `run_in_background`.
- Discuss with the owner in Vietnamese; write every file in English.

## Gotchas

- The Bash tool runs zsh: `$var` does not word-split; `"$sha:refs/heads/main"` is a history modifier, write `"${sha}:refs/heads/main"`.
- GNU tools come first on PATH: `sed -i`, not `sed -i ''`. Use `/usr/bin/grep` when `grep` is aliased to ugrep and a pattern fails with a complexity error.
- `gh` holds several accounts; for repository writes use `GH_TOKEN=$(gh auth token -u dannyota)`.
