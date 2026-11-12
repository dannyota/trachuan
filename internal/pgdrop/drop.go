// Package pgdrop drops databases without terminating foreign sessions.
// A DROP in flight is not interrupted by caller cancellation and may take up to 60 seconds after cancellation.
// If the last-resort statement timeout interrupts a DROP, PostgreSQL may leave the database invalid; the error advises dropping it again.
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
	return dropOnlyBounded(ctx, bounded, conn, name, wait)
}

func dropOnlyBounded(ctx, bounded context.Context, conn executor, name string, wait func(context.Context) error) error {
	var busy error
	err, between := retry(bounded, func() error {
		statement, stop := context.WithTimeout(context.WithoutCancel(ctx), 60*time.Second)
		defer stop()
		_, err := conn.Exec(statement, "DROP DATABASE "+pgx.Identifier{name}.Sanitize())
		if err != nil && (errors.Is(statement.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded)) {
			return fmt.Errorf("database %q may be invalid; drop it again: %w", name, err)
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "55006" {
			busy = err
		}
		return err
	}, wait)
	if err != nil {
		if between && ctx.Err() != nil {
			err = ctx.Err()
		} else if between && bounded.Err() != nil && busy != nil {
			err = busy
		}
		return fmt.Errorf("drop database: %w", err)
	}
	return nil
}

func retry(ctx context.Context, operation func() error, wait func(context.Context) error) (error, bool) {
	for {
		if err := ctx.Err(); err != nil {
			return err, true
		}
		err := operation()
		if err == nil {
			return nil, false
		}
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "55006" {
			return err, false
		}
		if waitErr := wait(ctx); waitErr != nil {
			return waitErr, true
		}
	}
}
