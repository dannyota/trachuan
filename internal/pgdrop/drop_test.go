package pgdrop

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestRetry_BusyThenSucceeds(t *testing.T) {
	attempts := 0
	err := retry(context.Background(), func() error {
		attempts++
		if attempts < 3 {
			return &pgconn.PgError{Code: "55006"}
		}
		return nil
	}, func(context.Context) error { return nil })
	if err != nil || attempts != 3 {
		t.Fatalf("attempts=%d error=%v", attempts, err)
	}
}

func TestRetry_OtherErrorReturnsImmediately(t *testing.T) {
	failure := errors.New("failure")
	attempts := 0
	err := retry(context.Background(), func() error { attempts++; return failure }, func(context.Context) error { t.Fatal("waited after other error"); return nil })
	if !errors.Is(err, failure) || attempts != 1 {
		t.Fatalf("attempts=%d error=%v", attempts, err)
	}
}

func TestRetry_Cancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	err := retry(ctx, func() error { cancel(); return &pgconn.PgError{Code: "55006"} }, func(ctx context.Context) error { return ctx.Err() })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v", err)
	}
}

type fakeExecutor struct {
	statements []string
}

func (f *fakeExecutor) Exec(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
	f.statements = append(f.statements, sql)
	if sql != `DROP DATABASE "fixture"` {
		return pgconn.CommandTag{}, errors.New("unexpected statement")
	}
	if len(f.statements) < 3 {
		return pgconn.CommandTag{}, &pgconn.PgError{Code: "55006"}
	}
	return pgconn.NewCommandTag("DROP DATABASE"), nil
}

func TestDropOnly_RetriesWithoutTerminating(t *testing.T) {
	conn := &fakeExecutor{}
	if err := dropOnly(context.Background(), conn, "fixture", func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if len(conn.statements) != 3 {
		t.Fatalf("statements: %v", conn.statements)
	}
}
