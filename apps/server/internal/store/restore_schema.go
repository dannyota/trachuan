package store

import (
	"context"
	"errors"
	"fmt"
	"io/fs"

	"danny.vn/trachuan/apps/server/migrations"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pressly/goose/v3"
)

const ErrAuthSchemaNotMigrated authorityFailure = "auth schema is not migrated; run migrate first"

func (q *Queries) requireCurrentAuthSchema(ctx context.Context) error {
	paths, err := fs.Glob(migrations.FS, "*.sql")
	if err != nil {
		return fmt.Errorf("read auth migration sources: %w", err)
	}
	var latest int64
	for _, path := range paths {
		version, parseErr := goose.NumericComponent(path)
		if parseErr != nil {
			return fmt.Errorf("read auth migration version: %w", parseErr)
		}
		latest = max(latest, version)
	}
	if latest == 0 {
		return errors.New("no auth migrations embedded")
	}
	var current int64
	err = q.db.QueryRow(ctx, `SELECT COALESCE(MAX(version_id),0) FROM goose_db_version WHERE is_applied`).Scan(&current)
	if err != nil {
		var missing *pgconn.PgError
		if errors.As(err, &missing) && missing.Code == "42P01" {
			return ErrAuthSchemaNotMigrated
		}
		return fmt.Errorf("read auth schema version: %w", err)
	}
	if current != latest {
		return ErrAuthSchemaNotMigrated
	}
	return nil
}
