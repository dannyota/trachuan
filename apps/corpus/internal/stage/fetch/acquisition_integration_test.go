//go:build integration

package fetch

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestFetch_ChinhphuAcquisitionRetainsFileTimesWithoutClient(t *testing.T) {
	ctx := context.Background()
	token := "placeholder"
	s, env := secondaryFixture(t, false, &token)
	var err error
	env.DB, err = pgxpool.New(ctx, testdb.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer env.DB.Close()
	snap := stage.Snapshot{Recipes: map[string]string{"fetch": s.Recipe(stage.Snapshot{})}}
	raw, err := json.Marshal(snap)
	if err != nil {
		t.Fatal(err)
	}
	var id int64
	if err = env.DB.QueryRow(ctx, `INSERT INTO run(country,rel,state,generation,snapshot) VALUES ('vn','20990111_1','running',1,$1) RETURNING id`, raw).Scan(&id); err != nil {
		t.Fatal(err)
	}
	exec := func(sql string) {
		t.Helper()
		if _, e := env.DB.Exec(ctx, sql, id); e != nil {
			t.Fatal(e)
		}
	}
	exec(`INSERT INTO run_item(run_id,stage,item_key,state,batch) VALUES ($1,'fetch','vn:chinhphu:218029','pending',0)`)
	batch := func(generation int) {
		t.Helper()
		got, e := stage.RunBatch(ctx, env, s, stage.BatchInput{RunID: id, Generation: generation, Stage: "fetch"}, 1, stage.BatchConfig{HeartbeatEvery: time.Second, MaxAttempts: 1}, func(...any) {})
		if e != nil || got.Done != 1 {
			t.Fatalf("batch=%+v: %v", got, e)
		}
	}
	batch(1)
	var before, after string
	if err = env.DB.QueryRow(ctx, `SELECT jsonb_agg(to_jsonb(f))::text FROM fetched_file f`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	exec(`UPDATE run SET generation=2 WHERE id=$1`)
	exec(`UPDATE run_item SET state='pending',input_hash=NULL,artifact_id=NULL,message=NULL,attempt=0 WHERE run_id=$1`)
	s.chinhphu = nil
	batch(2)
	if err = env.DB.QueryRow(ctx, `SELECT jsonb_agg(to_jsonb(f))::text FROM fetched_file f`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatalf("file provenance changed: %s -> %s", before, after)
	}
	var observations, adopted int
	err = env.DB.QueryRow(ctx, `SELECT (SELECT count(*) FROM run_fetch_observation WHERE generation=2),(SELECT count(*) FROM run_item WHERE generation=2 AND state='done' AND message='acquisition reused')`).Scan(&observations, &adopted)
	if err != nil || observations != 0 || adopted != 1 {
		t.Fatalf("observations=%d adopted=%d: %v", observations, adopted, err)
	}
}
