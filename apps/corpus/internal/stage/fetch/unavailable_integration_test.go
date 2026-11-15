//go:build integration

package fetch

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestUnavailable_CommitAndLaterRead(t *testing.T) {
	ctx := context.Background()
	f := &fixtureServer{}
	s, env := setup(t, f)
	db, err := pgxpool.New(ctx, testdb.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	env.DB = db
	snapshot, err := json.Marshal(stage.Snapshot{Recipes: map[string]string{"fetch": s.Recipe(stage.Snapshot{})}})
	if err != nil {
		t.Fatal(err)
	}
	batch := func(rel string) int64 {
		t.Helper()
		var id int64
		if err := db.QueryRow(ctx, `INSERT INTO run(country,rel,state,generation,snapshot) VALUES ('vn',$1,'running',1,$2) RETURNING id`, rel, snapshot).Scan(&id); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(ctx, `INSERT INTO run_item(run_id,stage,item_key,state,batch) VALUES ($1,'fetch','vn:vbpl:66518','pending',0)`, id); err != nil {
			t.Fatal(err)
		}
		result, err := stage.RunBatch(ctx, env, s, stage.BatchInput{RunID: id, Generation: 1, Stage: "fetch"}, 5, stage.BatchConfig{HeartbeatEvery: time.Second, MaxAttempts: 5}, func(...any) {})
		if err != nil || result.Done != 1 {
			t.Fatalf("result=%+v err=%v", result, err)
		}
		return id
	}
	first := batch("20990101_1")
	f.detailStatus = 500
	second := batch("20990101_2")
	var state, hash1, hash2, message string
	var gaps, observations, acquired int
	var baselineRun int64
	err = db.QueryRow(ctx, `SELECT i.state,a.sha256,(SELECT a1.sha256 FROM run_item i1 JOIN artifact a1 ON a1.id=i1.artifact_id WHERE i1.run_id=$1),i.message,(SELECT count(*) FROM gap WHERE run_id=$2 AND kind='source_unavailable'),(SELECT count(*) FROM run_fetch_observation n JOIN run_fetch_observation o ON o.run_id=$1 AND o.item_key=n.item_key AND o.resource_key=n.resource_key WHERE n.run_id=$2 AND n.acquisition='adopted' AND n.origin_ref IS NOT NULL AND n.fetched_at IS NOT DISTINCT FROM o.fetched_at AND n.content_sha256=o.content_sha256 AND n.path=o.path),(SELECT count(*) FROM run_acquisition WHERE run_id=$2),(SELECT run_id FROM fetch_baseline WHERE document_key='vn:vbpl:66518') FROM run_item i JOIN artifact a ON a.id=i.artifact_id WHERE i.run_id=$2`, first, second).Scan(&state, &hash2, &hash1, &message, &gaps, &observations, &acquired, &baselineRun)
	if err != nil || state != "done" || hash1 != hash2 || message != "read=baseline_after_unavailable" || gaps != 1 || observations != 3 || acquired != 0 || baselineRun != first {
		t.Fatalf("state=%s hashes=%s,%s message=%s gaps=%d observations=%d acquired=%d baseline=%d err=%v", state, hash1, hash2, message, gaps, observations, acquired, baselineRun, err)
	}
	f.detailStatus = 0
	f.detailBody = `{"success":true,"data":{"id":"66518","title":"Updated fixture","updatedDate":"2099-01-02"}}`
	third := batch("20990101_3")
	var hash3 string
	if err := db.QueryRow(ctx, `SELECT b.run_id,a.sha256 FROM fetch_baseline b JOIN artifact a ON a.id=b.artifact_id WHERE b.document_key='vn:vbpl:66518'`).Scan(&baselineRun, &hash3); err != nil || baselineRun != third || hash3 == hash1 {
		t.Fatalf("baseline=%d expected=%d hashes=%s,%s err=%v", baselineRun, third, hash1, hash3, err)
	}
}

func TestUnavailable_CorruptBaselineRemainsGap(t *testing.T) {
	ctx := context.Background()
	f := &fixtureServer{}
	s, env := setup(t, f)
	db, err := pgxpool.New(ctx, testdb.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	env.DB = db
	snapshot, err := json.Marshal(stage.Snapshot{Recipes: map[string]string{"fetch": s.Recipe(stage.Snapshot{})}})
	if err != nil {
		t.Fatal(err)
	}
	run := func(rel string) (int64, stage.BatchResult) {
		t.Helper()
		var id int64
		if err := db.QueryRow(ctx, `INSERT INTO run(country,rel,state,generation,snapshot) VALUES ('vn',$1,'running',1,$2) RETURNING id`, rel, snapshot).Scan(&id); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(ctx, `INSERT INTO run_item(run_id,stage,item_key,state,batch) VALUES($1,'fetch','vn:vbpl:66518','pending',0)`, id); err != nil {
			t.Fatal(err)
		}
		result, err := stage.RunBatch(ctx, env, s, stage.BatchInput{RunID: id, Generation: 1, Stage: "fetch"}, 5, stage.BatchConfig{HeartbeatEvery: time.Second, MaxAttempts: 5}, func(...any) {})
		if err != nil {
			t.Fatal(err)
		}
		return id, result
	}
	first, result := run("20990102_1")
	if result.Done != 1 {
		t.Fatalf("result=%+v", result)
	}
	var path string
	if err := db.QueryRow(ctx, `SELECT a.path FROM run_item i JOIN artifact a ON a.id=i.artifact_id WHERE i.run_id=$1`, first).Scan(&path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(env.Artifacts.Abs(path), "tree.json"), []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	f.detailStatus = 500
	second, result := run("20990102_2")
	if result.Gap != 1 {
		t.Fatalf("result=%+v", result)
	}
	var state string
	var artifactID *int64
	if err := db.QueryRow(ctx, `SELECT state,artifact_id FROM run_item WHERE run_id=$1`, second).Scan(&state, &artifactID); err != nil || state != "gap" || artifactID != nil {
		t.Fatalf("state=%s artifact=%v err=%v", state, artifactID, err)
	}
}

func TestUnavailable_HTTP429LeavesBatchPending(t *testing.T) {
	ctx := context.Background()
	f := &fixtureServer{}
	s, env := setup(t, f)
	db, err := pgxpool.New(ctx, testdb.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	env.DB = db
	snapshot, err := json.Marshal(stage.Snapshot{Recipes: map[string]string{"fetch": s.Recipe(stage.Snapshot{})}})
	if err != nil {
		t.Fatal(err)
	}
	newRun := func(rel string) int64 {
		t.Helper()
		var id int64
		if err := db.QueryRow(ctx, `INSERT INTO run(country,rel,state,generation,snapshot) VALUES('vn',$1,'running',1,$2) RETURNING id`, rel, snapshot).Scan(&id); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(ctx, `INSERT INTO run_item(run_id,stage,item_key,state,batch) VALUES($1,'fetch','vn:vbpl:66518','pending',0)`, id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	batch := func(id int64) (stage.BatchResult, error) {
		return stage.RunBatch(ctx, env, s, stage.BatchInput{RunID: id, Generation: 1, Stage: "fetch"}, 5, stage.BatchConfig{HeartbeatEvery: time.Second, MaxAttempts: 5}, func(...any) {})
	}
	first := newRun("20990103_1")
	if result, err := batch(first); err != nil || result.Done != 1 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	f.detailStatus = 429
	second := newRun("20990103_2")
	result, err := batch(second)
	var state string
	var gaps, observations, acquisitions int
	queryErr := db.QueryRow(ctx, `SELECT state,(SELECT count(*) FROM gap WHERE run_id=$1),(SELECT count(*) FROM run_fetch_observation WHERE run_id=$1),(SELECT count(*) FROM run_acquisition WHERE run_id=$1) FROM run_item WHERE run_id=$1`, second).Scan(&state, &gaps, &observations, &acquisitions)
	if err == nil || result.Pending != 1 || queryErr != nil || state != "pending" || gaps != 0 || observations != 0 || acquisitions != 0 {
		t.Fatalf("result=%+v err=%v state=%s gaps=%d observations=%d acquisitions=%d query=%v", result, err, state, gaps, observations, acquisitions, queryErr)
	}
}
