//go:build integration

package workflow

import (
	"context"
	"errors"
	"testing"

	"danny.vn/traquydinh/apps/corpus/internal/stage"
	"danny.vn/traquydinh/apps/corpus/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.temporal.io/sdk/temporal"
)

type recordingCheckpoint struct{ calls int }

func (c *recordingCheckpoint) Take(context.Context, string, int, string) (string, error) {
	c.calls++
	return "", nil
}

func TestActivities_SupersededGenerationChangesNothing(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	url := testdb.New(t)
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var id int64
	if err := pool.QueryRow(ctx, `INSERT INTO run (country,rel,state,generation,snapshot,next_stage) VALUES ('vn','20261109','running',2,'{}','fetch') RETURNING id`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	checkpoint := &recordingCheckpoint{}
	a := Activities{Env: stage.Env{DB: pool}, Checkpointer: checkpoint}
	in := stage.StageInput{RunID: id, Generation: 1, Stage: "fetch"}
	_, planErr := a.PlanStage(ctx, in)
	_, batchErr := a.RunBatch(ctx, stage.BatchInput{RunID: id, Generation: 1, Stage: "fetch"})
	_, closeErr := a.CloseStage(ctx, in)
	_, checkpointErr := a.Checkpoint(ctx, in)
	_, markErr := a.MarkRun(ctx, stage.MarkInput{RunID: id, Generation: 1, From: []string{"running"}, To: "failed"})
	for name, activityErr := range map[string]error{"plan": planErr, "batch": batchErr, "close": closeErr, "checkpoint": checkpointErr, "mark": markErr} {
		var appErr *temporal.ApplicationError
		if !errors.As(activityErr, &appErr) || appErr.Type() != "superseded" || !appErr.NonRetryable() {
			t.Errorf("%s: %v", name, activityErr)
		}
	}
	var state, next string
	var count int
	if err := pool.QueryRow(ctx, `SELECT state,next_stage,(SELECT count(*) FROM run_item WHERE run_id=$1) FROM run WHERE id=$1`, id).Scan(&state, &next, &count); err != nil {
		t.Fatal(err)
	}
	if state != "running" || next != "fetch" || count != 0 || checkpoint.calls != 0 {
		t.Fatalf("state=%s next=%s items=%d checkpoints=%d", state, next, count, checkpoint.calls)
	}
}
