//go:build integration

package stage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/dannyota/traquydinh/apps/corpus/internal/artifact"
	"github.com/jackc/pgx/v5"
)

type controlStage struct {
	items []Item
	calls int
}

func (*controlStage) Name() string           { return "fetch" }
func (*controlStage) Recipe(Snapshot) string { return "recipe" }
func (s *controlStage) Plan(context.Context, Env, Run) ([]Item, error) {
	s.calls++
	return s.items, nil
}
func (*controlStage) Process(context.Context, Env, Run, Item) (Output, error) { return Output{}, nil }
func (*controlStage) Rows(context.Context, artifact.Result) (func(context.Context, pgx.Tx, int64) error, error) {
	return nil, nil
}
func (*controlStage) Resolve(context.Context, Env, Run, Item, artifact.Result) (Resolution, error) {
	return Resolution{}, nil
}

func controlSetup(t *testing.T) (Env, StageInput, *controlStage) {
	t.Helper()
	pool, id := setup(t)
	batchExec(t, pool, `DELETE FROM run_item WHERE run_id=$1`, id)
	batchExec(t, pool, `UPDATE run SET snapshot='{"Recipes":{"fetch":"recipe","other":"other"}}', next_stage='fetch' WHERE id=$1`, id)
	return Env{DB: pool}, StageInput{RunID: id, Generation: 2, Stage: "fetch"}, &controlStage{}
}
func controlPlan(env Env, in StageInput, s *controlStage) (PlanResult, error) {
	return PlanStage(context.Background(), env, s, in, map[string]string{"fetch": "recipe"})
}
func TestPlanStage_InsertsBatchesOfHundred(t *testing.T) {
	env, in, s := controlSetup(t)
	for i := 0; i < 250; i++ {
		s.items = append(s.items, Item{Key: fmt.Sprintf("%03d", i), InputHash: "input"})
	}
	got, err := controlPlan(env, in, s)
	if err != nil || len(got.Batches) != 3 || got.Batches[2] != 2 {
		t.Fatalf("result=%+v err=%v", got, err)
	}
	var count, bad int
	if err := env.DB.QueryRow(context.Background(), `SELECT count(*), count(*) FILTER (WHERE batch <> (item_key::int / 100) OR input_hash <> 'input' OR recipe_hash <> 'recipe') FROM run_item WHERE run_id=$1`, in.RunID).Scan(&count, &bad); err != nil || count != 250 || bad != 0 {
		t.Fatalf("count=%d bad=%d err=%v", count, bad, err)
	}
}
func TestPlanStage_ListsBatchesWithHoles(t *testing.T) {
	env, in, s := controlSetup(t)
	for i := 0; i < 250; i++ {
		s.items = append(s.items, Item{Key: fmt.Sprintf("%03d", i)})
	}
	if _, err := controlPlan(env, in, s); err != nil {
		t.Fatal(err)
	}
	batchExec(t, env.DB, `UPDATE run_item SET state='done' WHERE run_id=$1 AND batch=1`, in.RunID)
	got, err := controlPlan(env, in, s)
	if err != nil || fmt.Sprint(got.Batches) != "[0 2]" {
		t.Fatalf("result=%+v err=%v", got, err)
	}
}
func TestPlanStage_RecordsSkipGap(t *testing.T) {
	env, in, s := controlSetup(t)
	s.items = []Item{{Key: "skip", Skip: &Gap{Kind: "missing", Detail: "reason"}}}
	if _, err := controlPlan(env, in, s); err != nil {
		t.Fatal(err)
	}
	var message string
	if err := env.DB.QueryRow(context.Background(), `SELECT message FROM run_item WHERE run_id=$1`, in.RunID).Scan(&message); err != nil {
		t.Fatal(err)
	}
	var got map[string]map[string]string
	if err := json.Unmarshal([]byte(message), &got); err != nil || got["skip"]["kind"] != "missing" || got["skip"]["detail"] != "reason" {
		t.Fatalf("message=%s err=%v", message, err)
	}
}
func TestPlanStage_ReportsCancelling(t *testing.T) {
	env, in, s := controlSetup(t)
	batchExec(t, env.DB, `UPDATE run SET state='cancelling' WHERE id=$1`, in.RunID)
	got, err := controlPlan(env, in, s)
	if err != nil || !got.Cancelling || s.calls != 0 {
		t.Fatalf("result=%+v calls=%d err=%v", got, s.calls, err)
	}
}
func TestPlanStage_RefusesRecipeMismatch(t *testing.T) {
	env, in, s := controlSetup(t)
	_, err := PlanStage(context.Background(), env, s, in, map[string]string{"other": "changed"})
	if err == nil || !strings.Contains(err.Error(), "other") || s.calls != 0 {
		t.Fatalf("calls=%d err=%v", s.calls, err)
	}
}
func TestPlanStage_ResetsFailedOnResume(t *testing.T) {
	env, in, s := controlSetup(t)
	batchExec(t, env.DB, `INSERT INTO run_item (run_id,stage,item_key,state,batch,generation) VALUES ($1,'fetch','old','failed',0,1),($1,'fetch','current','failed',1,2)`, in.RunID)
	got, err := controlPlan(env, in, s)
	if err != nil || len(got.Batches) != 1 {
		t.Fatalf("result=%+v err=%v", got, err)
	}
	var old, current string
	if err := env.DB.QueryRow(context.Background(), `SELECT (SELECT state FROM run_item WHERE run_id=$1 AND item_key='old'), (SELECT state FROM run_item WHERE run_id=$1 AND item_key='current')`, in.RunID).Scan(&old, &current); err != nil || old != "pending" || current != "failed" {
		t.Fatalf("old=%s current=%s err=%v", old, current, err)
	}
}
func TestCloseStage_EmptyStageAdvances(t *testing.T) {
	env, in, _ := controlSetup(t)
	got, err := CloseStage(context.Background(), env, in, "")
	if err != nil || got.Failed || got.NextStage != "" || got.Counts.Expected != 0 {
		t.Fatalf("result=%+v err=%v", got, err)
	}
	var next *string
	if err := env.DB.QueryRow(context.Background(), `SELECT next_stage FROM run WHERE id=$1`, in.RunID).Scan(&next); err != nil || next != nil {
		t.Fatalf("next=%v err=%v", next, err)
	}
}
func TestCloseStage_FailedItemsKeepCursor(t *testing.T) {
	env, in, _ := controlSetup(t)
	batchExec(t, env.DB, `INSERT INTO run_item (run_id,stage,item_key,state,batch) VALUES ($1,'fetch','a','failed',0),($1,'fetch','b','gap',0),($1,'fetch','c','done',0)`, in.RunID)
	batchExec(t, env.DB, `INSERT INTO gap (run_id,stage,item_key,kind,detail,accepted_at) VALUES ($1,'fetch','b','missing','a',NULL),($1,'fetch','b','missing','b',now())`, in.RunID)
	got, err := CloseStage(context.Background(), env, in, "extract")
	var next string
	queryErr := env.DB.QueryRow(context.Background(), `SELECT next_stage FROM run WHERE id=$1`, in.RunID).Scan(&next)
	if err != nil || queryErr != nil || !got.Failed || next != "fetch" || got.Counts.Expected != 3 || got.Counts.Done != 1 || got.Counts.Gap != 1 || got.Counts.Failed != 1 || got.Counts.UnacceptedGaps["missing"] != 1 {
		t.Fatalf("result=%+v next=%s err=%v query=%v", got, next, err, queryErr)
	}
}
func TestCloseStage_PendingItemsError(t *testing.T) {
	env, in, _ := controlSetup(t)
	batchExec(t, env.DB, `INSERT INTO run_item (run_id,stage,item_key,state,batch) VALUES ($1,'fetch','a','pending',0)`, in.RunID)
	_, err := CloseStage(context.Background(), env, in, "extract")
	var next string
	queryErr := env.DB.QueryRow(context.Background(), `SELECT next_stage FROM run WHERE id=$1`, in.RunID).Scan(&next)
	if err == nil || queryErr != nil || next != "fetch" {
		t.Fatalf("next=%s err=%v query=%v", next, err, queryErr)
	}
}
func TestMarkRun_OnlyFromAllowedStates(t *testing.T) {
	env, in, _ := controlSetup(t)
	batchExec(t, env.DB, `UPDATE run SET state='cancelling' WHERE id=$1`, in.RunID)
	mark := MarkInput{RunID: in.RunID, Generation: in.Generation, From: []string{"running"}, To: "paused", Message: "pause"}
	changed, err := MarkRun(context.Background(), env, mark)
	if err != nil || changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	mark.From = []string{"cancelling"}
	mark.To = "cancelled"
	changed, err = MarkRun(context.Background(), env, mark)
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
}
func TestControl_SupersededGenerationChangesNothing(t *testing.T) {
	env, in, s := controlSetup(t)
	s.items = []Item{{Key: "new"}}
	in.Generation--
	_, planErr := controlPlan(env, in, s)
	_, closeErr := CloseStage(context.Background(), env, in, "extract")
	_, markErr := MarkRun(context.Background(), env, MarkInput{RunID: in.RunID, Generation: in.Generation, From: []string{"running"}, To: "paused"})
	var count int
	var state, next string
	queryErr := env.DB.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM run_item WHERE run_id=$1),state,next_stage FROM run WHERE id=$1`, in.RunID).Scan(&count, &state, &next)
	if !errors.Is(planErr, ErrSuperseded) || !errors.Is(closeErr, ErrSuperseded) || !errors.Is(markErr, ErrSuperseded) || queryErr != nil || s.calls != 0 || count != 0 || state != "running" || next != "fetch" {
		t.Fatalf("plan=%v close=%v mark=%v query=%v calls=%d count=%d state=%s next=%s", planErr, closeErr, markErr, queryErr, s.calls, count, state, next)
	}
}
