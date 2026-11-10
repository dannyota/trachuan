package membership

import (
	"context"
	"fmt"

	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/store"
)

// FinalActiveKeys returns nil for legacy selection or the verified final active keys.
func FinalActiveKeys(ctx context.Context, q *store.Queries, run stage.Run) (map[string]bool, error) {
	list, err := source.Parse(run.Snapshot.SourceList)
	if err != nil {
		return nil, fmt.Errorf("parse source list: %w", err)
	}
	if list.VBPL.SelectionVersion == 0 {
		return nil, nil
	}
	phase, err := ReadPhase(ctx, q, run.ID, Final)
	if err != nil {
		return nil, fmt.Errorf("read final membership: %w", err)
	}
	keys := make(map[string]bool, len(phase.Active))
	for _, entry := range phase.Active {
		keys[entry.DocumentKey] = true
	}
	return keys, nil
}
