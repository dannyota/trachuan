//go:build integration

package run

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"danny.vn/traquydinh/apps/corpus/internal/stage"
	"danny.vn/traquydinh/apps/corpus/internal/testdb"
	"danny.vn/traquydinh/apps/corpus/internal/testtemporal"
	"github.com/jackc/pgx/v5/pgxpool"
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/converter"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
)

func TestMain(m *testing.M) {
	code := m.Run()
	if err := testtemporal.Stop(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "stop Temporal server:", err)
		code = 1
	}
	os.Exit(code)
}

func lifecycleDeps(t *testing.T) Deps {
	t.Helper()
	ctx := context.Background()
	url := testdb.New(t)
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	server := testtemporal.Server(t)
	rel := testtemporal.Release()
	w := worker.New(server.Client(), rel, worker.Options{})
	w.RegisterWorkflowWithOptions(func(ctx workflow.Context, input BuildInput) error {
		var signal string
		workflow.GetSignalChannel(ctx, "finish").Receive(ctx, &signal)
		return nil
	}, workflow.RegisterOptions{Name: WorkflowName})
	if err = w.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(w.Stop)
	return Deps{TaskQueue: rel, DB: db, Temporal: server.Client(), Stages: []string{"discover", "fetch"}, RecipesFor: func(stage.Snapshot) map[string]string { return map[string]string{"discover": "one", "fetch": "two"} }, SourceList: json.RawMessage(`{}`)}
}

func insertLifecycleRun(t *testing.T, d Deps, state string, updated time.Time, snapshot stage.Snapshot, next *string) {
	t.Helper()
	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	_, err = d.DB.Exec(context.Background(), `INSERT INTO run (country, rel, state, generation, snapshot, next_stage, updated_at) VALUES ('vn',$5,$1,1,$2,$3,$4)`, state, data, next, updated, d.TaskQueue)
	if err != nil {
		t.Fatal(err)
	}
}

func testSnapshot() stage.Snapshot {
	return stage.Snapshot{SourceList: json.RawMessage(`{}`), Stages: []string{"discover", "fetch"}, Recipes: map[string]string{"discover": "one", "fetch": "two"}}
}
func startOpen(t *testing.T, d Deps) {
	t.Helper()
	_, err := d.Temporal.ExecuteWorkflow(context.Background(), client.StartWorkflowOptions{ID: WorkflowID("vn", d.TaskQueue), TaskQueue: d.TaskQueue}, WorkflowName, BuildInput{})
	if err != nil {
		t.Fatal(err)
	}
}
func TestStart_RefusesWhileWorkflowOpen(t *testing.T) {
	t.Parallel()
	d := lifecycleDeps(t)
	insertLifecycleRun(t, d, "running", time.Now(), testSnapshot(), nil)
	startOpen(t, d)
	if err := Start(context.Background(), d, "vn", d.TaskQueue, "", ""); err == nil {
		t.Fatal("open workflow accepted")
	}
}
func TestStart_RefusesFreshStarting(t *testing.T) {
	t.Parallel()
	d := lifecycleDeps(t)
	insertLifecycleRun(t, d, "starting", time.Now(), testSnapshot(), nil)
	if err := Start(context.Background(), d, "vn", d.TaskQueue, "", ""); err == nil {
		t.Fatal("fresh start accepted")
	}
}
func TestStart_ReplacesStaleStarting(t *testing.T) {
	t.Parallel()
	d := lifecycleDeps(t)
	insertLifecycleRun(t, d, "starting", time.Now().Add(-3*time.Minute), testSnapshot(), nil)
	if err := Start(context.Background(), d, "vn", d.TaskQueue, "", ""); err != nil {
		t.Fatal(err)
	}
	var gen int
	if err := d.DB.QueryRow(context.Background(), `SELECT generation FROM run`).Scan(&gen); err != nil || gen != 2 {
		t.Fatalf("generation = %d, %v", gen, err)
	}
}
func TestStart_RefusesOlderGenerationWorkflow(t *testing.T) {
	t.Parallel()
	d := lifecycleDeps(t)
	insertLifecycleRun(t, d, "failed", time.Now(), testSnapshot(), nil)
	startOpen(t, d)
	err := Start(context.Background(), d, "vn", d.TaskQueue, "", "")
	if err == nil || !strings.Contains(err.Error(), "corpus cancel") || !strings.Contains(err.Error(), "corpus recover") {
		t.Fatalf("refusal = %v", err)
	}
}
func TestStart_ResumesAtNextStage(t *testing.T) {
	t.Parallel()
	d := lifecycleDeps(t)
	next := "fetch"
	insertLifecycleRun(t, d, "failed", time.Now(), testSnapshot(), &next)
	_, err := d.DB.Exec(context.Background(), `INSERT INTO run_item (run_id,stage,item_key,state,batch) SELECT id,'fetch','item','failed',0 FROM run`)
	if err != nil {
		t.Fatal(err)
	}
	if err = Start(context.Background(), d, "vn", d.TaskQueue, "", ""); err != nil {
		t.Fatal(err)
	}
	var state string
	if err = d.DB.QueryRow(context.Background(), `SELECT state FROM run_item`).Scan(&state); err != nil || state != "pending" {
		t.Fatalf("item = %s, %v", state, err)
	}
	h := d.Temporal.GetWorkflowHistory(context.Background(), WorkflowID("vn", d.TaskQueue), "", false, enumspb.HISTORY_EVENT_FILTER_TYPE_ALL_EVENT)
	event, err := h.Next()
	if err != nil {
		t.Fatal(err)
	}
	var input BuildInput
	if err = converter.GetDefaultDataConverter().FromPayloads(event.GetWorkflowExecutionStartedEventAttributes().Input, &input); err != nil || input.Stage != "fetch" || !input.Begin {
		t.Fatalf("input = %+v, %v", input, err)
	}
}
func TestStart_PausedRunAppendsNewStageRecipes(t *testing.T) {
	t.Parallel()
	d := lifecycleDeps(t)
	snap := testSnapshot()
	snap.Stages = snap.Stages[:1]
	delete(snap.Recipes, "fetch")
	next := "fetch"
	insertLifecycleRun(t, d, "paused", time.Now(), snap, &next)
	if err := Start(context.Background(), d, "vn", d.TaskQueue, "", ""); err != nil {
		t.Fatal(err)
	}
	var raw []byte
	if err := d.DB.QueryRow(context.Background(), `SELECT snapshot FROM run`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &snap); err != nil || len(snap.Stages) != 2 || snap.Recipes["fetch"] != "two" {
		t.Fatalf("snapshot = %+v, %v", snap, err)
	}
}
func TestRecover_BumpsGenerationAndFails(t *testing.T) {
	t.Parallel()
	d := lifecycleDeps(t)
	insertLifecycleRun(t, d, "running", time.Now(), testSnapshot(), nil)
	if err := Recover(context.Background(), d, "vn", d.TaskQueue, false); err != nil {
		t.Fatal(err)
	}
	var gen int
	var state, message string
	if err := d.DB.QueryRow(context.Background(), `SELECT generation,state,message FROM run`).Scan(&gen, &state, &message); err != nil || gen != 2 || state != "failed" || message != "recovered" {
		t.Fatalf("run = %d %s %s, %v", gen, state, message, err)
	}
}

