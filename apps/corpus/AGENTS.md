# Corpus

`cmd/corpus/` wires the CLI. `internal/cli/` dispatches commands. `internal/config/` loads configuration.

The backend lane owns `apps/corpus/`, `apps/server/`, `packages/schema/` and migrations; stay within the paths named by the brief.

Checks: `make test`, `make test-integration`, `make test-proof`, `make pre-push`.
