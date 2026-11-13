# Go and SQL

Read for `apps/server`, `apps/corpus`, migrations and SQL.

- Google Go style (https://google.github.io/styleguide/go/) with `gofmt` and `goimports`. No comments except tool directives and a one-line why for an external tool or service behaviour no design doc owns (`docs/standards/engineering.md`); no doc comments that only repeat a name. Wrap errors with `%w` and context. `context.Context` is the first parameter. No package-level mutable state. Define small interfaces at the consumer.
- One concern per file, at most 400 lines, named after the concern. `cmd/*/main.go` only wires dependencies; construction lives in `internal/` packages.
- Table-driven tests beside the code: `<subject>[_<scenario>]_test.go`, `TestSubject_Behavior`. Integration tests that need PostgreSQL use the `integration` build tag and `-count=1`.
- Migrations: goose SQL under `apps/server/migrations/` for the auth database; workshop goose migrations under `apps/corpus/migrations/`, applied by `corpus migrate`; the corpus schema is a template in the root package `internal/corpusdb`, applied by the `build` stage and shared with `internal/retrieval` tests. sqlc generates each app's `internal/store`, including `apps/corpus/internal/store`; never hand-edit generated files.
- golangci-lint with errcheck, shadow and misspell (US spelling). `//nolint:` needs a reason.
- The server never writes to a corpus database. The pipeline never touches the auth database. Enforce with separate PostgreSQL roles.
- Vectors: dimensions come from the release recipe, never from a constant in the server.
