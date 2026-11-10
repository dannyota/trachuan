//go:build integration

package expand_test

import (
	"context"
	"encoding/json"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/fetch"
	"danny.vn/trachuan/apps/corpus/internal/stage/membership"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"danny.vn/trachuan/apps/corpus/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPredecessor_FetchDoesNotReselect(t *testing.T) {
	ctx := context.Background()
	db, err := pgxpool.New(ctx, testdb.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	data, err := json.Marshal(source.List{Country: "vn", VBPL: source.VBPLSources{SelectionVersion: 1}})
	if err != nil {
		t.Fatal(err)
	}
	r := stage.Run{Country: "vn", Rel: "20990103", Snapshot: stage.Snapshot{SourceList: data, Stages: []string{"discover", "expand", "fetch"}}}
	snap, err := json.Marshal(r.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	insert := func(rel string) int64 {
		var id int64
		if e := db.QueryRow(ctx, `INSERT INTO run(country,rel,state,snapshot) VALUES ('vn',$1,'paused',$2) RETURNING id`, rel, snap).Scan(&id); e != nil {
			t.Fatal(e)
		}
		return id
	}
	old := insert("20990101")
	if err = membership.PublishPhase(ctx, store.New(db), old, membership.Final, nil); err != nil {
		t.Fatal(err)
	}
	previous, err := membership.ReadPhase(ctx, store.New(db), old, membership.Final)
	if err != nil {
		t.Fatal(err)
	}
	r.ID = insert(r.Rel)
	if err = membership.PublishPhase(ctx, store.New(db), r.ID, membership.Expanded, nil); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `UPDATE run_scope SET predecessor_run_id=$2,predecessor_phase='final',predecessor_snapshot_hash=$3 WHERE run_id=$1`, r.ID, old, previous.Hash); err != nil {
		t.Fatal(err)
	}
	newer := insert("20990102")
	if err = membership.PublishPhase(ctx, store.New(db), newer, membership.Final, nil); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = fetch.New(nil).Finalize(ctx, stage.Env{}, tx, r); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	scope, err := store.New(db).GetRunScope(ctx, r.ID)
	if err != nil || scope.PredecessorRunID == nil || *scope.PredecessorRunID != old {
		t.Fatalf("scope=%+v err=%v", scope, err)
	}
}
