//go:build integration

package stage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"danny.vn/traquydinh/apps/corpus/internal/artifact"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type batchStage struct {
	process func(context.Context, Env, Run, Item) (Output, error)
	calls   int
}

func (*batchStage) Name() string                                   { return "fetch" }
func (*batchStage) Recipe(Snapshot) string                         { return strings.Repeat("b", 64) }
func (*batchStage) Plan(context.Context, Env, Run) ([]Item, error) { return nil, nil }
func (s *batchStage) Process(ctx context.Context, env Env, r Run, it Item) (Output, error) {
	s.calls++
	if s.process != nil {
		return s.process(ctx, env, r, it)
	}
	return Output{State: "gap", Gaps: []Gap{{Kind: "missing", Detail: it.Key}}}, nil
}
func (*batchStage) Rows(_ context.Context, env Env, art artifact.Result) (func(context.Context, pgx.Tx, int64) error, error) {
	data, err := os.ReadFile(filepath.Join(env.Artifacts.Abs(art.Path), "row"))
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context, tx pgx.Tx, id int64) error {
		_, execErr := tx.Exec(ctx, `INSERT INTO fetched_file (artifact_id, name, sha256, size, path) VALUES ($1, $2, 'hash', 1, 'path')`, id, string(data))
		return execErr
	}, nil
}
func (*batchStage) Resolve(_ context.Context, _ Env, _ Run, it Item, _ artifact.Result) (Resolution, error) {
	return Resolution{Gaps: []Gap{{Kind: "resolved", Detail: it.Key}}}, nil
}

func batchSetup(t *testing.T) (Env, int64, *batchStage) {
	t.Helper()
	pool, id := setup(t)
	return Env{DB: pool, Artifacts: artifact.Store{Root: t.TempDir()}}, id, &batchStage{}
}
func batchRun(env Env, id int64, s *batchStage, attempt int, heartbeat func(...any)) (BatchResult, error) {
	return RunBatch(context.Background(), env, s, BatchInput{RunID: id, Generation: 2, Stage: "fetch", Batch: 0}, attempt, BatchConfig{HeartbeatEvery: 50 * time.Millisecond, MaxAttempts: 5}, heartbeat)
}
func batchExec(t *testing.T, pool *pgxpool.Pool, query string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), query, args...); err != nil {
		t.Fatal(err)
	}
}
func batchState(t *testing.T, pool *pgxpool.Pool, id int64) (string, int) {
	t.Helper()
	var state string
	var gaps int
	if err := pool.QueryRow(context.Background(), `SELECT state, (SELECT count(*) FROM gap WHERE run_id=$1) FROM run_item WHERE run_id=$1 AND item_key='item'`, id).Scan(&state, &gaps); err != nil {
		t.Fatal(err)
	}
	return state, gaps
}
func publishBatchArtifact(t *testing.T, env Env, s *batchStage) string {
	t.Helper()
	hash := strings.Repeat("a", 64)
	_, err := env.Artifacts.Publish(context.Background(), "fetch", hash, s.Recipe(Snapshot{}), func(dir string) error { return os.WriteFile(filepath.Join(dir, "row"), []byte("from artifact"), 0600) })
	if err != nil {
		t.Fatal(err)
	}
	return hash
}

