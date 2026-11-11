//go:build integration

package fetch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/testdb"
	"danny.vn/trachuan/apps/corpus/internal/vbpl"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/time/rate"
)

func TestRunBatch_HungVBPLRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/1") {
			<-r.Context().Done()
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	ctx := context.Background()
	db, err := pgxpool.New(ctx, testdb.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var id int64
	if err = db.QueryRow(ctx, `INSERT INTO run(country,rel,state,generation,snapshot) VALUES ('vn','20990102','running',2,'{}') RETURNING id`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `INSERT INTO run_item(run_id,stage,item_key,state,batch,generation,attempt) VALUES ($1,'fetch','vn:vbpl:1','pending',0,0,0),($1,'fetch','vn:vbpl:2','pending',0,0,0)`, id); err != nil {
		t.Fatal(err)
	}
	c := vbpl.NewWithBase(server.URL, "test", vbpl.WithLimits(float64(rate.Inf), 16), vbpl.WithBackoffs(40*time.Millisecond), vbpl.WithTestTimeouts(5*time.Millisecond, 15*time.Millisecond))
	env := stage.Env{DB: db, Artifacts: artifact.Store{Root: t.TempDir()}}
	result, err := stage.RunBatch(ctx, env, New(c), stage.BatchInput{RunID: id, Generation: 2, Stage: "fetch"}, 5, stage.BatchConfig{HeartbeatEvery: time.Second, MaxAttempts: 5}, func(...any) {})
	if err != nil || result.Gap != 2 {
		t.Fatalf("result = %+v, error = %v", result, err)
	}
	var kind, origin, cause string
	if err = db.QueryRow(ctx, `SELECT kind,origin,cause FROM gap WHERE run_id=$1 AND item_key='vn:vbpl:1'`, id).Scan(&kind, &origin, &cause); err != nil || kind != "source_unavailable" || origin != "transport" || cause != "timeout" {
		t.Fatalf("gap = %s %s %s, error = %v", kind, origin, cause, err)
	}
	var state string
	if err = db.QueryRow(ctx, `SELECT state FROM run_item WHERE run_id=$1 AND item_key='vn:vbpl:2'`, id).Scan(&state); err != nil || state != "gap" {
		t.Fatalf("later state = %q, error = %v", state, err)
	}
}
