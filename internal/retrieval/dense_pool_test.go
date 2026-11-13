package retrieval

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5"
)

type densePoolRows struct {
	pgx.Rows
	flags  []*bool
	pos    int
	closed bool
}

func (rows *densePoolRows) Next() bool {
	if rows.pos == len(rows.flags) {
		return false
	}
	rows.pos++
	return true
}

func (rows *densePoolRows) Scan(dest ...any) error {
	*dest[0].(*int64) = int64(rows.pos)
	*dest[1].(*float64) = float64(10-rows.pos) / 10
	*dest[2].(**bool) = rows.flags[rows.pos-1]
	return nil
}

func (*densePoolRows) Err() error  { return nil }
func (rows *densePoolRows) Close() { rows.closed = true }

func TestANNQuery_PartitionsCapsAndRanksEachGroup(t *testing.T) {
	current, nonCurrent := true, false
	rows := &densePoolRows{flags: []*bool{nil, &current, &nonCurrent, &current, &current, &nonCurrent, &nonCurrent}}
	groups := []*ArmGroup{{}, {}}
	if err := annQuery(context.Background(), rowsTx{rows: rows}, groups, 2, ""); err != nil {
		t.Fatal(err)
	}
	want := [][]Candidate{
		{{ChunkID: 2, Cosine: .8, Rank: 1}, {ChunkID: 4, Cosine: .6, Rank: 2}},
		{{ChunkID: 3, Cosine: .7, Rank: 1}, {ChunkID: 6, Cosine: .4, Rank: 2}},
	}
	for i, group := range groups {
		if !reflect.DeepEqual(group.RawANN, want[i]) {
			t.Errorf("group %d = %+v, want %+v", i, group.RawANN, want[i])
		}
	}
	if !rows.closed {
		t.Fatal("pool rows left open")
	}
}

func TestANNQuery_EmptyPoolPreservesNilArms(t *testing.T) {
	groups := []*ArmGroup{{}, {}}
	if err := annQuery(context.Background(), rowsTx{rows: &densePoolRows{}}, groups, 2, ""); err != nil {
		t.Fatal(err)
	}
	if groups[0].RawANN != nil || groups[1].RawANN != nil {
		t.Fatal("empty pool changed nil arms")
	}
}

func TestANNQuery_ClassifiesDatabaseErrors(t *testing.T) {
	cause := errors.New("private arguments")
	for _, tc := range []struct {
		name string
		tx   pgx.Tx
	}{
		{"query", failingTx{err: cause}},
		{"scan", rowsTx{rows: &failingRows{scan: true, err: cause}}},
		{"iteration", rowsTx{rows: &failingRows{err: cause}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := annQuery(context.Background(), tc.tx, []*ArmGroup{{}, {}}, 2, ""); !errors.Is(err, ErrUnavailable) {
				t.Fatalf("got %v, want classified database error", err)
			}
		})
	}
}
