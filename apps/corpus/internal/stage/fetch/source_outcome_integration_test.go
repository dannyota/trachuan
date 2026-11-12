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

func TestRunBatch_SourceEvidenceAndLoadOutcomes(t *testing.T) {
	for _, name := range []string{"conflict", "page 429", "PDF 429"} {
		t.Run(name, func(t *testing.T) {
			var s *Stage
			var env stage.Env
			key := "vn:chinhphu:218029"
			if name == "conflict" {
				s, env = conflictingAttachmentFixture(t)
				key = "vn:vbpl:66518"
			} else {
				s, env = broken429Fixture(t, name == "PDF 429")
			}
			ctx := context.Background()
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
			var id int64
			if err = db.QueryRow(ctx, `INSERT INTO run(country,rel,state,generation,snapshot) VALUES ('vn','20990111_1','running',1,$1) RETURNING id`, snapshot).Scan(&id); err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec(ctx, `INSERT INTO run_item(run_id,stage,item_key,state,batch) VALUES ($1,'fetch',$2,'pending',0)`, id, key); err != nil {
				t.Fatal(err)
			}
			result, err := stage.RunBatch(ctx, env, s, stage.BatchInput{RunID: id, Generation: 1, Stage: "fetch"}, 1, stage.BatchConfig{HeartbeatEvery: time.Second, MaxAttempts: 5}, func(...any) {})
			var state string
			var gaps int
			if e := db.QueryRow(ctx, `SELECT state,(SELECT count(*) FROM gap WHERE run_id=$1 AND kind='invalid_source_data') FROM run_item WHERE run_id=$1`, id).Scan(&state, &gaps); e != nil {
				t.Fatal(e)
			}
			if name == "conflict" {
				if err != nil || state != "done" || result.Done != 1 || gaps != 1 {
					t.Fatalf("result=%+v state=%s gaps=%d error=%v", result, state, gaps, err)
				}
			} else if err == nil || state != "pending" || result.Pending != 1 || result.Failed != 0 || gaps != 0 {
				t.Fatalf("result=%+v state=%s gaps=%d error=%v", result, state, gaps, err)
			}
		})
	}
}
