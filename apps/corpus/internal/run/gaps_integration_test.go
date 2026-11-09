//go:build integration

package run

import (
	"context"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

func gapDB(t *testing.T) (*pgxpool.Pool, int64) {
	t.Helper()
	ctx := context.Background()
	url := testdb.New(t)
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	var id int64
	if err = db.QueryRow(ctx, `INSERT INTO run (country, rel, state, snapshot) VALUES ('vn', '20261109', 'paused', '{}') RETURNING id`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `INSERT INTO run_item (run_id, stage, item_key, state, batch) VALUES ($1, 'fetch', 'item', 'done', 0)`, id); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `INSERT INTO gap (run_id, stage, item_key, kind, detail) VALUES ($1, 'fetch', 'item', 'one', ''), ($1, 'fetch', 'item', 'two', '')`, id); err != nil {
		t.Fatal(err)
	}
	return db, id
}

func TestAcceptGaps_AcceptsOnlyThatKind(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db, id := gapDB(t)
	n, err := AcceptGaps(ctx, Deps{DB: db}, "vn", "20261109", "one", "accepted")
	if err != nil || n != 1 {
		t.Fatalf("accept = %d, %v", n, err)
	}
	var accepted, unaccepted int
	err = db.QueryRow(ctx, `SELECT count(*) FILTER (WHERE kind = 'one' AND accepted_at IS NOT NULL AND note = 'accepted'), count(*) FILTER (WHERE kind = 'two' AND accepted_at IS NULL) FROM gap WHERE run_id = $1`, id).Scan(&accepted, &unaccepted)
	if err != nil || accepted != 1 || unaccepted != 1 {
		t.Fatalf("gap acceptance: accepted=%d unaccepted=%d err=%v", accepted, unaccepted, err)
	}
	ok, _, err := Publishable(ctx, db, id)
	if err != nil || ok {
		t.Fatalf("publishable = %v, %v", ok, err)
	}
}

func TestPublishable_DoneItemWithUnacceptedGap(t *testing.T) {
	t.Parallel()
	db, id := gapDB(t)
	ok, reasons, err := Publishable(context.Background(), db, id)
	if err != nil || ok || len(reasons) == 0 {
		t.Fatalf("publishable = %v, %v, %v", ok, reasons, err)
	}
}
