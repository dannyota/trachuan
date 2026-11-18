package stage

import (
	"context"

	"danny.vn/trachuan/apps/corpus/internal/detector"
	"danny.vn/trachuan/apps/corpus/internal/store"
)

func InsertGap(ctx context.Context, q *store.Queries, params store.InsertGapParams) error {
	if err := detector.ValidateKind(params.Kind); err != nil {
		return err
	}
	return q.InsertGap(ctx, params)
}
