//go:build integration

package build

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestTemporaryDatabase_ClientSessions(t *testing.T) {
	ctx := context.Background()
	builder := testpg.NewBuilder(t)
	admin, err := pgx.Connect(ctx, builder.URL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close(ctx) })
	rel := testpg.Release(t)
	sum := sha256.Sum256([]byte(rel))
	temp := fmt.Sprintf("corpus_tmp_vn_%x", sum[:12])
	final := "corpus_vn_" + rel
	marker := testOwnershipMarker(t, "a", "built")
	cfg := Config{Template: builder.Template}
	if err = recreateTemporary(ctx, admin, cfg, temp, marker); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		testpg.DropCorpusDatabase(t, admin, temp, "vn/"+rel)
		testpg.DropCorpusDatabase(t, admin, final, "vn/"+rel)
	})
	waitClosed := func(pid uint32) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for {
			var active bool
			if queryErr := admin.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE pid=$1)`, pid).Scan(&active); queryErr != nil {
				t.Fatal(queryErr)
			}
			if !active {
				return
			}
			if time.Now().After(deadline) {
				t.Fatal("client did not disconnect")
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	ownCfg := admin.Config().Copy()
	ownCfg.Database = temp
	own, err := pgx.ConnectConfig(ctx, ownCfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func(c *pgx.Conn) { _ = c.Close(ctx) }(own)
	const short = 200 * time.Millisecond
	if err = recreateTemporaryWithin(ctx, admin, cfg, temp, marker, short); err == nil || !strings.Contains(err.Error(), "active sessions") {
		t.Fatalf("same-role client was not refused: %v", err)
	}
	if err = own.Close(ctx); err != nil {
		t.Fatal(err)
	}
	waitClosed(own.PgConn().PID())
	closing, err := pgx.ConnectConfig(ctx, ownCfg)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(300 * time.Millisecond)
		_ = closing.Close(context.Background())
	}()
	if err = recreateTemporary(ctx, admin, cfg, temp, marker); err != nil {
		t.Fatalf("session closing during the grace was not awaited: %v", err)
	}
	observer, base, err := testpg.OpenAdministrator(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = observer.Close(ctx) }()
	base.Path = "/" + temp
	foreign, err := pgx.Connect(ctx, base.String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = foreign.Close(ctx) }()
	if err = recreateTemporaryWithin(ctx, admin, cfg, temp, marker, short); err == nil || !strings.Contains(err.Error(), "active sessions") {
		t.Fatalf("foreign client was not refused: %v", err)
	}
	own, err = pgx.ConnectConfig(ctx, ownCfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func(c *pgx.Conn) { _ = c.Close(ctx) }(own)
	if err = renameToFinal(ctx, admin, temp, final); err == nil || !strings.Contains(err.Error(), "foreign sessions") {
		t.Fatalf("foreign client was not refused on rename: %v", err)
	}
	if err = foreign.Close(ctx); err != nil {
		t.Fatal(err)
	}
	waitClosed(foreign.PgConn().PID())
	if err = renameToFinal(ctx, admin, temp, final); err != nil {
		t.Fatalf("rename with own client: %v", err)
	}
	if err = own.Ping(ctx); err == nil {
		t.Fatal("own client survived rename")
	}
}

type connectAfterCheck struct {
	cfg     *pgx.ConnConfig
	waiting bool
	client  *pgx.Conn
	err     error
}

func (tracer *connectAfterCheck) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	tracer.waiting = strings.Contains(data.SQL, "FROM pg_stat_activity WHERE datname=$1 AND usesysid IS NOT NULL") && tracer.client == nil
	return ctx
}

func (tracer *connectAfterCheck) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	if tracer.waiting && data.Err == nil {
		tracer.waiting = false
		tracer.client, tracer.err = pgx.ConnectConfig(ctx, tracer.cfg)
	}
}

func TestRecoveryDrop_ClientConnectsAfterCheck(t *testing.T) {
	ctx := context.Background()
	builder := testpg.NewBuilder(t)
	setup, err := pgx.Connect(ctx, builder.URL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = setup.Close(ctx) })
	rel := testpg.Release(t)
	sum := sha256.Sum256([]byte(rel))
	name := fmt.Sprintf("corpus_tmp_vn_%x", sum[:12])
	marker := testOwnershipMarker(t, "a", "built")
	cfg := Config{Template: builder.Template}
	if err = recreateTemporary(ctx, setup, cfg, name, marker); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		testpg.DropCorpusDatabase(t, setup, name, "vn/"+rel)
	})
	clientCfg := setup.Config().Copy()
	clientCfg.Database = name
	tracer := &connectAfterCheck{cfg: clientCfg}
	adminCfg := setup.Config().Copy()
	adminCfg.Tracer = tracer
	admin, err := pgx.ConnectConfig(ctx, adminCfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = admin.Close(ctx) }()
	start := time.Now()
	err = recreateTemporary(ctx, admin, cfg, name, marker)
	if tracer.err != nil || tracer.client == nil {
		t.Fatalf("connect after check: %v", tracer.err)
	}
	defer func() { _ = tracer.client.Close(ctx) }()
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "55006" {
		t.Fatalf("expected 55006: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 9*time.Second || elapsed > 20*time.Second {
		t.Fatalf("drop wait %v", elapsed)
	}
	if err = tracer.client.Ping(ctx); err != nil {
		t.Fatalf("client disconnected: %v", err)
	}
}
