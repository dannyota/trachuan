package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
)

// Checkpointer saves a stage boundary.
type Checkpointer interface {
	Take(ctx context.Context, runKey string, generation int, stage string) (string, error)
}

// NoCheckpoint skips checkpoint creation.
type NoCheckpoint struct{}

// Take returns an empty checkpoint path.
func (NoCheckpoint) Take(context.Context, string, int, string) (string, error) { return "", nil }

// Activities provides the stage and control activities.
type Activities struct {
	Env          stage.Env
	Registry     Registry
	Checkpointer Checkpointer
	Batch        stage.BatchConfig
}

func activityError(err error) error {
	if errors.Is(err, stage.ErrSuperseded) {
		return temporal.NewNonRetryableApplicationError("superseded", "superseded", err)
	}
	var recipe *stage.RecipeError
	if errors.As(err, &recipe) {
		return temporal.NewNonRetryableApplicationError(recipe.Error(), "recipe_mismatch", err)
	}
	return err
}

func (a *Activities) load(ctx context.Context, id int64, generation int) (store.Run, error) {
	row, err := store.New(a.Env.DB).GetBatchRun(ctx, id)
	if err != nil {
		return row, fmt.Errorf("load run: %w", err)
	}
	if int(row.Generation) != generation {
		return row, stage.ErrSuperseded
	}
	return row, nil
}

func (a *Activities) lookup(name string) (stage.Stage, error) {
	st, ok := a.Registry.Lookup(name)
	if !ok {
		return nil, fmt.Errorf("unknown stage %q", name)
	}
	return st, nil
}

// PlanStage plans pending batches for the stage.
func (a *Activities) PlanStage(ctx context.Context, in stage.StageInput) (stage.PlanResult, error) {
	row, err := a.load(ctx, in.RunID, in.Generation)
	if err != nil {
		return stage.PlanResult{}, activityError(err)
	}
	var snapshot stage.Snapshot
	if err = json.Unmarshal(row.Snapshot, &snapshot); err != nil {
		return stage.PlanResult{}, fmt.Errorf("decode run snapshot: %w", err)
	}
	st, err := a.lookup(in.Stage)
	if err != nil {
		return stage.PlanResult{}, err
	}
	result, err := stage.PlanStage(ctx, a.Env, st, in, a.Registry.Recipes(snapshot))
	return result, activityError(err)
}

// RunBatch processes one stage batch.
func (a *Activities) RunBatch(ctx context.Context, in stage.BatchInput) (stage.BatchResult, error) {
	if _, err := a.load(ctx, in.RunID, in.Generation); err != nil {
		return stage.BatchResult{}, activityError(err)
	}
	st, err := a.lookup(in.Stage)
	if err != nil {
		return stage.BatchResult{}, err
	}
	result, err := stage.RunBatch(ctx, a.Env, st, in, int(activity.GetInfo(ctx).Attempt), a.Batch, func(details ...any) { activity.RecordHeartbeat(ctx, details...) })
	return result, activityError(err)
}

// CloseStage computes counts and advances a completed stage.
func (a *Activities) CloseStage(ctx context.Context, in stage.StageInput) (stage.CloseResult, error) {
	row, err := a.load(ctx, in.RunID, in.Generation)
	if err != nil {
		return stage.CloseResult{}, activityError(err)
	}
	next, err := frozenFollowing(row.Snapshot, in.Stage)
	if err != nil {
		return stage.CloseResult{}, err
	}
	st, err := a.lookup(in.Stage)
	if err != nil {
		return stage.CloseResult{}, err
	}
	result, err := stage.CloseStage(ctx, a.Env, st, in, next)
	return result, activityError(err)
}

// Checkpoint saves the workshop at a stage boundary.
func (a *Activities) Checkpoint(ctx context.Context, in stage.StageInput) (string, error) {
	row, err := a.load(ctx, in.RunID, in.Generation)
	if err != nil {
		return "", activityError(err)
	}
	result, err := a.Checkpointer.Take(ctx, row.Country+"-"+row.Rel, in.Generation, in.Stage)
	return result, activityError(err)
}

// MarkRun applies a fenced run state transition.
func (a *Activities) MarkRun(ctx context.Context, in stage.MarkInput) (bool, error) {
	result, err := stage.MarkRun(ctx, a.Env, in)
	return result, activityError(err)
}

func frozenFollowing(raw json.RawMessage, current string) (string, error) {
	var snapshot stage.Snapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return "", fmt.Errorf("decode run snapshot: %w", err)
	}
	for i, name := range snapshot.Stages {
		if name == current {
			if i+1 < len(snapshot.Stages) {
				return snapshot.Stages[i+1], nil
			}
			return "", nil
		}
	}
	return "", fmt.Errorf("stage %q is not in run snapshot", current)
}
