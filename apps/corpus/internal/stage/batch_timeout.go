package stage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/sourcegate"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const retryPrefix = "retry:"

type retryRecord struct {
	Cause    Cause  `json:"cause"`
	Endpoint string `json:"endpoint"`
}

func repeatedSourceTimeout(name string, generation, attempt int, row store.GetRunItemCommittedRow, current *RetryableError) bool {
	if (name != "fetch" && name != "expand") || current.GapKind != "source_unavailable" || current.Cause.Cause != CauseTimeout || int(row.Generation) != generation || int(row.Attempt) != attempt-1 || row.State != "pending" || row.Message == nil {
		return false
	}
	var previous retryRecord
	return strings.HasPrefix(*row.Message, retryPrefix) && json.Unmarshal([]byte(strings.TrimPrefix(*row.Message, retryPrefix)), &previous) == nil && previous.Cause == CauseTimeout
}

// recordRetry locks the same run and item fences as a terminal commit.
func recordRetry(ctx context.Context, pool *pgxpool.Pool, c Commit, retry *RetryableError) (bool, bool, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return false, false, fmt.Errorf("begin retry record: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := store.New(tx)
	generation, err := q.GetRunGenerationForShare(ctx, c.RunID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, false, nil
	}
	if err != nil {
		return false, false, fmt.Errorf("lock run generation: %w", err)
	}
	if int(generation) != c.Generation {
		return false, false, nil
	}
	row, err := q.GetRunItemCommitted(ctx, store.GetRunItemCommittedParams{RunID: c.RunID, Stage: c.Stage, ItemKey: c.ItemKey})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, false, nil
	}
	if err != nil {
		return false, false, fmt.Errorf("lock retry item: %w", err)
	}
	if row.State != "pending" || int(row.Generation) > c.Generation || (int(row.Generation) == c.Generation && int(row.Attempt) > c.Attempt) {
		return false, false, nil
	}
	if repeatedSourceTimeout(c.Stage, c.Generation, c.Attempt, row, retry) {
		return true, true, nil
	}
	cause := retry.Cause.Cause
	if errors.Is(retry, sourcegate.ErrSourcePaused) {
		cause = "source_paused"
	} else if cause == "" {
		cause = CauseUnknown
	}
	data, err := json.Marshal(retryRecord{Cause: cause, Endpoint: retry.Cause.Endpoint})
	if err != nil {
		return false, false, fmt.Errorf("encode retry record: %w", err)
	}
	message := retryPrefix + string(data)
	rows, err := q.UpdateRunItemFenced(ctx, store.UpdateRunItemFencedParams{RunID: c.RunID, Stage: c.Stage, ItemKey: c.ItemKey, Generation: int32(c.Generation), Attempt: int32(c.Attempt), RecipeHash: &c.RecipeHash, State: "pending", Message: &message})
	if err != nil {
		return false, false, fmt.Errorf("write retry record: %w", err)
	}
	if rows == 0 {
		return false, false, nil
	}
	if err := tx.Commit(ctx); err != nil {
		return false, false, fmt.Errorf("commit retry record: %w", err)
	}
	return false, true, nil
}
