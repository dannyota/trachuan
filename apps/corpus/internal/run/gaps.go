package run

import (
	"context"
	"fmt"

	"danny.vn/traquydinh/apps/corpus/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ListGaps lists the recorded gaps for a release.
func ListGaps(ctx context.Context, d Deps, cc, rel string) ([]store.Gap, error) {
	gaps, err := store.New(d.DB).ListRunGaps(ctx, store.ListRunGapsParams{Country: cc, Rel: rel})
	if err != nil {
		return nil, fmt.Errorf("list gaps: %w", err)
	}
	return gaps, nil
}

// AcceptGaps accepts all currently unaccepted gaps of a kind.
func AcceptGaps(ctx context.Context, d Deps, cc, rel, kind, note string) (int, error) {
	n, err := store.New(d.DB).AcceptRunGaps(ctx, store.AcceptRunGapsParams{Country: cc, Rel: rel, Kind: kind, Note: &note})
	if err != nil {
		return 0, fmt.Errorf("accept gaps: %w", err)
	}
	return int(n), nil
}

// Publishable reports whether all items and gaps permit publication.
func Publishable(ctx context.Context, db *pgxpool.Pool, runID int64) (bool, []string, error) {
	q := store.New(db)
	items, err := q.CountIncompleteItems(ctx, runID)
	if err != nil {
		return false, nil, fmt.Errorf("count incomplete items: %w", err)
	}
	gaps, err := q.CountUnacceptedGaps(ctx, runID)
	if err != nil {
		return false, nil, fmt.Errorf("count unaccepted gaps: %w", err)
	}
	var reasons []string
	if items > 0 {
		reasons = append(reasons, fmt.Sprintf("%d pending or failed items", items))
	}
	if gaps > 0 {
		reasons = append(reasons, fmt.Sprintf("%d unaccepted gaps", gaps))
	}
	return len(reasons) == 0, reasons, nil
}
