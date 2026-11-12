package stage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync/atomic"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/fault"
	"danny.vn/trachuan/apps/corpus/internal/sourcegate"
	"danny.vn/trachuan/apps/corpus/internal/store"
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
	HeartbeatEvery   time.Duration
	HeartbeatTimeout time.Duration
	MaxAttempts      int
}

// ErrSuperseded reports that the run's generation moved past the caller's.
var ErrSuperseded = errors.New("superseded")

// RunBatch processes the pending items of one batch.
func RunBatch(ctx context.Context, env Env, st Stage, in BatchInput, attempt int, cfg BatchConfig, heartbeat func(details ...any)) (BatchResult, error) {
	ctx = sourcegate.WithActivity(ctx)
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
	if err = checkRecipe(snapshot, in.Stage, recipe); err != nil {
		return result, err
	}
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
		lastTick := time.Now()
		timeout := cfg.HeartbeatTimeout
		if timeout <= 0 {
			timeout = 2 * time.Minute
		}
		for {
			select {
			case <-ticker.C:
				started := time.Now()
				gap := started.Sub(lastTick)
				lastTick = started
				heartbeat(int(last.Load()))
				duration := time.Since(started)
				if gap > timeout/2 || duration > timeout/2 {
					slog.Warn("activity heartbeat delayed", "activity", in.Stage, "batch", in.Batch, "gap", gap, "call_duration", duration)
				}
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
			output, acquired, adoptErr := adoptAcquisition(ctx, env, st, run, item)
			if adoptErr != nil {
				return result, fmt.Errorf("adopt acquisition for %q: %w", item.Key, adoptErr)
			}
			found := acquired
			var art artifact.Result
			_, acquisitionCapable := st.(acquisitionVerifier)
			if !acquired && item.InputHash != "" && !acquisitionCapable {
				art, found, err = env.Artifacts.Open(in.Stage, item.InputHash, recipe)
				if err != nil {
					return result, fmt.Errorf("open artifact for %q: %w", item.Key, err)
				}
			}
			if found && !acquired {
				output = Output{Artifact: &art, State: "done"}
			} else if !found {
				itemCtx := sourcegate.WithItem(sourcegate.WithRunID(ctx, in.RunID), item.Key)
				output, err = st.Process(itemCtx, env, run, item)
				commit.SourceRead = err == nil
				if err != nil {
					// Only the activity's own context ends the batch; a timeout
					// inside one item stays with that item.
					if ctx.Err() != nil || errors.Is(err, ErrSuperseded) {
						return result, err
					}
					var retryable *RetryableError
					if !errors.Is(err, sourcegate.ErrSourcePaused) && errors.As(err, &retryable) && attempt < cfg.MaxAttempts && !repeatedSourceTimeout(in.Stage, attempt, retryable) {
						result.Pending++
						last.Store(int64(i))
						continue
					}
					sources := output.Sources
					output = processFailure(err, retryable)
					output.Sources = sources
				} else if output.Artifact != nil {
					fault.Hit("after_rename", i)
				}
			}
			commit.State, commit.Message, commit.Gaps, commit.Artifact, commit.Sources, commit.Observations = output.State, output.Message, output.Gaps, output.Artifact, output.Sources, output.Observations
			if output.Artifact != nil {
				if commit.State == "" {
					commit.State = "done"
				}
				commit.ArtifactRows, err = st.Rows(ctx, env, *output.Artifact)
				if err != nil {
					if !itemError(ctx, err) {
						return result, fmt.Errorf("load artifact rows for %q: %w", item.Key, err)
					}
					commit = itemCommit(commit, err)
				}
				var resolution Resolution
				if err == nil {
					resolution, err = st.Resolve(ctx, env, run, item, *output.Artifact)
					if err != nil {
						if !itemError(ctx, err) {
							return result, fmt.Errorf("resolve artifact for %q: %w", item.Key, err)
						}
						commit = itemCommit(commit, err)
					} else {
						commit.RunRows = resolution.RunRows
						commit.Gaps = append(commit.Gaps, resolution.Gaps...)
					}
				}
			}
		}
		applied, commitErr := commitItem(ctx, env.DB, &commit)
		if commitErr != nil {
			var pgErr *pgconn.PgError
			if !errors.As(commitErr, &pgErr) || !dataError(pgErr) {
				return result, fmt.Errorf("commit item %q: %w", item.Key, commitErr)
			}
			commit = itemCommit(commit, commitErr)
			applied, commitErr = commitItem(ctx, env.DB, &commit)
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
// rather than to the infrastructure the whole batch shares. Only the activity's
// own context ends the batch; a timeout inside one item stays with that item.
func itemError(ctx context.Context, err error) bool {
	if ctx.Err() != nil || errors.Is(err, ErrSuperseded) {
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

// processFailure turns a final Process error into an item outcome: a retryable
// error with a gap kind becomes that gap, source data becomes gap
// invalid_source_data, anything else a failed item.
func processFailure(err error, retryable *RetryableError) Output {
	c := itemCommit(Commit{}, err)
	if errors.Is(err, sourcegate.ErrSourcePaused) {
		return Output{State: c.State, Message: c.Message}
	}
	if retryable != nil && retryable.GapKind != "" {
		c.State, c.Gaps = "gap", []Gap{{Kind: retryable.GapKind, Detail: trimRunes(c.Message, 200)}}
		cause := retryable.Cause
		if cause.Origin == "" && sourceDataError(err) && retryable.GapKind == "invalid_source_data" {
			cause.Origin = OriginSourceData
		}
		c.Gaps[0].setCause(cause)
	}
	if retryable != nil && retryable.GapKind == "" && len(c.Gaps) > 0 {
		cause := retryable.Cause
		if cause.Origin == "" {
			cause.Origin = OriginSourceData
		}
		c.Gaps[0].setCause(cause)
	}
	return Output{State: c.State, Message: c.Message, Gaps: c.Gaps}
}

func trimRunes(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		r = r[:n]
	}
	return string(r)
}

// sourceDataError reports errors caused by the source document's content
// (broken payload JSON, text PostgreSQL rejects, duplicate ids), which a retry
// cannot fix and the owner may accept as a gap.
func sourceDataError(err error) bool {
	var syntax *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	var pgErr *pgconn.PgError
	return errors.Is(err, ErrSourceData) || errors.As(err, &syntax) || errors.As(err, &typeErr) || (errors.As(err, &pgErr) && dataError(pgErr))
}

// itemCommit records an item-level error: source data errors become gap
// invalid_source_data, anything else a failed item a later attempt may fix.
func itemCommit(c Commit, err error) Commit {
	if errors.Is(err, sourcegate.ErrSourcePaused) {
		return failedCommit(c, sourcegate.ErrSourcePaused)
	}
	if !sourceDataError(err) {
		return failedCommit(c, err)
	}
	c = failedCommit(c, err)
	detail := []rune(c.Message)
	if len(detail) > 200 {
		detail = detail[:200]
	}
	c.State, c.Gaps = "gap", []Gap{{Kind: "invalid_source_data", Detail: string(detail), Origin: OriginSourceData}}
	return c
}

func failedCommit(c Commit, err error) Commit {
	c.Artifact, c.ArtifactRows, c.RunRows, c.Gaps, c.Observations = nil, nil, nil, nil, nil
	c.State = "failed"
	message := []rune(strings.ReplaceAll(err.Error(), "\x00", ""))
	if len(message) > 500 {
		message = message[:500]
	}
	c.Message = string(message)
	return c
}
