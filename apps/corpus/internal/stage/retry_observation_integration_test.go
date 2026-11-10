//go:build integration

package stage_test

import (
	"context"
	"encoding/json"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/run"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.temporal.io/api/serviceerror"
	workflowservice "go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"
)

type retryTemporal struct{ client.Client }

func (retryTemporal) DescribeWorkflowExecution(context.Context, string, string) (*workflowservice.DescribeWorkflowExecutionResponse, error) {
	return nil, serviceerror.NewNotFound("no workflow")
}
func (retryTemporal) ExecuteWorkflow(context.Context, client.StartWorkflowOptions, interface{}, ...interface{}) (client.WorkflowRun, error) {
	return nil, nil
}

func TestRetry_DetachesObservations(t *testing.T) {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, testdb.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	snapshot, err := json.Marshal(stage.Snapshot{SourceList: json.RawMessage(`{}`), Stages: []string{"fetch"}, Recipes: map[string]string{"fetch": "recipe"}})
	if err != nil {
		t.Fatal(err)
	}
	var id int64
	if err = pool.QueryRow(ctx, `INSERT INTO run(country,rel,state,generation,snapshot,next_stage) VALUES ('vn','20990109','paused',1,$1,'fetch') RETURNING id`, snapshot).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO run_item(run_id,stage,item_key,state,batch,generation,attempt) VALUES ($1,'fetch','item','gap',0,1,3)`, id); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO gap(run_id,stage,item_key,kind,detail) VALUES ($1,'fetch','item','source_unavailable','')`, id); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO run_fetch_observation(run_id,item_key,generation,attempt,resource_key,artifact_sha256,source,path,content_sha256,size,acquisition) VALUES ($1,'item',1,3,'["detail",null,null]','sha','vbpl','detail.json','sha',1,'network')`, id); err != nil {
		t.Fatal(err)
	}
	deps := run.Deps{DB: pool, Temporal: retryTemporal{}, Stages: []string{"fetch"}, SourceList: json.RawMessage(`{}`), RecipesFor: func(stage.Snapshot) map[string]string { return map[string]string{"fetch": "recipe"} }}
	if err = run.Start(ctx, deps, "vn", "20990109", "", ""); err != nil {
		t.Fatal(err)
	}
	var history, current int
	if err = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM run_fetch_observation WHERE run_id=$1),(SELECT count(*) FROM run_fetch_observation o JOIN run_item i ON i.run_id=o.run_id AND i.item_key=o.item_key AND i.generation=o.generation AND i.attempt=o.attempt WHERE o.run_id=$1)`, id).Scan(&history, &current); err != nil || history != 1 || current != 0 {
		t.Fatalf("history=%d current=%d: %v", history, current, err)
	}
}
