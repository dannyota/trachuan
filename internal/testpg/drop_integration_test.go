//go:build integration

package testpg

import (
	"context"
	"errors"
	"testing"
	"time"

	"danny.vn/trachuan/internal/pgdrop"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestDropDatabase_ClientSessions(t *testing.T) {
	ctx := context.Background()
	builder := NewBuilder(t)
	conn, err := pgx.Connect(ctx, builder.URL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close(ctx) })
	name := builder.Template + "_drop"
	if _, err = conn.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()+" TEMPLATE "+pgx.Identifier{builder.Template}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if dropErr := pgdrop.Drop(ctx, conn, name); dropErr != nil {
			var e *pgconn.PgError
			if !errors.As(dropErr, &e) || e.Code != "3D000" {
				t.Error(dropErr)
			}
		}
	})
	ownCfg := conn.Config().Copy()
	ownCfg.Database = name
	own, err := pgx.ConnectConfig(ctx, ownCfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = own.Close(ctx) }()
	observer, base, err := OpenAdministrator(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = observer.Close(ctx) }()
	base.Path = "/" + name
	foreign, err := pgx.Connect(ctx, base.String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = foreign.Close(ctx) }()
	started := time.Now()
	err = pgdrop.Drop(ctx, conn, name)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "55006" {
		t.Fatalf("expected 55006, got %v", err)
	}
	if elapsed := time.Since(started); elapsed < 9*time.Second || elapsed > 20*time.Second {
		t.Fatalf("drop wait %v", elapsed)
	}
	if err = own.Ping(ctx); err == nil {
		t.Fatal("own client survived drop attempt")
	}
	_ = conn.Close(ctx)
	reconnected, err := pgx.Connect(ctx, builder.URL)
	if err != nil {
		t.Fatal(err)
	}
	conn = reconnected
	cancelConn, err := pgx.Connect(ctx, builder.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cancelConn.Close(ctx) }()
	cancelCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	if err = pgdrop.Drop(cancelCtx, cancelConn, name); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cancelled drop: %v", err)
	}
	if err = foreign.Close(ctx); err != nil {
		t.Fatal(err)
	}
	own, err = pgx.ConnectConfig(ctx, ownCfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = own.Close(ctx) }()
	if err = pgdrop.Drop(ctx, conn, name); err != nil {
		t.Fatalf("drop with own client: %v", err)
	}
	if err = own.Ping(ctx); err == nil {
		t.Fatal("own client survived drop")
	}
}
