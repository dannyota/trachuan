package pgdrop

import (
	"context"
	"errors"
	"strings"
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

type cancelDuringDrop struct {
	cancel       context.CancelFunc
	statementErr error
}

func (f *cancelDuringDrop) Exec(ctx context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
	f.cancel()
	f.statementErr = ctx.Err()
	return pgconn.CommandTag{}, &pgconn.PgError{Code: "55006"}
}

func TestDropOnly_CallerCancellationDoesNotCancelStatement(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	conn := &cancelDuringDrop{cancel: cancel}
	err := DropOnly(ctx, conn, "fixture")
	if conn.statementErr != nil {
		t.Errorf("drop statement context cancelled: %v", conn.statementErr)
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v", err)
	}
}

type invalidDatabase struct {
	statements []string
}

func (f *invalidDatabase) Exec(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
	f.statements = append(f.statements, sql)
	return pgconn.NewCommandTag("OK"), nil
}

func TestDrop_InvalidDatabaseUsesNormalDrop(t *testing.T) {
	conn := &invalidDatabase{}
	if err := Drop(context.Background(), conn, "invalid"); err != nil {
		t.Fatal(err)
	}
	if len(conn.statements) != 2 || !strings.Contains(conn.statements[0], "pg_terminate_backend") || conn.statements[1] != `DROP DATABASE "invalid"` {
		t.Fatalf("statements = %v", conn.statements)
	}
}

func TestDropOnly_InvalidDatabaseErrorReturnsImmediately(t *testing.T) {
	conn := &errorExecutor{err: &pgconn.PgError{Code: "55000", Message: "invalid database"}}
	err := DropOnly(context.Background(), conn, "invalid")
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "55000" || conn.calls != 1 {
		t.Fatalf("calls=%d error=%v", conn.calls, err)
	}
}

type errorExecutor struct {
	err   error
	calls int
}

func (f *errorExecutor) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	f.calls++
	return pgconn.CommandTag{}, f.err
}
