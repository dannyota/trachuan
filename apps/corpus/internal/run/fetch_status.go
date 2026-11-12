package run

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/store"
)

// FetchReads counts committed fetch and attachment decisions in the current generation.
type FetchReads struct {
	Attachments        map[string]int64 `json:"attachments,omitempty"`
	Reused             int64            `json:"reused"`
	NoBaseline         int64            `json:"no_baseline"`
	InvalidBaseline    int64            `json:"invalid_baseline"`
	NullDate           int64            `json:"null_date"`
	DateChanged        int64            `json:"date_changed"`
	MissingAttachment  int64            `json:"missing_attachment"`
	Control            int64            `json:"control"`
	ControlSame        int64            `json:"control_same"`
	ControlTreeIDsOnly int64            `json:"control_tree_ids_only"`
	ControlChanged     int64            `json:"control_changed"`
}

func countFetchReads(ctx context.Context, q *store.Queries, runID int64, generation int32) (FetchReads, error) {
	row, err := q.CountFetchReads(ctx, store.CountFetchReadsParams{RunID: runID, Generation: generation})
	if err != nil {
		return FetchReads{}, fmt.Errorf("count fetch reads: %w", err)
	}
	result := FetchReads{Reused: row.Reused, NoBaseline: row.NoBaseline, InvalidBaseline: row.InvalidBaseline, NullDate: row.NullDate, DateChanged: row.DateChanged, MissingAttachment: row.MissingAttachment, Control: row.Control, ControlSame: row.ControlSame, ControlTreeIDsOnly: row.ControlTreeIdsOnly, ControlChanged: row.ControlChanged}
	messages, err := q.ListFetchReadMessages(ctx, store.ListFetchReadMessagesParams{RunID: runID, Generation: generation})
	if err != nil {
		return FetchReads{}, fmt.Errorf("count attachment reads: %w", err)
	}
	for _, message := range messages {
		result.addFiles(message)
	}
	return result, nil
}

func (f *FetchReads) addFiles(message string) {
	for _, part := range strings.Fields(message) {
		if !strings.HasPrefix(part, "files=") {
			continue
		}
		var counts map[string]int64
		if json.Unmarshal([]byte(strings.TrimPrefix(part, "files=")), &counts) != nil {
			continue
		}
		if f.Attachments == nil {
			f.Attachments = map[string]int64{}
		}
		for name, n := range counts {
			f.Attachments[name] += n
		}
	}
}
