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
