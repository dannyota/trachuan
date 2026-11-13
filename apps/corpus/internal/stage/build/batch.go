package build

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type loadBatch struct {
	tx       pgx.Tx
	limit    int
	batch    pgx.Batch
	contexts []string
}

func (b *loadBatch) queue(ctx context.Context, label, sql string, args ...any) error {
	b.batch.Queue(sql, args...)
	b.contexts = append(b.contexts, label)
	if len(b.contexts) == b.limit {
		return b.flush(ctx)
	}
	return nil
}

func (b *loadBatch) flush(ctx context.Context) error {
	if b == nil || len(b.contexts) == 0 {
		return nil
	}
	results := b.tx.SendBatch(ctx, &b.batch)
	var first error
	for _, label := range b.contexts {
		_, err := results.Exec()
		if err != nil && first == nil {
			first = fmt.Errorf("%s: %w", label, err)
		}
	}
	closeErr := results.Close()
	b.batch = pgx.Batch{}
	b.contexts = nil
	if first != nil {
		return first
	}
	return closeErr
}
