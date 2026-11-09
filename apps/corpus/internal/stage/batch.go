package stage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync/atomic"
	"time"

	"github.com/dannyota/traquydinh/apps/corpus/internal/artifact"
	"github.com/dannyota/traquydinh/apps/corpus/internal/fault"
	"github.com/dannyota/traquydinh/apps/corpus/internal/store"
	"github.com/jackc/pgx/v5/pgconn"
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
		err = nil
		state, stateErr := q.GetBatchRunState(ctx, in.RunID)
		if stateErr != nil {
			return result, fmt.Errorf("load run state: %w", stateErr)
		}
		if state == "cancelling" || state == "cancelled" {
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
					if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, ErrSuperseded) {
						return result, err
					}
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
					if !itemError(err) {
						return result, fmt.Errorf("load artifact rows for %q: %w", item.Key, err)
					}
					commit = failedCommit(commit, err)
				}
				var resolution Resolution
				if err == nil {
					resolution, err = st.Resolve(ctx, env, run, item, *output.Artifact)
					if err != nil {
						if !itemError(err) {
							return result, fmt.Errorf("resolve artifact for %q: %w", item.Key, err)
						}
						commit = failedCommit(commit, err)
					} else {
						commit.RunRows = resolution.RunRows
						commit.Gaps = append(commit.Gaps, resolution.Gaps...)
					}
				}
			}
		}
		applied, commitErr := CommitItem(ctx, env.DB, commit)
		if commitErr != nil {
			var pgErr *pgconn.PgError
			if !errors.As(commitErr, &pgErr) || !dataError(pgErr) {
				return result, fmt.Errorf("commit item %q: %w", item.Key, commitErr)
			}
			commit = failedCommit(commit, commitErr)
			applied, commitErr = CommitItem(ctx, env.DB, commit)
			if commitErr != nil {
				return result, fmt.Errorf("commit failed item %q: %w", item.Key, commitErr)
			}
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

// itemError reports whether err belongs to one item (its data or its files)
// rather than to the infrastructure the whole batch shares.
func itemError(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, ErrSuperseded) {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return dataError(pgErr)
	}
	// Match concrete network and connection errors only: syscall.Errno also
	// satisfies net.Error, so a missing local file would look like a network fault.
	var opErr *net.OpError
	var connErr *pgconn.ConnectError
	return !errors.As(err, &opErr) && !errors.As(err, &connErr)
}

// dataError reports PostgreSQL data exceptions (class 22) and integrity
// violations (class 23), which one item's content causes.
func dataError(e *pgconn.PgError) bool {
	return strings.HasPrefix(e.Code, "22") || strings.HasPrefix(e.Code, "23")
}

func failedCommit(c Commit, err error) Commit {
	c.Artifact, c.ArtifactRows, c.RunRows, c.Gaps = nil, nil, nil, nil
	c.State = "failed"
	message := []rune(strings.ReplaceAll(err.Error(), "\x00", ""))
	if len(message) > 500 {
		message = message[:500]
	}
	c.Message = string(message)
	return c
}
