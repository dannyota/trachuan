//go:build integration

package fetch

import (
	"encoding/json"
	"testing"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/membership"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"danny.vn/trachuan/apps/corpus/internal/vbpl"
)

func TestConflictBundle_CloseAndAdoption(t *testing.T) {
	f := newHandOverFixture(t, "2026-01-01", "2026-01-01", "gap", "gap")
	s, _ := conflictingAttachmentFixture(t, "1")
	s.WithChinhphu(f.stage.chinhphu)
	f.run.Snapshot.Recipes = map[string]string{"fetch": s.Recipe(f.run.Snapshot)}
	snapshot, err := json.Marshal(f.run.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.Exec(f.ctx, `UPDATE run SET snapshot=$2 WHERE id=$1`, f.run.ID, snapshot); err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.Exec(f.ctx, `DELETE FROM gap WHERE run_id=$1 AND item_key='vn:vbpl:1'`, f.run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.Exec(f.ctx, `UPDATE run_item SET state='pending' WHERE run_id=$1 AND stage='fetch' AND item_key='vn:vbpl:1'`, f.run.ID); err != nil {
		t.Fatal(err)
	}
	batch := stage.BatchInput{RunID: f.run.ID, Stage: "fetch"}
	cfg := stage.BatchConfig{HeartbeatEvery: time.Second, MaxAttempts: 5}
	result, err := stage.RunBatch(f.ctx, f.env, s, batch, 1, cfg, func(...any) {})
	if err != nil || result.Done != 1 || result.Failed != 0 {
		t.Fatalf("batch=%+v error=%v", result, err)
	}
	closed, err := stage.CloseStage(f.ctx, f.env, s, stage.StageInput{RunID: f.run.ID, Stage: "fetch"}, "extract")
	if err != nil || closed.Failed || closed.NextStage != "extract" {
		t.Fatalf("close=%+v error=%v", closed, err)
	}
	if _, err = membership.ReadPhase(f.ctx, store.New(f.db), f.run.ID, membership.Final); err != nil {
		t.Fatal(err)
	}
	var before int
	if err = f.db.QueryRow(f.ctx, `SELECT count(*) FROM run_fetch_observation WHERE run_id=$1`, f.run.ID).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.Exec(f.ctx, `UPDATE run SET generation=1,next_stage='fetch' WHERE id=$1`, f.run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.Exec(f.ctx, `UPDATE run_item SET state='pending',artifact_id=NULL,input_hash=NULL,attempt=0 WHERE run_id=$1 AND stage='fetch' AND item_key='vn:vbpl:1'`, f.run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.Exec(f.ctx, `DELETE FROM gap WHERE run_id=$1 AND stage='fetch' AND item_key='vn:vbpl:1'`, f.run.ID); err != nil {
		t.Fatal(err)
	}
	// Any source call would fail to produce a done item after adoption.
	s.client = vbpl.NewWithBase("http://127.0.0.1:1", "test", vbpl.WithTestHTTP(), vbpl.WithBackoffs())
	batch.Generation = 1
	result, err = stage.RunBatch(f.ctx, f.env, s, batch, 1, cfg, func(...any) {})
	if err != nil || result.Done != 1 {
		t.Fatalf("adoption=%+v error=%v", result, err)
	}
	var after, gaps int
	if err = f.db.QueryRow(f.ctx, `SELECT (SELECT count(*) FROM run_fetch_observation WHERE run_id=$1),(SELECT count(*) FROM gap WHERE run_id=$1 AND stage='fetch' AND item_key='vn:vbpl:1' AND kind='missing_attachment' AND detail='a')`, f.run.ID).Scan(&after, &gaps); err != nil {
		t.Fatal(err)
	}
	if after != before || gaps != 1 {
		t.Fatalf("observations=%d want=%d reproduced gaps=%d", after, before, gaps)
	}
}