func TestRunBatch_SkipsDoneItems(t *testing.T) {
	t.Parallel()
	env, id, s := batchSetup(t)
	batchExec(t, env.DB, `UPDATE run_item SET state='done' WHERE run_id=$1`, id)
	result, err := batchRun(env, id, s, 1, func(...any) {})
	if err != nil || s.calls != 0 || result != (BatchResult{}) {
		t.Fatalf("result=%+v calls=%d err=%v", result, s.calls, err)
	}
}
func TestRunBatch_ReusesExistingArtifactAndResolves(t *testing.T) {
	t.Parallel()
	env, id, s := batchSetup(t)
	hash := publishBatchArtifact(t, env, s)
	batchExec(t, env.DB, `UPDATE run_item SET input_hash=$2 WHERE run_id=$1`, id, hash)
	batchExec(t, env.DB, `INSERT INTO artifact (stage,item_key,input_hash,recipe_hash,path,sha256) VALUES ('fetch','other',$1,$2,'existing','hash')`, hash, s.Recipe(Snapshot{}))
	result, err := batchRun(env, id, s, 1, func(...any) {})
	state, gaps := batchState(t, env.DB, id)
	if err != nil || s.calls != 0 || result.Done != 1 || state != "done" || gaps != 1 {
		t.Fatalf("result=%+v state=%s gaps=%d calls=%d err=%v", result, state, gaps, s.calls, err)
	}
}
func TestRunBatch_AdoptedDirWithoutRowLoadsRows(t *testing.T) {
	t.Parallel()
	env, id, s := batchSetup(t)
	hash := publishBatchArtifact(t, env, s)
	batchExec(t, env.DB, `UPDATE run_item SET input_hash=$2 WHERE run_id=$1`, id, hash)
	result, err := batchRun(env, id, s, 1, func(...any) {})
	var name string
	queryErr := env.DB.QueryRow(context.Background(), `SELECT name FROM fetched_file`).Scan(&name)
	if err != nil || queryErr != nil || result.Done != 1 || s.calls != 0 || name != "from artifact" {
		t.Fatalf("result=%+v name=%q calls=%d err=%v query=%v", result, name, s.calls, err, queryErr)
	}
}
func TestRunBatch_SkipItemCommitsGap(t *testing.T) {
	t.Parallel()
	env, id, s := batchSetup(t)
	batchExec(t, env.DB, `UPDATE run_item SET message='{"skip":{"kind":"scope_removed","detail":"reason"}}' WHERE run_id=$1`, id)
	result, err := batchRun(env, id, s, 1, func(...any) {})
	var kind, detail string
	queryErr := env.DB.QueryRow(context.Background(), `SELECT kind, detail FROM gap WHERE run_id=$1`, id).Scan(&kind, &detail)
	state, _ := batchState(t, env.DB, id)
	if err != nil || queryErr != nil || result.Gap != 1 || s.calls != 0 || state != "gap" || kind != "scope_removed" || detail != "reason" {
		t.Fatalf("result=%+v state=%s gap=%s/%s calls=%d err=%v query=%v", result, state, kind, detail, s.calls, err, queryErr)
	}
}
func TestRunBatch_RetryableFailureErrorsUntilLastAttempt(t *testing.T) {
	t.Parallel()
	env, id, s := batchSetup(t)
	s.process = func(context.Context, Env, Run, Item) (Output, error) {
		return Output{}, Retryable(errors.New("temporary"))
	}
	result, err := batchRun(env, id, s, 1, func(...any) {})
	state, _ := batchState(t, env.DB, id)
	if err == nil || result.Pending != 1 || state != "pending" {
		t.Fatalf("result=%+v state=%s err=%v", result, state, err)
	}
	result, err = batchRun(env, id, s, 5, func(...any) {})
	state, _ = batchState(t, env.DB, id)
	if err != nil || result.Failed != 1 || result.Pending != 0 || state != "failed" {
		t.Fatalf("result=%+v state=%s err=%v", result, state, err)
	}
}
func TestRunBatch_HeartbeatsDuringSlowItem(t *testing.T) {
	t.Parallel()
	env, id, s := batchSetup(t)
	s.process = func(context.Context, Env, Run, Item) (Output, error) {
		time.Sleep(300 * time.Millisecond)
		return Output{State: "gap"}, nil
	}
	var beats atomic.Int32
	_, err := batchRun(env, id, s, 1, func(...any) { beats.Add(1) })
	if err != nil || beats.Load() < 3 {
		t.Fatalf("heartbeats=%d err=%v", beats.Load(), err)
	}
}
func TestRunBatch_StopsWhenCancelling(t *testing.T) {
	t.Parallel()
	env, id, s := batchSetup(t)
	batchExec(t, env.DB, `INSERT INTO run_item (run_id, stage, item_key, state, batch) VALUES ($1, 'fetch', 'later', 'pending', 0)`, id)
	s.process = func(ctx context.Context, env Env, _ Run, _ Item) (Output, error) {
		_, err := env.DB.Exec(ctx, `UPDATE run SET state='cancelling' WHERE id=$1`, id)
		return Output{State: "gap"}, err
	}
	result, err := batchRun(env, id, s, 1, func(...any) {})
	if err != nil || !result.Cancelling || result.Gap != 1 || s.calls != 1 {
		t.Fatalf("result=%+v calls=%d err=%v", result, s.calls, err)
	}
}
func TestRunBatch_SupersededGenerationWritesNothing(t *testing.T) {
	t.Parallel()
	env, id, s := batchSetup(t)
	batchExec(t, env.DB, `UPDATE run SET generation=3 WHERE id=$1`, id)
	_, err := batchRun(env, id, s, 1, func(...any) {})
	state, gaps := batchState(t, env.DB, id)
	if !errors.Is(err, ErrSuperseded) || s.calls != 0 || state != "pending" || gaps != 0 {
		t.Fatalf("state=%s gaps=%d calls=%d err=%v", state, gaps, s.calls, err)
	}
}

