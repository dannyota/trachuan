package run

import (
	"context"
	"errors"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type statusCountDB struct {
	calls int
	query string
	args  []any
	count int64
}

func (*statusCountDB) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, errors.New("status must not write")
}
func (d *statusCountDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	d.calls++
	return nil, errors.New("status must not list or lock individual items")
}
func (d *statusCountDB) QueryRow(_ context.Context, query string, args ...any) pgx.Row {
	d.calls++
	d.query = query
	d.args = args
	return statusCountRow{count: d.count}
}

type statusCountRow struct{ count int64 }

func (r statusCountRow) Scan(dest ...any) error { *dest[0].(*int64) = r.count; return nil }

func TestStatus_CountsAdoptionsWithOneUnlockedAggregate(t *testing.T) {
	for _, want := range []int{0, 7} {
		db := &statusCountDB{count: int64(want)}
		got, err := countAdoptedItems(context.Background(), store.New(db), 42, 3)
		if err != nil || got != want {
			t.Fatalf("adopted=%d, want %d: %v", got, want, err)
		}
		if db.calls != 1 || !strings.Contains(strings.ToLower(db.query), "count(*)") || strings.Contains(strings.ToUpper(db.query), "FOR UPDATE") {
			t.Fatalf("calls=%d query=%s", db.calls, db.query)
		}
		if len(db.args) < 2 || db.args[0] != int64(42) || db.args[1] != int32(3) {
			t.Fatalf("arguments=%v", db.args)
		}
	}
}
