package run

import (
	"context"
	"fmt"

	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/store"
)

func countAdoptedItems(ctx context.Context, q *store.Queries, runID int64, generation int) (int, error) {
	message := stage.AcquisitionMessage
	count, err := q.CountAdoptedRunItems(ctx, store.CountAdoptedRunItemsParams{RunID: runID, Generation: int32(generation), Message: &message})
	if err != nil {
		return 0, fmt.Errorf("count adopted items: %w", err)
	}
	return int(count), nil
}
