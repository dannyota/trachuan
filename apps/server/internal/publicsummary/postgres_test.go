package publicsummary

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgres_EmptyPoolReturnsBusyWithoutConnecting(t *testing.T) {
	cfg, err := pgxpool.ParseConfig("postgres://synthetic:synthetic@127.0.0.1:1/synthetic")
	if err != nil {
		t.Fatal(err)
	}
	cfg.MinConns, cfg.MaxConns = 0, 1
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p := &postgresPool{pool: pool}
	if _, err = p.Row(ctx, "vn:vbpl:1"); !errors.Is(err, ErrBusy) {
		t.Fatalf("empty row pool did not reject acquisition: %v", err)
	}
	if _, err = p.List(ctx); !errors.Is(err, ErrBusy) {
		t.Fatalf("empty list pool did not reject acquisition: %v", err)
	}
	if pool.Stat().TotalConns() != 0 {
		t.Fatal("empty pool started a connection")
	}
}
