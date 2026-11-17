//go:build integration

package summary

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	"danny.vn/trachuan/apps/corpus/internal/stage/build"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5"
)

func Load(ctx context.Context, database string) error {
	if !regexp.MustCompile(`^corpus_vn_2099[0-9]{4}(_[0-9]+)?$`).MatchString(database) {
		return errors.New("invalid fixture database name")
	}
	admin, base, err := testpg.OpenAdministrator(ctx)
	if err != nil {
		return fmt.Errorf("open administrator: %w", err)
	}
	defer func() { _ = admin.Close(context.Background()) }()
	base.Path = "/" + database
	conn, err := pgx.Connect(ctx, base.String())
	if err != nil {
		return errors.New("connect to fixture database")
	}
	defer func() { _ = conn.Close(context.Background()) }()
	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin summary transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, _, err = build.LoadSummaries(ctx, tx, nil, nil); err != nil {
		return fmt.Errorf("load summaries: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit summaries: %w", err)
	}
	return nil
}
