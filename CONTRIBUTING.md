# Contributing

Read [AGENTS.md](AGENTS.md) for how work is organized and
[docs/standards/engineering.md](docs/standards/engineering.md) for writing rules.

- Install hooks once: `make hooks-install` (gitleaks and file-length checks on
  every commit).
- Before pushing: `make pre-push`.
- One change per pull request; draft while iterating, ready when complete.
- Never commit secrets, personal data or user questions.