func TestCancel_OpenWorkflowDrains(t *testing.T) {
	t.Parallel()
	d := lifecycleDeps(t)
	insertLifecycleRun(t, d, "running", time.Now(), testSnapshot(), nil)
	startOpen(t, d)
	if err := Cancel(context.Background(), d, "vn", d.TaskQueue); err != nil {
		t.Fatal(err)
	}
	var state string
	if err := d.DB.QueryRow(context.Background(), `SELECT state FROM run`).Scan(&state); err != nil || state != "cancelling" {
		t.Fatalf("state = %q, %v", state, err)
	}
}

func TestRecover_TerminatesOnlyWhenRequested(t *testing.T) {
	t.Parallel()
	d := lifecycleDeps(t)
	insertLifecycleRun(t, d, "running", time.Now(), testSnapshot(), nil)
	startOpen(t, d)
	if err := Recover(context.Background(), d, "vn", d.TaskQueue, false); err != nil {
		t.Fatal(err)
	}
	open, err := workflowOpen(context.Background(), d, WorkflowID("vn", d.TaskQueue))
	if err != nil || !open {
		t.Fatalf("open after recover = %v, %v", open, err)
	}
	if err = Recover(context.Background(), d, "vn", d.TaskQueue, true); err != nil {
		t.Fatal(err)
	}
	open, err = workflowOpen(context.Background(), d, WorkflowID("vn", d.TaskQueue))
	if err != nil || open {
		t.Fatalf("open after terminate = %v, %v", open, err)
	}
}

