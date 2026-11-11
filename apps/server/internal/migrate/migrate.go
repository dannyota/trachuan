// Package migrate applies auth database migrations.
package migrate

import (
	"context"
	"fmt"

	"danny.vn/trachuan/apps/server/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// Up applies all pending auth migrations.
func Up(ctx context.Context, url string) error {
	config, err := pgx.ParseConfig(url)
	if err != nil {
		return fmt.Errorf("parse auth database URL: %w", err)
	}
	db := stdlib.OpenDB(*config)
	defer func() { _ = db.Close() }()
	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations.FS)
	if err != nil {
		return fmt.Errorf("prepare auth migrations: %w", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("apply auth migrations: %w", err)
	}
	return nil
}
