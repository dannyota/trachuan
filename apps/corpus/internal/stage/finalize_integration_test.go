//go:build integration

package stage

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5"
)

type testFinalizer struct {
	*controlStage
	result        Finalization
	err           error
	calls         int
	artifactsRoot string
}

func (s *testFinalizer) Finalize(ctx context.Context, env Env, tx pgx.Tx, run Run) (Finalization, error) {
	s.calls++
	s.artifactsRoot = env.Artifacts.Root
	_, err := tx.Exec(ctx, `UPDATE run SET message='finalized' WHERE id=$1`, run.ID)
	if err != nil {
		return Finalization{}, err
	}
	return s.result, s.err
}

var _ Finalizer = (*testFinalizer)(nil)

func TestFinalize_FencedAndIdempotent(t *testing.T) {
	env, in, base := controlSetup(t)
	env.Artifacts.Root = t.TempDir()
	st := &testFinalizer{controlStage: base}
	batchExec(t, env.DB, `INSERT INTO run_item (run_id,stage,item_key,state,batch) VALUES ($1,'fetch','done','done',0)`, in.RunID)
	stale := in
	stale.Generation--
	if _, err := CloseStage(context.Background(), env, st, stale, "extract"); !errors.Is(err, ErrSuperseded) || st.calls != 0 {
		t.Fatalf("stale err=%v calls=%d", err, st.calls)
	}
	first, err := CloseStage(context.Background(), env, st, in, "extract")
	if err != nil || first.NextStage != "extract" || st.artifactsRoot != env.Artifacts.Root {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	second, err := CloseStage(context.Background(), env, st, in, "extract")
	if err != nil || !reflect.DeepEqual(second, first) || st.calls != 1 {
		t.Fatalf("second=%+v calls=%d err=%v", second, st.calls, err)
	}
}

func TestFinalize_DomainFailureCommitsReason(t *testing.T) {
	env, in, base := controlSetup(t)
	st := &testFinalizer{controlStage: base, result: Finalization{Code: "scope_changed", Reason: "scope drift", ItemKey: "item", EvidenceRefs: []string{"evidence"}}}
	got, err := CloseStage(context.Background(), env, st, in, "extract")
	if err != nil || !got.Failed || got.Reason != "scope drift" {
		t.Fatalf("result=%+v err=%v", got, err)
	}
	again, err := CloseStage(context.Background(), env, st, in, "extract")
	if err != nil || !again.Failed || again.Reason != got.Reason || st.calls != 1 {
		t.Fatalf("again=%+v calls=%d err=%v", again, st.calls, err)
	}
	var state, next, message, reason string
	var count int
	err = env.DB.QueryRow(context.Background(), `SELECT state,next_stage,message,(SELECT count(*) FROM run_failure WHERE run_id=$1),(SELECT reason FROM run_failure WHERE run_id=$1) FROM run WHERE id=$1`, in.RunID).Scan(&state, &next, &message, &count, &reason)
	if err != nil || state != "failed" || next != "fetch" || message != "scope drift" || count != 1 || reason != message {
		t.Fatalf("state=%s next=%s message=%s count=%d reason=%s err=%v", state, next, message, count, reason, err)
	}
}

func TestFinalize_InfraErrorRollsBack(t *testing.T) {
	env, in, base := controlSetup(t)
	failure := errors.New("infrastructure")
	st := &testFinalizer{controlStage: base, err: failure}
	_, err := CloseStage(context.Background(), env, st, in, "extract")
	if !errors.Is(err, failure) {
		t.Fatalf("err=%v", err)
	}
	var message *string
	var next string
	if err = env.DB.QueryRow(context.Background(), `SELECT message,next_stage FROM run WHERE id=$1`, in.RunID).Scan(&message, &next); err != nil || message != nil || next != "fetch" {
		t.Fatalf("message=%v next=%s err=%v", message, next, err)
	}
}

func TestCloseStage_WithoutFinalizerUnchanged(t *testing.T) {
	env, in, st := controlSetup(t)
	got, err := CloseStage(context.Background(), env, st, in, "extract")
	if err != nil || got.Failed || got.NextStage != "extract" {
		t.Fatalf("result=%+v err=%v", got, err)
	}
}

func TestCloseStage_CancellingSkipsFinalizer(t *testing.T) {
	for _, state := range []string{"cancelling", "cancelled"} {
		t.Run(state, func(t *testing.T) {
			env, in, base := controlSetup(t)
			batchExec(t, env.DB, `UPDATE run SET state=$2 WHERE id=$1`, in.RunID, state)
			st := &testFinalizer{controlStage: base}
			got, err := CloseStage(context.Background(), env, st, in, "extract")
			if err != nil || !got.Cancelling || st.calls != 0 {
				t.Fatalf("result=%+v calls=%d err=%v", got, st.calls, err)
			}
			var gotState, next string
			var message *string
			err = env.DB.QueryRow(context.Background(), `SELECT state,next_stage,message FROM run WHERE id=$1`, in.RunID).Scan(&gotState, &next, &message)
			if err != nil || gotState != state || next != "fetch" || message != nil {
				t.Fatalf("state=%s next=%s message=%v err=%v", gotState, next, message, err)
			}
		})
	}
}
