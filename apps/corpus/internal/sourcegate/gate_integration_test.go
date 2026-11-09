//go:build integration

package sourcegate

import (
	"context"
	"errors"
	"sync"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

func gateDB(t *testing.T) (*pgxpool.Pool, int64) {
	t.Helper()
	ctx := context.Background()
	db, err := pgxpool.New(ctx, testdb.New(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	var id int64
	if err := db.QueryRow(ctx, `INSERT INTO run (country,rel,state,snapshot) VALUES ('vn','20990109','running','{}') RETURNING id`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return db, id
}

func TestSourcePause_PersistsAcrossWorkers(t *testing.T) {
	ctx := context.Background()
	db, id := gateDB(t)
	first, second := New(db), New(db)
	if err := first.Admit(ctx, id, "vbpl"); err != nil {
		t.Fatal(err)
	}
	if err := first.Pause(ctx, id, "vbpl", "challenge", "path hash"); err != nil {
		t.Fatal(err)
	}
	for _, gate := range []Gate{second, New(db)} {
		if err := gate.Admit(ctx, id, "vbpl"); !errors.Is(err, ErrSourcePaused) {
			t.Fatalf("admit = %v", err)
		}
	}
}

func TestSourceGate_PauseIsIdempotentWithOneEvent(t *testing.T) {
	ctx := context.Background()
	db, id := gateDB(t)
	gate := New(db)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- gate.Pause(ctx, id, "vbpl", "challenge", "path hash") }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var n int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM run_source_event WHERE run_id=$1 AND event='pause'`, id).Scan(&n); err != nil || n != 1 {
		t.Fatalf("events=%d err=%v", n, err)
	}
}

func TestSourceGate_ClearNeedsExplicitCall(t *testing.T) {
	ctx := context.Background()
	db, id := gateDB(t)
	gate := New(db)
	if err := gate.Pause(ctx, id, "vbpl", "challenge", "path hash"); err != nil {
		t.Fatal(err)
	}
	if err := New(db).Admit(ctx, id, "vbpl"); !errors.Is(err, ErrSourcePaused) {
		t.Fatalf("admit=%v", err)
	}
	if err := gate.Clear(ctx, id, "vbpl", "owner note"); err != nil {
		t.Fatal(err)
	}
	if err := gate.Admit(ctx, id, "vbpl"); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM run_source_event WHERE run_id=$1 AND event='clear' AND clear_note='owner note'`, id).Scan(&n); err != nil || n != 1 {
		t.Fatalf("events=%d err=%v", n, err)
	}
}
