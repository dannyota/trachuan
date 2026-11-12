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
	for _, name := range []string{"conflict", "page 429", "PDF 429", "stalled page 429", "stalled PDF 429", "unexpected page 206"} {
		t.Run(name, func(t *testing.T) {
			var s *Stage
			var env stage.Env
			var calls *int
			var waits *[]time.Duration
			key := "vn:chinhphu:218029"
			switch name {
			case "conflict":
				s, env = conflictingAttachmentFixture(t)
				key = "vn:vbpl:66518"
			case "stalled page 429", "stalled PDF 429":
				s, env, calls, waits = stalled429Fixture(t, name == "stalled PDF 429", time.Millisecond)
			case "unexpected page 206":
				s, env, calls = unexpected206Fixture(t, time.Millisecond)
			default:
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
			var processor stage.Stage = s
			if calls != nil {
				processor = clockedSourceStage{Stage: s, test: t}
			}
			result, err := stage.RunBatch(ctx, env, processor, stage.BatchInput{RunID: id, Generation: 1, Stage: "fetch"}, 5, stage.BatchConfig{HeartbeatEvery: time.Second, MaxAttempts: 5}, func(...any) {})
			if waits != nil && (*calls != 4 || len(*waits) != 3) {
				t.Fatalf("calls=%d waits=%v", *calls, *waits)
			}
			var state string
			var gaps int
			if e := db.QueryRow(ctx, `SELECT state,(SELECT count(*) FROM gap WHERE run_id=$1 AND kind IN ('missing_attachment','source_unavailable')) FROM run_item WHERE run_id=$1`, id).Scan(&state, &gaps); e != nil {
				t.Fatal(e)
			}
			switch name {
			case "unexpected page 206":
				if err != nil || *calls != 1 || state != "gap" || result.Gap != 1 || result.Failed != 0 || result.Pending != 0 || gaps != 1 {
					t.Fatalf("result=%+v state=%s gaps=%d calls=%d error=%v", result, state, gaps, *calls, err)
				}
			case "conflict":
				if err != nil || state != "done" || result.Done != 1 || gaps != 1 {
					t.Fatalf("result=%+v state=%s gaps=%d error=%v", result, state, gaps, err)
				}
			default:
				if err == nil || state != "pending" || result.Pending != 1 || result.Failed != 0 || gaps != 0 {
					t.Fatalf("result=%+v state=%s gaps=%d error=%v", result, state, gaps, err)
				}
			}
		})
	}
}
