# Go and SQL

Read for `apps/server`, `apps/corpus`, migrations and SQL.

- Google Go style (https://google.github.io/styleguide/go/) with `gofmt` and `goimports`. Doc comment on every exported symbol. Wrap errors with `%w` and context. `context.Context` is the first parameter. No package-level mutable state. Define small interfaces at the consumer.
- One concern per file, at most 400 lines, named after the concern. `cmd/*/main.go` only wires dependencies; construction lives in `internal/` packages.
- Table-driven tests beside the code: `<subject>[_<scenario>]_test.go`, `TestSubject_Behavior`. Integration tests that need PostgreSQL use the `integration` build tag and `-count=1`.
- Migrations: goose SQL under `apps/server/migrations/` for the auth database; the corpus schema is a template under `apps/corpus/schema/` applied by `corpus build`. sqlc generates `internal/store`; never hand-edit generated files.
- golangci-lint with errcheck, shadow and misspell (US spelling). `//nolint:` needs a reason.
- The server never writes to a corpus database. The pipeline never touches the auth database. Enforce with separate PostgreSQL roles.
- Vectors: dimensions come from the release recipe, never from a constant in the server.