func TestRunBatch_RowsErrorFailsOnlyThatItem(t *testing.T) {
	t.Parallel()
	env, id, s := batchSetup(t)
	s.process = func(_ context.Context, _ Env, _ Run, item Item) (Output, error) {
		if item.Key == "item" {
			return Output{Artifact: &artifact.Result{Path: filepath.Join(t.TempDir(), "missing"), InputHash: "input"}}, nil
		}
		return Output{State: "gap"}, nil
	}
	batchExec(t, env.DB, `INSERT INTO run_item (run_id,stage,item_key,state,batch) VALUES ($1,'fetch','later','pending',0)`, id)
	result, err := batchRun(env, id, s, 1, func(...any) {})
	state, _ := batchState(t, env.DB, id)
	if err != nil || state != "failed" || result.Failed != 1 || result.Gap != 1 {
		t.Fatalf("result=%+v state=%s err=%v", result, state, err)
	}
}

func TestRunBatch_CancelledStopsBatch(t *testing.T) {
	t.Parallel()
	env, id, s := batchSetup(t)
	batchExec(t, env.DB, `UPDATE run SET state='cancelled' WHERE id=$1`, id)
	result, err := batchRun(env, id, s, 1, func(...any) {})
	if err != nil || !result.Cancelling || s.calls != 0 {
		t.Fatalf("result=%+v calls=%d err=%v", result, s.calls, err)
	}
}

func TestRunBatch_SQLDataErrorFailsOnlyThatItem(t *testing.T) {
	t.Parallel()
	env, id, s := batchSetup(t)
	hash := strings.Repeat("a", 64)
	_, err := env.Artifacts.Publish(context.Background(), "fetch", hash, s.Recipe(Snapshot{}), func(dir string) error { return os.WriteFile(filepath.Join(dir, "row"), []byte("bad\x00text"), 0600) })
	if err != nil {
		t.Fatal(err)
	}
	batchExec(t, env.DB, `UPDATE run_item SET input_hash=$2 WHERE run_id=$1`, id, hash)
	batchExec(t, env.DB, `INSERT INTO run_item (run_id,stage,item_key,state,batch) VALUES ($1,'fetch','later','pending',0)`, id)
	result, err := batchRun(env, id, s, 1, func(...any) {})
	var state, message string
	var artifacts int
	queryErr := env.DB.QueryRow(context.Background(), `SELECT state, message, (SELECT count(*) FROM artifact) FROM run_item WHERE run_id=$1 AND item_key='item'`, id).Scan(&state, &message, &artifacts)
	if err != nil || queryErr != nil || state != "failed" || message == "" || artifacts != 0 || result.Failed != 1 || result.Gap != 1 {
		t.Fatalf("result=%+v state=%s message=%q artifacts=%d err=%v query=%v", result, state, message, artifacts, err, queryErr)
	}
}
