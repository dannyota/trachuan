package build

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type gapBatchTx struct {
	pgx.Tx
	calls int
}

func (tx *gapBatchTx) Exec(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
	tx.calls++
	return pgconn.CommandTag{}, nil
}

func TestLoadGaps_QueuesRows(t *testing.T) {
	tx := &gapBatchTx{}
	b := &loadBatch{tx: tx, limit: 2000}
	l := loader{tx: tx, batch: b, docs: map[string]*loadedDocument{}, gaps: []InventoryGap{{Stage: "fetch", ItemKey: "one", Kind: "missing"}, {Stage: "fetch", ItemKey: "two", Kind: "missing"}}}
	if err := l.loadGaps(context.Background()); err != nil {
		t.Fatal(err)
	}
	if tx.calls != 0 || b.batch.Len() != 2 {
		t.Fatalf("direct executions = %d, queued rows = %d; want 0 and 2", tx.calls, b.batch.Len())
	}
}
