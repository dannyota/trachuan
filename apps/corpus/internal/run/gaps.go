package run

import (
	"context"
	"fmt"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/store"
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
	tx, err := d.DB.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin gap acceptance: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := store.New(tx)
	row, err := q.GetRunForUpdate(ctx, store.GetRunForUpdateParams{Country: cc, Rel: rel})
	if err != nil {
		return 0, fmt.Errorf("lock run for gap acceptance: %w", err)
	}
	codes, err := q.ListBlockingRunFailureCodes(ctx, store.ListBlockingRunFailureCodesParams{RunID: row.ID, Codes: stage.BlockingFailureCodes()})
	if err != nil {
		return 0, fmt.Errorf("list blocking run failures: %w", err)
	}
	if len(codes) > 0 {
		return 0, fmt.Errorf("cannot accept gaps with failure %s", strings.Join(codes, ", "))
	}
	n, err := q.AcceptRunGaps(ctx, store.AcceptRunGapsParams{Country: cc, Rel: rel, Kind: kind, Note: &note})
	if err != nil {
		return 0, fmt.Errorf("accept gaps: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit gap acceptance: %w", err)
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
	codes, err := q.ListBlockingRunFailureCodes(ctx, store.ListBlockingRunFailureCodesParams{RunID: runID, Codes: stage.BlockingFailureCodes()})
	if err != nil {
		return false, nil, fmt.Errorf("list blocking run failures: %w", err)
	}
	paused, err := q.CountPausedSources(ctx, runID)
	if err != nil {
		return false, nil, fmt.Errorf("count paused sources: %w", err)
	}
	var reasons []string
	for _, code := range codes {
		reasons = append(reasons, fmt.Sprintf("run failure %s", code))
	}
	if paused > 0 {
		reasons = append(reasons, fmt.Sprintf("%d paused sources", paused))
	}
	if items > 0 {
		reasons = append(reasons, fmt.Sprintf("%d pending or failed items", items))
	}
	if gaps > 0 {
		reasons = append(reasons, fmt.Sprintf("%d unaccepted gaps", gaps))
	}
	return len(reasons) == 0, reasons, nil
}