func TestStart_RecipesUseFrozenSourceList(t *testing.T) {
	t.Parallel()
	d := lifecycleDeps(t)
	d.SourceList = json.RawMessage(`{"source":"original"}`)
	d.RecipesFor = func(s stage.Snapshot) map[string]string {
		var source struct {
			Source string `json:"source"`
		}
		_ = json.Unmarshal(s.SourceList, &source)
		return map[string]string{"discover": source.Source, "fetch": source.Source}
	}
	if err := Start(context.Background(), d, "vn", d.TaskQueue, "", ""); err != nil {
		t.Fatal(err)
	}
	var raw []byte
	if err := d.DB.QueryRow(context.Background(), `SELECT snapshot FROM run`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var snap stage.Snapshot
	if err := json.Unmarshal(raw, &snap); err != nil || snap.Recipes["discover"] != "original" {
		t.Fatalf("snapshot = %+v, %v", snap, err)
	}
	if _, err := d.DB.Exec(context.Background(), `UPDATE run SET state='paused', snapshot=jsonb_set(snapshot::jsonb, '{Stages}', '["discover"]'::jsonb)`); err != nil {
		t.Fatal(err)
	}
	if _, err := d.DB.Exec(context.Background(), `UPDATE run SET snapshot=jsonb_set(snapshot::jsonb, '{Recipes}', jsonb_build_object('discover', snapshot::jsonb->'Recipes'->'discover'))`); err != nil {
		t.Fatal(err)
	}
	if err := d.Temporal.TerminateWorkflow(context.Background(), WorkflowID("vn", d.TaskQueue), "", "test resume"); err != nil {
		t.Fatal(err)
	}
	d.SourceList = json.RawMessage(`{"source":"changed"}`)
	if err := Start(context.Background(), d, "vn", d.TaskQueue, "", ""); err != nil {
		t.Fatal(err)
	}
	if err := d.DB.QueryRow(context.Background(), `SELECT snapshot FROM run`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	snap = stage.Snapshot{}
	if err := json.Unmarshal(raw, &snap); err != nil || snap.Recipes["discover"] != "original" || snap.Recipes["fetch"] != "original" {
		t.Fatalf("resumed snapshot = %+v, %v", snap, err)
	}
}

func TestStart_CompleteRunRefusesWithoutNewStages(t *testing.T) {
	t.Parallel()
	d := lifecycleDeps(t)
	next := ""
	insertLifecycleRun(t, d, "paused", time.Now(), testSnapshot(), &next)
	if err := Start(context.Background(), d, "vn", d.TaskQueue, "", ""); err == nil || !strings.Contains(err.Error(), "run is complete") {
		t.Fatalf("err=%v", err)
	}
}

func TestStart_CompleteRunStartsAtAppendedStage(t *testing.T) {
	t.Parallel()
	d := lifecycleDeps(t)
	snap := testSnapshot()
	snap.Stages = snap.Stages[:1]
	delete(snap.Recipes, "fetch")
	next := ""
	insertLifecycleRun(t, d, "paused", time.Now(), snap, &next)
	if err := Start(context.Background(), d, "vn", d.TaskQueue, "", ""); err != nil {
		t.Fatal(err)
	}
	var got string
	if err := d.DB.QueryRow(context.Background(), `SELECT next_stage FROM run`).Scan(&got); err != nil || got != "fetch" {
		t.Fatalf("next=%q err=%v", got, err)
	}
}

func TestStart_RefusesUntilBeforeResumeStage(t *testing.T) {
	t.Parallel()
	d := lifecycleDeps(t)
	next := "fetch"
	insertLifecycleRun(t, d, "paused", time.Now(), testSnapshot(), &next)
	if err := Start(context.Background(), d, "vn", d.TaskQueue, "discover", ""); err == nil {
		t.Fatal("earlier until accepted")
	}
}

func TestStart_ChecksOpenWorkflowInEveryState(t *testing.T) {
	t.Parallel()
	for _, state := range []string{"failed", "paused", "cancelled"} {
		t.Run(state, func(t *testing.T) {
			d := lifecycleDeps(t)
			insertLifecycleRun(t, d, state, time.Now(), testSnapshot(), nil)
			startOpen(t, d)
			err := Start(context.Background(), d, "vn", d.TaskQueue, "", "")
			var generation int
			queryErr := d.DB.QueryRow(context.Background(), `SELECT generation FROM run`).Scan(&generation)
			if err == nil || !strings.Contains(err.Error(), "workflow is open") || queryErr != nil || generation != 1 {
				t.Fatalf("err=%v generation=%d query=%v", err, generation, queryErr)
			}
		})
	}
}

func TestCancel_RefusesReleasedRun(t *testing.T) {
	t.Parallel()
	for _, state := range []string{"evaluated", "released"} {
		t.Run(state, func(t *testing.T) {
			d := lifecycleDeps(t)
			insertLifecycleRun(t, d, state, time.Now(), testSnapshot(), nil)
			if err := Cancel(context.Background(), d, "vn", d.TaskQueue); err == nil {
				t.Fatal("terminal run accepted")
			}
		})
	}
}

func TestRecover_RefusesReleasedRun(t *testing.T) {
	t.Parallel()
	for _, state := range []string{"evaluated", "released"} {
		t.Run(state, func(t *testing.T) {
			d := lifecycleDeps(t)
			insertLifecycleRun(t, d, state, time.Now(), testSnapshot(), nil)
			if err := Recover(context.Background(), d, "vn", d.TaskQueue, false); err == nil {
				t.Fatal("terminal run accepted")
			}
		})
	}
}
