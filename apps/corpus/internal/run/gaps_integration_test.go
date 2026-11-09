//go:build integration

package run

import (
	"context"
	"strings"

	"testing"

	"danny.vn/trachuan/apps/corpus/internal/stage"

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

func insertScopeFailure(t *testing.T, db *pgxpool.Pool, id int64) {
	t.Helper()
	if _, err := db.Exec(context.Background(), `INSERT INTO run_failure (run_id,failure_identity,code,stage,item_key,reason,evidence_refs) VALUES ($1,'scope','scope_changed','fetch','item','drift','[]')`, id); err != nil {
		t.Fatal(err)
	}
}

func TestAcceptGaps_RejectsScopeChanged(t *testing.T) {
	db, id := gapDB(t)
	insertScopeFailure(t, db, id)
	n, err := AcceptGaps(context.Background(), Deps{DB: db}, "vn", "20261109", "one", "accepted")
	if err == nil || !strings.Contains(err.Error(), "scope_changed") || n != 0 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	var count int
	if err := db.QueryRow(context.Background(), `SELECT count(*) FROM gap WHERE run_id=$1 AND accepted_at IS NOT NULL`, id).Scan(&count); err != nil || count != 0 {
		t.Fatalf("count=%d err=%v", count, err)
	}
}

func TestPublishable_RejectsRunFailure(t *testing.T) {
	db, id := gapDB(t)
	insertScopeFailure(t, db, id)
	ok, reasons, err := Publishable(context.Background(), db, id)
	if err != nil || ok || !strings.Contains(strings.Join(reasons, ","), "scope_changed") {
		t.Fatalf("ok=%v reasons=%v err=%v", ok, reasons, err)
	}
}

func TestStart_RejectsScopeChanged(t *testing.T) {
	db, id := gapDB(t)
	insertScopeFailure(t, db, id)
	d := Deps{DB: db, Stages: []string{"fetch"}, SourceList: []byte(`[]`), RecipesFor: func(stage.Snapshot) map[string]string { return map[string]string{"fetch": "recipe"} }}
	err := Start(context.Background(), d, "vn", "20261109", "", "")
	if err == nil || !strings.Contains(err.Error(), "scope_changed") {
		t.Fatalf("err=%v", err)
	}
}

func TestRecover_PreservesScopeFailure(t *testing.T) {
	db, id := gapDB(t)
	insertScopeFailure(t, db, id)
	d := Deps{DB: db, Stages: []string{"fetch"}, SourceList: []byte(`[]`), RecipesFor: func(stage.Snapshot) map[string]string { return map[string]string{"fetch": "recipe"} }}
	if err := Recover(context.Background(), d, "vn", "20261109", false); err != nil {
		t.Fatal(err)
	}
	if err := Start(context.Background(), d, "vn", "20261109", "", ""); err == nil || !strings.Contains(err.Error(), "scope_changed") {
		t.Fatalf("start after recovery: %v", err)
	}
}

func TestPublishable_ReportsPausedSource(t *testing.T) {
	db, id := gapDB(t)
	if _, err := db.Exec(context.Background(), `DELETE FROM gap WHERE run_id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(context.Background(), `INSERT INTO run_source(run_id,source,state,pause_reason) VALUES ($1,'vbpl','source_paused','challenge')`, id); err != nil {
		t.Fatal(err)
	}
	ok, reasons, err := Publishable(context.Background(), db, id)
	if err != nil || ok || !strings.Contains(strings.Join(reasons, ","), "paused source") {
		t.Fatalf("ok=%v reasons=%v err=%v", ok, reasons, err)
	}
}
