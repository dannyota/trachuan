package run

import (
	"context"
	"encoding/json"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type listingDB struct {
	pgx.Rows
	keys    []string
	deleted map[string]int
	index   int
}

func (d *listingDB) Query(context.Context, string, ...interface{}) (pgx.Rows, error) { return d, nil }
func (d *listingDB) QueryRow(context.Context, string, ...interface{}) pgx.Row        { return nil }
func (d *listingDB) Exec(_ context.Context, _ string, args ...interface{}) (pgconn.CommandTag, error) {
	d.deleted[args[1].(string)]++
	return pgconn.NewCommandTag("DELETE 1"), nil
}
func (d *listingDB) Next() bool { d.index++; return d.index <= len(d.keys) }
func (d *listingDB) Scan(dest ...interface{}) error {
	*dest[0].(*string) = d.keys[d.index-1]
	*dest[1].(*string) = "item"
	*dest[2].(*json.RawMessage) = json.RawMessage(`{}`)
	return nil
}
func (d *listingDB) Close()     {}
func (d *listingDB) Err() error { return nil }

func TestDeleteDiscoverListings_DeletesEachQueryOnce(t *testing.T) {
	key := "vbpl:keyword:law"
	first := key + "@0123456789abcdef"
	second := key + "@fedcba9876543210"
	db := &listingDB{keys: []string{first, first, second, second, key + "@invalid", "other@0123456789abcdef"}, deleted: map[string]int{}}
	if err := deleteDiscoverListings(context.Background(), store.New(db), 1, key); err != nil {
		t.Fatal(err)
	}
	if len(db.deleted) != 2 || db.deleted[first] != 1 || db.deleted[second] != 1 {
		t.Fatalf("deletions = %v, want each valid query once", db.deleted)
	}
}
