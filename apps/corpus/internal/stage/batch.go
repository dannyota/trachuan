package stage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/dannyota/traquydinh/apps/corpus/internal/artifact"
	"github.com/dannyota/traquydinh/apps/corpus/internal/fault"
	"github.com/dannyota/traquydinh/apps/corpus/internal/store"
)

// BatchInput identifies one batch of one stage.
type BatchInput struct {
	RunID      int64
	Generation int
	Stage      string
	Batch      int
}

// BatchResult counts the batch's items by outcome.
type BatchResult struct {
	Done, Gap, Failed, Pending int
	Cancelling                 bool
}

// BatchConfig bounds a batch run.
type BatchConfig struct {
	HeartbeatEvery time.Duration
	MaxAttempts    int
}

// ErrSuperseded reports that the run's generation moved past the caller's.
var ErrSuperseded = errors.New("superseded")

// RunBatch processes the pending items of one batch.
func RunBatch(ctx context.Context, env Env, st Stage, in BatchInput, attempt int, cfg BatchConfig, heartbeat func(details ...any)) (BatchResult, error) {
	var result BatchResult
	q := store.New(env.DB)
	runRow, err := q.GetBatchRun(ctx, in.RunID)
	if err != nil {
		return result, fmt.Errorf("load batch run: %w", err)
	}
	if int(runRow.Generation) != in.Generation {
		return result, ErrSuperseded
	}
	var snapshot Snapshot
	if err = json.Unmarshal(runRow.Snapshot, &snapshot); err != nil {
		return result, fmt.Errorf("decode run snapshot: %w", err)
	}
	run := Run{ID: runRow.ID, Country: runRow.Country, Rel: runRow.Rel, Generation: int(runRow.Generation), Snapshot: snapshot}
	recipe := st.Recipe(snapshot)
	if cfg.HeartbeatEvery <= 0 {
		return result, fmt.Errorf("heartbeat interval must be positive")
	}
	var last atomic.Int64
	last.Store(-1)
	stop := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		ticker := time.NewTicker(cfg.HeartbeatEvery)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				heartbeat(int(last.Load()))
			case <-stop:
				return
			}
		}
	}()
	defer func() { close(stop); <-stopped }()
	items, err := q.ListPendingBatchItems(ctx, store.ListPendingBatchItemsParams{RunID: in.RunID, Stage: in.Stage, Batch: int32(in.Batch)})
	if err != nil {
		return result, fmt.Errorf("list pending batch items: %w", err)
	}
	for i, itemRow := range items {
		state, stateErr := q.GetBatchRunState(ctx, in.RunID)
		if stateErr != nil {
			return result, fmt.Errorf("load run state: %w", stateErr)
		}
		if state == "cancelling" {
			result.Cancelling = true
			return result, nil
		}
		item := Item{Key: itemRow.ItemKey}
		if itemRow.InputHash != nil {
			item.InputHash = *itemRow.InputHash
		}
		if itemRow.Message != nil {
			var planned struct {
				Skip *Gap `json:"skip"`
			}
			if json.Unmarshal([]byte(*itemRow.Message), &planned) == nil {
				item.Skip = planned.Skip
			}
		}
		commit := Commit{RunID: in.RunID, Stage: in.Stage, ItemKey: item.Key, Generation: in.Generation, Attempt: attempt, RecipeHash: recipe}
		if item.Skip != nil {
			commit.State, commit.Gaps = "gap", []Gap{*item.Skip}
		} else {
			var output Output
			var found bool
			var art artifact.Result
			if item.InputHash != "" {
				art, found, err = env.Artifacts.Open(in.Stage, item.InputHash, recipe)
				if err != nil {
					return result, fmt.Errorf("open artifact for %q: %w", item.Key, err)
				}
			}
			if found {
				output = Output{Artifact: &art, State: "done"}
			} else {
				output, err = st.Process(ctx, env, run, item)
				if err != nil {
					var retryable *RetryableError
					if errors.As(err, &retryable) && attempt < cfg.MaxAttempts {
						result.Pending++
						last.Store(int64(i))
						continue
					}
					output = Output{State: "failed", Message: err.Error()}
				} else if output.Artifact != nil {
					fault.Hit("after_rename", i)
				}
			}
			commit.State, commit.Message, commit.Gaps, commit.Artifact = output.State, output.Message, output.Gaps, output.Artifact
			if output.Artifact != nil {
				if commit.State == "" {
					commit.State = "done"
				}
				commit.ArtifactRows, err = st.Rows(ctx, *output.Artifact)
				if err != nil {
					return result, fmt.Errorf("load artifact rows for %q: %w", item.Key, err)
				}
				var resolution Resolution
				resolution, err = st.Resolve(ctx, env, run, item, *output.Artifact)
				if err != nil {
					return result, fmt.Errorf("resolve artifact for %q: %w", item.Key, err)
				}
				commit.RunRows = resolution.RunRows
				commit.Gaps = append(commit.Gaps, resolution.Gaps...)
			}
		}
		applied, commitErr := CommitItem(ctx, env.DB, commit)
		if commitErr != nil {
			return result, fmt.Errorf("commit item %q: %w", item.Key, commitErr)
		}
		if !applied {
			return result, ErrSuperseded
		}
		fault.Hit("after_commit", i)
		switch commit.State {
		case "done":
			result.Done++
		case "gap":
			result.Gap++
		case "failed":
			result.Failed++
		}
		last.Store(int64(i))
	}
	if result.Pending > 0 {
		return result, fmt.Errorf("%d items pending after retryable errors", result.Pending)
	}
	return result, nil
}
