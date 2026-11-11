package retrieval

import (
	"context"
	"errors"
	"fmt"
)

// GapPageSize is the maximum quality_gaps page size from the request budget.
const GapPageSize = 200

// Gaps pages gaps by ascending coverage_gap.id, optionally restricted to kind.
// A zero limit uses GapPageSize; next is zero when the page is final.
func (e *Engine) Gaps(ctx context.Context, kind string, after int64, limit int) ([]Gap, int64, error) {
	if after < 0 || limit < 0 || limit > GapPageSize {
		return nil, 0, errors.New("invalid gap pagination")
	}
	if limit == 0 {
		limit = GapPageSize
	}
	ctx, cancel := context.WithTimeout(ctx, e.deadline())
	defer cancel()
	tx, err := e.metadataTx(ctx)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	rows, err := tx.Query(ctx, `SELECT id,kind,COALESCE(document_key,''),message FROM coverage_gap WHERE id>$1 AND ($2='' OR kind=$2) ORDER BY id LIMIT $3`, after, kind, limit+1)
	if err != nil {
		return nil, 0, fmt.Errorf("query gaps: %w", err)
	}
	page := make([]Gap, 0, limit)
	var last, next int64
	for rows.Next() {
		var id int64
		var gap Gap
		if err = rows.Scan(&id, &gap.Kind, &gap.DocumentKey, &gap.Message); err != nil {
			rows.Close()
			return nil, 0, fmt.Errorf("scan gap: %w", err)
		}
		if len(page) == limit {
			next = last
			break
		}
		page = append(page, gap)
		last = id
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, 0, fmt.Errorf("read gaps: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, 0, fmt.Errorf("commit gaps: %w", err)
	}
	return page, next, nil
}
