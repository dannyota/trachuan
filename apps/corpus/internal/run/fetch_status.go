package run

import (
	"context"
	"fmt"

	"danny.vn/trachuan/apps/corpus/internal/store"
)

// FetchReads counts committed VBPL fetch decisions in the current generation.
type FetchReads struct {
	Reused             int64 `json:"reused"`
	NoBaseline         int64 `json:"no_baseline"`
	InvalidBaseline    int64 `json:"invalid_baseline"`
	NullDate           int64 `json:"null_date"`
	DateChanged        int64 `json:"date_changed"`
	MissingAttachment  int64 `json:"missing_attachment"`
	Age                int64 `json:"age"`
	Control            int64 `json:"control"`
	ControlSame        int64 `json:"control_same"`
	ControlTreeIDsOnly int64 `json:"control_tree_ids_only"`
	ControlChanged     int64 `json:"control_changed"`
}

func countFetchReads(ctx context.Context, q *store.Queries, runID int64, generation int32) (FetchReads, error) {
	row, err := q.CountFetchReads(ctx, store.CountFetchReadsParams{RunID: runID, Generation: generation})
	if err != nil {
		return FetchReads{}, fmt.Errorf("count fetch reads: %w", err)
	}
	return FetchReads{row.Reused, row.NoBaseline, row.InvalidBaseline, row.NullDate, row.DateChanged, row.MissingAttachment, row.Age, row.Control, row.ControlSame, row.ControlTreeIdsOnly, row.ControlChanged}, nil
}
