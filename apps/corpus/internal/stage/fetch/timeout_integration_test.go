//go:build integration

package fetch

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
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
	cancelled := make(chan error, 1)
	transport := sourceTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/doc/1" || r.URL.Path == "/doc/1/diagram" {
			<-r.Context().Done()
			err := r.Context().Err()
			if errors.Is(err, context.DeadlineExceeded) {
				select {
				case cancelled <- err:
				default:
				}
			}
			return nil, err
		}
		status := http.StatusNotFound
		if strings.Contains(r.URL.Path, "/3") {
			status = http.StatusTooManyRequests
		}
		return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
	})
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
	if _, err = db.Exec(ctx, `INSERT INTO run_item(run_id,stage,item_key,state,batch,generation,attempt) VALUES ($1,'fetch','vn:vbpl:1','pending',0,0,0),($1,'fetch','vn:vbpl:2','pending',0,0,0),($1,'fetch','vn:vbpl:3','pending',0,0,0)`, id); err != nil {
		t.Fatal(err)
	}
	c := vbpl.NewWithBase("http://127.0.0.1", "test", vbpl.WithTransport(transport), vbpl.WithLimits(float64(rate.Inf), 16), vbpl.WithBackoffs(), vbpl.WithTestTimeouts(5*time.Millisecond, 20*time.Millisecond))
	env := stage.Env{DB: db, Artifacts: artifact.Store{Root: t.TempDir()}}
	extractor := New(c)
	raw, err := json.Marshal(stage.Snapshot{Recipes: map[string]string{"fetch": extractor.Recipe(stage.Snapshot{})}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `UPDATE run SET snapshot=$2 WHERE id=$1`, id, raw); err != nil {
		t.Fatal(err)
	}
	in := stage.BatchInput{RunID: id, Generation: 2, Stage: "fetch"}
	batchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	result, err := stage.RunBatch(batchCtx, env, extractor, in, 1, stage.BatchConfig{HeartbeatEvery: time.Second, MaxAttempts: 5}, func(...any) {})
	if err == nil || result.Pending != 1 || result.Gap != 2 {
		t.Fatalf("attempt 1: result = %+v, error = %v", result, err)
	}
	select {
	case cancelErr := <-cancelled:
		if !errors.Is(cancelErr, context.DeadlineExceeded) || batchCtx.Err() != nil {
			t.Fatalf("request cancellation = %v", cancelErr)
		}
	case <-batchCtx.Done():
		t.Fatal("request cancellation was not observed")
	}
	result, err = stage.RunBatch(batchCtx, env, extractor, in, 2, stage.BatchConfig{HeartbeatEvery: time.Second, MaxAttempts: 5}, func(...any) {})
	if err == nil || result.Pending != 1 || result.Gap != 0 {
		t.Fatalf("result = %+v, error = %v", result, err)
	}
	var kind, origin, cause string
	if err = db.QueryRow(ctx, `SELECT kind,origin,cause FROM gap WHERE run_id=$1 AND item_key='vn:vbpl:1'`, id).Scan(&kind, &origin, &cause); err != nil || kind != "source_unavailable" || origin != "transport" || cause != "timeout" {
		t.Fatalf("gap = %s %s %s, error = %v", kind, origin, cause, err)
	}
	var state string
	if err = db.QueryRow(ctx, `SELECT state FROM run_item WHERE run_id=$1 AND item_key='vn:vbpl:3'`, id).Scan(&state); err != nil || state != "pending" {
		t.Fatalf("rate-limited state = %q, error = %v", state, err)
	}
	if err = db.QueryRow(ctx, `SELECT state FROM run_item WHERE run_id=$1 AND item_key='vn:vbpl:2'`, id).Scan(&state); err != nil || state != "gap" {
		t.Fatalf("later state = %q, error = %v", state, err)
	}
}
