//go:build integration

package build

import (
	"context"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/store"
)

func TestEnsureForRun_AfterCommitItem(t *testing.T) {
	f := recoverySetup(t)
	ctx := context.Background()
	if _, err := New(f.cfg).Resolve(ctx, f.env, f.run, f.item, f.art); err != nil {
		t.Fatalf("resolve before commit: %v", err)
	}
	if _, err := f.env.DB.Exec(ctx, `INSERT INTO run_item(run_id,stage,item_key,state,batch) VALUES($1,'build',$2,'pending',0)`, f.run.ID, f.item.Key); err != nil {
		t.Fatal(err)
	}
	applied, err := stage.CommitItem(ctx, f.env.DB, stage.Commit{RunID: f.run.ID, Stage: "build", ItemKey: f.item.Key, Generation: f.run.Generation, Attempt: 1, RecipeHash: New(f.cfg).Recipe(f.run.Snapshot), Artifact: &f.art, State: "done"})
	if err != nil || !applied {
		t.Fatalf("commit build item: applied=%v err=%v", applied, err)
	}
	db, err := EnsureForRun(ctx, f.cfg, store.New(f.env.DB), f.env.Artifacts, f.run)
	if err != nil || db.Name != "corpus_vn_"+f.run.Rel {
		t.Fatalf("resolve committed build: %v", err)
	}
}
