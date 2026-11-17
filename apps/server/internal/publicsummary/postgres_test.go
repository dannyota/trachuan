package publicsummary

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestPostgres_EmptyPoolReturnsBusyWithoutConnecting(t *testing.T) {
	var err error
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p := &postgresPool{}
	if _, err = p.Row(ctx, "vn:vbpl:1"); !errors.Is(err, ErrBusy) {
		t.Fatalf("empty row pool did not reject acquisition: %v", err)
	}
	if _, err = p.List(ctx); !errors.Is(err, ErrBusy) {
		t.Fatalf("empty list pool did not reject acquisition: %v", err)
	}
	if p.conn != nil {
		t.Fatal("empty pool started a connection")
	}
}

func TestPostgres_ConfigCapsConnectionAttempts(t *testing.T) {
	for _, query := range []string{"", "?connect_timeout=60", "?pool_min_conns=9&pool_max_conns=12"} {
		cfg, err := postgresConfig("postgres://synthetic:synthetic@127.0.0.1:1/{database}"+query, "corpus_vn_20990101")
		if err != nil {
			t.Fatal(err)
		}
		if cfg.ConnectTimeout <= 0 || cfg.ConnectTimeout > 2*time.Second {
			t.Fatalf("unbounded handshake for %q: %v", query, cfg.ConnectTimeout)
		}
		if _, present := cfg.RuntimeParams["pool_min_conns"]; present {
			t.Fatal("pool options leaked into PostgreSQL startup parameters")
		}
		if cfg.RuntimeParams["statement_timeout"] != "2000" || cfg.RuntimeParams["default_transaction_read_only"] != "on" {
			t.Fatal("public connection limits missing")
		}
	}
}
