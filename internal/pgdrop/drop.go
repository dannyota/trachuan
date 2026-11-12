// Package pgdrop drops databases without terminating foreign sessions.
package pgdrop

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Drop terminates the current role's client sessions and drops the database.
func Drop(ctx context.Context, conn *pgx.Conn, name string) error {
	if _, err := conn.Exec(ctx, `SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname=$1 AND usesysid IS NOT NULL AND usename=current_user AND pid<>pg_backend_pid()`, name); err != nil {
		return fmt.Errorf("disconnect database sessions: %w", err)
	}
	bounded, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var busy error
	err := retry(bounded, func() error {
		_, err := conn.Exec(bounded, "DROP DATABASE "+pgx.Identifier{name}.Sanitize())
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "55006" {
			busy = err
		}
		return err
	}, func(ctx context.Context) error {
		timer := time.NewTimer(100 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return nil
		}
	})
	if err != nil {
		if ctx.Err() != nil {
			err = ctx.Err()
		} else if bounded.Err() != nil && busy != nil {
			err = busy
		}
		return fmt.Errorf("drop database: %w", err)
	}
	return nil
}

func retry(ctx context.Context, operation func() error, wait func(context.Context) error) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := operation()
		if err == nil {
			return nil
		}
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "55006" {
			return err
		}
		if waitErr := wait(ctx); waitErr != nil {
			return waitErr
		}
	}
}
