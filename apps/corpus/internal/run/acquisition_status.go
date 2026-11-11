package run

import (
	"context"
	"errors"
	"fmt"

	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"github.com/jackc/pgx/v5"
)

func countAdoptedItems(ctx context.Context, q *store.Queries, runID int64, generation int) (int, error) {
	count := 0
	for _, name := range []string{"expand", "fetch"} {
		items, err := q.ListStageItemArtifacts(ctx, store.ListStageItemArtifactsParams{RunID: runID, Stage: name})
		if err != nil {
			return 0, fmt.Errorf("list acquisition items: %w", err)
		}
		for _, item := range items {
			if item.State != "done" || item.Sha256 == nil {
				continue
			}
			committed, err := q.GetRunItemCommitted(ctx, store.GetRunItemCommittedParams{RunID: runID, Stage: name, ItemKey: item.ItemKey})
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			if err != nil {
				return 0, fmt.Errorf("read acquisition item: %w", err)
			}
			if committed.State == "done" && int(committed.Generation) == generation && committed.Message != nil && *committed.Message == stage.AcquisitionMessage {
				count++
			}
		}
	}
	return count, nil
}
