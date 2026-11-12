//go:build integration

package testpg

import (
	"context"
	"errors"
	"strings"
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
	defer func(c *pgx.Conn) { _ = c.Close(ctx) }(own)
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
	assertConnectable := func() {
		t.Helper()
		var exists bool
		if queryErr := conn.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname=$1)`, name).Scan(&exists); queryErr != nil || !exists {
			t.Fatalf("database exists=%v error=%v", exists, queryErr)
		}
		probe, connectErr := pgx.ConnectConfig(ctx, ownCfg)
		if connectErr != nil {
			t.Fatalf("connect to database: %v", connectErr)
		}
		if closeErr := probe.Close(ctx); closeErr != nil {
			t.Fatal(closeErr)
		}
	}
	alreadyCancelled, cancelAlready := context.WithCancel(ctx)
	cancelAlready()
	if err = pgdrop.DropOnly(alreadyCancelled, conn, name); !errors.Is(err, context.Canceled) {
		t.Fatalf("already cancelled drop: %v", err)
	}
	assertConnectable()
	tracer := &dropWaitTracer{busy: make(chan struct{}, 1)}
	cancelCfg := conn.Config().Copy()
	cancelCfg.Tracer = tracer
	cancelConn, err := pgx.ConnectConfig(ctx, cancelCfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cancelConn.Close(ctx) }()
	cancelCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	cancelledAt := make(chan time.Time, 1)
	go func() {
		select {
		case <-tracer.busy:
			timer := time.NewTimer(10 * time.Millisecond)
			defer timer.Stop()
			select {
			case <-timer.C:
				cancel()
				cancelledAt <- time.Now()
			case <-cancelCtx.Done():
			}
		case <-cancelCtx.Done():
		}
	}()
	if err = pgdrop.DropOnly(cancelCtx, cancelConn, name); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled drop: %v", err)
	}
	if elapsed := time.Since(<-cancelledAt); elapsed > time.Second {
		t.Fatalf("cancellation took %v", elapsed)
	}
	assertConnectable()
	if err = foreign.Close(ctx); err != nil {
		t.Fatal(err)
	}
	own, err = pgx.ConnectConfig(ctx, ownCfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func(c *pgx.Conn) { _ = c.Close(ctx) }(own)
	if err = pgdrop.Drop(ctx, conn, name); err != nil {
		t.Fatalf("drop with own client: %v", err)
	}
	if err = own.Ping(ctx); err == nil {
		t.Fatal("own client survived drop")
	}
}

type dropWaitTracer struct {
	busy chan struct{}
	drop bool
}

func (tracer *dropWaitTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	tracer.drop = strings.HasPrefix(data.SQL, "DROP DATABASE ")
	return ctx
}

func (tracer *dropWaitTracer) TraceQueryEnd(_ context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	var pgErr *pgconn.PgError
	if tracer.drop && errors.As(data.Err, &pgErr) && pgErr.Code == "55006" {
		select {
		case tracer.busy <- struct{}{}:
		default:
		}
	}
}
