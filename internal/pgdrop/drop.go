// Package pgdrop drops databases without terminating foreign sessions.
// Cancelling a DROP DATABASE statement can leave the database invalid, so statements use an independent timeout.
package pgdrop

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type executor interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

// Drop terminates the current role's client sessions and drops the database.
func Drop(ctx context.Context, conn executor, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := conn.Exec(ctx, `SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname=$1 AND usesysid IS NOT NULL AND usename=current_user AND pid<>pg_backend_pid()`, name); err != nil {
		return fmt.Errorf("disconnect database sessions: %w", err)
	}
	return DropOnly(ctx, conn, name)
}

// DropOnly drops the database without terminating client sessions.
func DropOnly(ctx context.Context, conn executor, name string) error {
	return dropOnly(ctx, conn, name, waitRetry)
}

func waitRetry(ctx context.Context) error {
	timer := time.NewTimer(100 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func dropOnly(ctx context.Context, conn executor, name string, wait func(context.Context) error) error {
	bounded, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var busy error
	err := retry(bounded, func() error {
		statement, stop := context.WithTimeout(context.WithoutCancel(ctx), 60*time.Second)
		defer stop()
		_, err := conn.Exec(statement, "DROP DATABASE "+pgx.Identifier{name}.Sanitize())
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "55006" {
			busy = err
		}
		return err
	}, wait)
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
