package membership

import (
	"context"
	"errors"
	"fmt"

	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"github.com/jackc/pgx/v5"
)

// FinalActiveKeys returns nil when no final phase applies.
func FinalActiveKeys(ctx context.Context, q *store.Queries, run stage.Run) (map[string]bool, error) {
	list, err := source.Parse(run.Snapshot.SourceList)
	if err != nil {
		return nil, fmt.Errorf("parse source list: %w", err)
	}
	if list.VBPL.SelectionVersion == 0 {
		return nil, nil
	}
	scope, err := q.GetRunScope(ctx, run.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read scope: %w", err)
	}
	if scope.FinalSnapshot == nil && scope.FinalHash == nil {
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
