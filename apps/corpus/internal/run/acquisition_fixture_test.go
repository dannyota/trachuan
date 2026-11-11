//go:build integration

package run_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/run"
	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/sourcegate"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/expand"
	"danny.vn/trachuan/apps/corpus/internal/stage/fetch"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"danny.vn/trachuan/apps/corpus/internal/testdb"
	"danny.vn/trachuan/apps/corpus/internal/vbpl"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.temporal.io/api/serviceerror"
	workflowservice "go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"
)

type acquisitionFixture struct {
	t             *testing.T
	env           stage.Env
	run           stage.Run
	expand, fetch stage.Stage
	mu            sync.Mutex
	requests      map[string]int
	fail          bool
	content       string
	dbURL         string
	base          string
}

func newAcquisitionFixture(t *testing.T) *acquisitionFixture {
	t.Helper()
	f := &acquisitionFixture{t: t, requests: map[string]int{}, fail: true, content: "original"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.requests[r.URL.Path]++
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/doc/2/diagram" && f.fail:
			w.WriteHeader(http.StatusInternalServerError)
		case strings.HasSuffix(r.URL.Path, "/diagram"):
			_, _ = w.Write([]byte(`{"success":true,"data":{"documentNamesByType":{}}}`))
		case strings.Contains(r.URL.Path, "/tree/"):
			_, _ = w.Write([]byte(`{"success":true,"data":[]}`))
		case strings.HasSuffix(r.URL.Path, "/files"):
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": []map[string]any{{"fileName": "a.txt", "fullPath": "a", "size": len(f.content), "presignedUrl": "http://" + r.Host + "/attachment/a"}, {"fileName": "missing.txt", "fullPath": "missing", "size": 1, "presignedUrl": "http://" + r.Host + "/attachment/missing"}}})
		case strings.HasPrefix(r.URL.Path, "/attachment/"):
			if strings.Contains(r.URL.Path, "missing") {
				http.NotFound(w, r)
			} else {
				_, _ = w.Write([]byte(f.content))
			}
		case strings.HasPrefix(r.URL.Path, "/doc/"):
			id := strings.TrimPrefix(r.URL.Path, "/doc/")
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"id": id, "title": f.content, "references": []any{}}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	f.base = server.URL
	f.env.Artifacts = artifact.Store{Root: t.TempDir()}
	var err error
	f.dbURL = testdb.New(t)
	f.env.DB, err = pgxpool.New(context.Background(), f.dbURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.env.DB.Close)
	f.bindSourceStages()
	list, err := json.Marshal(source.List{Country: "vn"})
	if err != nil {
		t.Fatal(err)
	}
	f.run = stage.Run{Country: "vn", Rel: "20990111_1", Generation: 1, Snapshot: stage.Snapshot{SourceList: list, Stages: []string{"discover", "expand", "fetch"}}}
	f.run.Snapshot.Recipes = map[string]string{"discover": strings.Repeat("d", 64), "expand": f.expand.Recipe(f.run.Snapshot), "fetch": f.fetch.Recipe(f.run.Snapshot)}
	raw, err := json.Marshal(f.run.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	err = f.env.DB.QueryRow(context.Background(), `INSERT INTO run(country,rel,state,generation,snapshot,next_stage) VALUES ('vn',$1,'running',1,$2,'expand') RETURNING id`, f.run.Rel, raw).Scan(&f.run.ID)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *acquisitionFixture) bindSourceStages() {
	client := vbpl.NewWithBase(f.base, "test", vbpl.WithTestHTTP(), vbpl.WithLimits(100000, 8), vbpl.WithBackoffs(), vbpl.WithGate(sourcegate.New(f.env.DB)))
	f.expand, f.fetch = expand.New(client), fetch.New(client)
}

func (f *acquisitionFixture) exec(sql string, args ...any) {
	f.t.Helper()
	if _, err := f.env.DB.Exec(context.Background(), sql, args...); err != nil {
		f.t.Fatal(err)
	}
}

func (f *acquisitionFixture) plan(name string) {
	f.t.Helper()
	for _, key := range []string{"vn:vbpl:1", "vn:vbpl:2"} {
		f.exec(`INSERT INTO run_item(run_id,stage,item_key,state,batch) VALUES ($1,$2,$3,'pending',0) ON CONFLICT DO NOTHING`, f.run.ID, name, key)
	}
}

func (f *acquisitionFixture) batch(st stage.Stage) {
	f.t.Helper()
	_, err := stage.RunBatch(context.Background(), f.env, st, stage.BatchInput{RunID: f.run.ID, Stage: st.Name(), Generation: f.run.Generation}, 1, stage.BatchConfig{HeartbeatEvery: time.Second, MaxAttempts: 1}, func(...any) {})
	if err != nil {
		f.t.Fatal(err)
	}
}

func (f *acquisitionFixture) rewind() {
	f.t.Helper()
	ctx := context.Background()
	f.exec(`UPDATE run SET state='paused',next_stage='' WHERE id=$1`, f.run.ID)
	if err := run.Start(ctx, f.deps(), "vn", f.run.Rel, "fetch", ""); err != nil {
		f.t.Fatal(err)
	}
	f.run.Generation++
}

func (f *acquisitionFixture) deps() run.Deps {
	return run.Deps{DB: f.env.DB, Artifacts: f.env.Artifacts, Temporal: closedWorkflowClient{}, Stages: f.run.Snapshot.Stages, SourceList: f.run.Snapshot.SourceList, RecipesFor: func(stage.Snapshot) map[string]string { return f.run.Snapshot.Recipes }}
}

type closedWorkflowClient struct{ client.Client }

func (closedWorkflowClient) DescribeWorkflowExecution(context.Context, string, string) (*workflowservice.DescribeWorkflowExecutionResponse, error) {
	return nil, serviceerror.NewNotFound("no workflow")
}
func (closedWorkflowClient) ExecuteWorkflow(context.Context, client.StartWorkflowOptions, any, ...any) (client.WorkflowRun, error) {
	return nil, nil
}

func (f *acquisitionFixture) count(path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests[path]
}

func (f *acquisitionFixture) scalar(sql string, args ...any) int {
	f.t.Helper()
	var n int
	if err := f.env.DB.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		f.t.Fatal(err)
	}
	return n
}

func (f *acquisitionFixture) acquired(name, key string) store.GetRunAcquisitionRow {
	f.t.Helper()
	a, err := store.New(f.env.DB).GetRunAcquisition(context.Background(), store.GetRunAcquisitionParams{RunID: f.run.ID, Stage: name, DocumentKey: key})
	if err != nil {
		f.t.Fatal(err)
	}
	return a
}

func (f *acquisitionFixture) bundlePath() string {
	return filepath.Join(f.env.Artifacts.Root, f.acquired("fetch", "vn:vbpl:1").Path)
}

func (f *acquisitionFixture) provenance() string {
	f.t.Helper()
	var s string
	err := f.env.DB.QueryRow(context.Background(), `SELECT coalesce(jsonb_agg(to_jsonb(ff) ORDER BY artifact_id,name,sha256),'[]')::text FROM fetched_file ff`).Scan(&s)
	if err != nil {
		f.t.Fatal(err)
	}
	return s
}

func (f *acquisitionFixture) assertRequests(path string, want int) {
	f.t.Helper()
	if got := f.count(path); got != want {
		f.t.Fatalf("requests %s=%d, want %d", path, got, want)
	}
}
