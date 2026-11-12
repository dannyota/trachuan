//go:build integration

package fetch

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/sourcegate"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/testdb"
	"danny.vn/trachuan/apps/corpus/internal/vbpl"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/time/rate"
)

type failedPauseGate struct{ *sourcegate.Memory }

func (*failedPauseGate) Pause(context.Context, int64, string, string, string) error {
	return errors.New("pause write failed")
}

func TestRunBatch_PausePersistenceFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusForbidden) }))
	defer server.Close()
	db, err := pgxpool.New(ctx, testdb.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var id int64
	if err = db.QueryRow(ctx, `INSERT INTO run(country,rel,state,generation,snapshot) VALUES ('vn','20990102','running',2,'{}') RETURNING id`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `INSERT INTO run_item(run_id,stage,item_key,state,batch,generation,attempt,message) VALUES ($1,'fetch','vn:vbpl:1','pending',0,2,1,NULL)`, id); err != nil {
		t.Fatal(err)
	}
	gate := &failedPauseGate{Memory: sourcegate.NewMemory()}
	client := vbpl.NewWithBase(server.URL, "test", vbpl.WithGate(gate), vbpl.WithLimits(float64(rate.Inf), 8), vbpl.WithBackoffs(), vbpl.WithTestTimeouts(5*time.Millisecond, 20*time.Millisecond))
	activityCtx := sourcegate.WithActivity(sourcegate.WithRunID(ctx, id))
	if _, _, err = client.Detail(activityCtx, "1"); !errors.Is(err, sourcegate.ErrSourcePaused) {
		t.Fatalf("initial error = %v", err)
	}
	extractor := New(client)
	raw, err := json.Marshal(stage.Snapshot{Recipes: map[string]string{"fetch": extractor.Recipe(stage.Snapshot{})}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `UPDATE run SET snapshot=$2 WHERE id=$1`, id, raw); err != nil {
		t.Fatal(err)
	}
	result, err := stage.RunBatch(ctx, stage.Env{DB: db}, extractor, stage.BatchInput{RunID: id, Generation: 2, Stage: "fetch"}, 2, stage.BatchConfig{HeartbeatEvery: time.Second, MaxAttempts: 5}, func(...any) {})
	if err == nil || result.Pending != 1 || result.Gap != 0 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	var state string
	var gaps int
	if err = db.QueryRow(ctx, `SELECT state,(SELECT count(*) FROM gap WHERE run_id=$1) FROM run_item WHERE run_id=$1`, id).Scan(&state, &gaps); err != nil || state != "pending" || gaps != 0 {
		t.Fatalf("state=%s gaps=%d err=%v", state, gaps, err)
	}
}
