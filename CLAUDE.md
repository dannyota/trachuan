@AGENTS.md

## Claude Code specifics

- The primary session runs as `manager` (`.claude/settings.json`); duties are in AGENTS.md "Agents".
- Agent dispatch and model routes are in `instructions/workflow.md`. The CLI fallback reads `.codex/agents/<role>.toml` from the worker's worktree and saves its final message to the report file.
- Codex CLI workers run offline unit checks in the sandbox: the launcher makes the Go build and lint caches writable, allows loopback listeners and sets `GOPROXY=off`; integration tests and proofs stay with the primary. Workers mount `.codex/` read-only; the primary edits role files.
- When a CLI worker stops with a question, answer in a short file and resume it instead of starting a fresh worker; copy the previous report first, because `-o` overwrites it. Put standing answers for small ambiguities in the brief.
- Write briefs with a quoted heredoc (`<<'EOF'`); point workers at exact files, not whole dependencies.
- Probe and fixture-recording scripts record what the source returns and stop only on real errors (transport failure, non-2xx), never on valid but unexpected shapes such as nullable fields or an HTTP 200 with an error envelope.
- When a reviewer keeps finding variants of one edge case, close the class with one fail-closed rule in the design instead of patching each variant.
- Low-risk slices (read-only paths, decode fixes, docs) get one review round, and low findings are fixed without a separate confirmation; writes, security and release paths keep the full process.
- Keep briefs, reports and temporary source files in the scratchpad, outside the checkout. Go tools can scan `.go` files under gitignored `work/`. Findings not fixed at once become lines in `docs/plans/backlog.md`.
- Capture a check's exit status before filtering its log; `make pre-push | tail -1` can hide a failure.
- Run `make pre-push` before every push and the checks the brief names from `instructions/testing.md` on this MacBook. Run long jobs with `run_in_background`.
- Discuss with the owner in Vietnamese; write every file in English.

## Gotchas

- The Bash tool runs zsh: `$var` does not word-split; `"$sha:refs/heads/main"` is a history modifier, write `"${sha}:refs/heads/main"`.
- GNU tools come first on PATH: `sed -i`, not `sed -i ''`. Use `/usr/bin/grep` when `grep` is aliased to ugrep and a pattern fails with a complexity error.
- `gh` holds several accounts; for repository writes use `GH_TOKEN=$(gh auth token -u dannyota)`.
