package embed

import (
	"context"
	"reflect"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type orderedDB struct{ hashes []string }

func (d *orderedDB) Exec(_ context.Context, _ string, args ...interface{}) (pgconn.CommandTag, error) {
	d.hashes = append(d.hashes, args[1].(string))
	return pgconn.NewCommandTag("INSERT 0 1"), nil
}
func (*orderedDB) Query(context.Context, string, ...interface{}) (pgx.Rows, error) {
	panic("unexpected query")
}
func (*orderedDB) QueryRow(context.Context, string, ...interface{}) pgx.Row {
	panic("unexpected query")
}

func TestInsertRows_OrdersByInputHashWithoutMutatingRows(t *testing.T) {
	rows := []Row{{InputSHA256: "b"}, {InputSHA256: "a"}}
	db := &orderedDB{}
	if _, err := insertRows(context.Background(), store.New(db), "recipe", 1, rows); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(db.hashes, []string{"a", "b"}) {
		t.Fatalf("insertion order=%v", db.hashes)
	}
	if rows[0].InputSHA256 != "b" {
		t.Fatal("mutated artifact row order")
	}
}
