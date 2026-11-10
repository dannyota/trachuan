//go:build integration

package extract

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/membership"
	"danny.vn/trachuan/apps/corpus/internal/stage/relate"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"danny.vn/trachuan/apps/corpus/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestExtract_ChinhphuNoTree(t *testing.T) {
	ctx := context.Background()
	db, err := pgxpool.New(ctx, testdb.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	artifacts := artifact.Store{Root: t.TempDir()}
	art, err := artifacts.PublishContent(ctx, "fetch", strings.Repeat("b", 64), func(dir string) error {
		return os.WriteFile(filepath.Join(dir, "document.json"), []byte(`{"version":2}`), 0600)
	})
	if err != nil {
		t.Fatal(err)
	}
	var runID, artifactID int64
	err = db.QueryRow(ctx, `INSERT INTO run (country,rel,state,snapshot) VALUES ('vn','20990101','running','{}') RETURNING id`).Scan(&runID)
	if err != nil {
		t.Fatal(err)
	}
	err = db.QueryRow(ctx, `INSERT INTO artifact (stage,item_key,input_hash,recipe_hash,path,sha256) VALUES ('fetch','vn:chinhphu:1',$1,$2,$3,$4) RETURNING id`, art.InputHash, strings.Repeat("b", 64), art.Path, art.SHA256).Scan(&artifactID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(ctx, `INSERT INTO run_item (run_id,stage,item_key,state,batch,artifact_id) VALUES ($1,'fetch','vn:chinhphu:1','done',0,$2)`, runID, artifactID)
	if err != nil {
		t.Fatal(err)
	}
	out, err := New().Process(ctx, stage.Env{DB: db, Artifacts: artifacts}, stage.Run{ID: runID}, stage.Item{Key: "vn:chinhphu:1", InputHash: art.SHA256})
	if err != nil {
		t.Fatal(err)
	}
	if out.Artifact == nil {
		t.Fatalf("output %+v", out)
	}
	resolution, err := New().Resolve(ctx, stage.Env{Artifacts: artifacts}, stage.Run{}, stage.Item{}, *out.Artifact)
	if err != nil || len(resolution.Gaps) != 1 || resolution.Gaps[0].Kind != "no_tree" {
		t.Fatalf("resolution %+v: %v", resolution, err)
	}
	rows, err := os.ReadFile(filepath.Join(artifacts.Abs(out.Artifact.Path), "provisions.json"))
	if err != nil || string(rows) != "[]" {
		t.Fatalf("provisions %s: %v", rows, err)
	}
}

func TestPlan_FinalMembershipFiltersFetchedItems(t *testing.T) {
	ctx := context.Background()
	db, err := pgxpool.New(ctx, testdb.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var id int64
	if err = db.QueryRow(ctx, `INSERT INTO run (country,rel,state,snapshot) VALUES ('vn','20990102','running','{}') RETURNING id`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"vn:vbpl:1", "vn:chinhphu:2"} {
		var art int64
		if err = db.QueryRow(ctx, `INSERT INTO artifact (stage,item_key,input_hash,recipe_hash,path,sha256) VALUES ('fetch',$1,$1,$1,$1,$1) RETURNING id`, key).Scan(&art); err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(ctx, `INSERT INTO run_item (run_id,stage,item_key,state,batch,artifact_id) VALUES ($1,'fetch',$2,'done',0,$3)`, id, key, art); err != nil {
			t.Fatal(err)
		}
	}
	entries := []membership.Entry{{DocumentKey: "vn:vbpl:1", Source: "vbpl", SourceID: "1", Class: "primary", Disposition: "active"}, {DocumentKey: "vn:chinhphu:2", Source: "chinhphu", SourceID: "2", Class: "secondary", Disposition: "source_replaced", ReplacementKey: ptr("vn:vbpl:1")}}
	if err = membership.PublishPhase(ctx, store.New(db), id, membership.Final, entries); err != nil {
		t.Fatal(err)
	}
	run := stage.Run{ID: id, Snapshot: stage.Snapshot{SourceList: []byte(`{"vbpl":{"selection_version":1}}`)}}
	items, err := New().Plan(ctx, stage.Env{DB: db}, run)
	if err != nil || len(items) != 1 || items[0].Key != "vn:vbpl:1" {
		t.Fatalf("items=%+v err=%v", items, err)
	}
	related, err := relate.New().Plan(ctx, stage.Env{DB: db}, run)
	if err != nil || len(related) != 1 || related[0].Key != "vn:vbpl:1" {
		t.Fatalf("related=%+v err=%v", related, err)
	}
	legacy := run
	legacy.Snapshot.SourceList = []byte(`{"vbpl":{"selection_version":0}}`)
	items, err = New().Plan(ctx, stage.Env{DB: db}, legacy)
	if err != nil || len(items) != 2 {
		t.Fatalf("legacy items=%+v err=%v", items, err)
	}
	if _, err = db.Exec(ctx, `UPDATE run_scope SET final_hash='invalid' WHERE run_id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if _, err = New().Plan(ctx, stage.Env{DB: db}, run); err == nil {
		t.Fatal("corrupt final phase accepted")
	}
}

func TestPlan_SelectionOneWithoutFinalMembershipFails(t *testing.T) {
	ctx := context.Background()
	db, err := pgxpool.New(ctx, testdb.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var id int64
	if err = db.QueryRow(ctx, `INSERT INTO run (country,rel,state,snapshot) VALUES ('vn','20990103','running','{}') RETURNING id`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	run := stage.Run{ID: id, Snapshot: stage.Snapshot{SourceList: []byte(`{"vbpl":{"selection_version":1}}`)}}
	check := func(t *testing.T) {
		t.Helper()
		if _, planErr := New().Plan(ctx, stage.Env{DB: db}, run); planErr == nil {
			t.Error("extract accepted missing final membership")
		}
		if _, planErr := relate.New().Plan(ctx, stage.Env{DB: db}, run); planErr == nil {
			t.Error("relate accepted missing final membership")
		}
	}
	t.Run("missing scope", check)
	if err = membership.PublishPhase(ctx, store.New(db), id, membership.Final, nil); err != nil {
		t.Fatal(err)
	}
	if err = membership.ClearPhase(ctx, store.New(db), id, membership.Final); err != nil {
		t.Fatal(err)
	}
	t.Run("cleared final phase", check)
}

func ptr(s string) *string { return &s }
