package retrieval

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

type locatorRows struct {
	pgx.Rows
	done bool
}

func (r *locatorRows) Next() bool {
	if r.done {
		return false
	}
	r.done = true
	return true
}
func (*locatorRows) Close()     {}
func (*locatorRows) Err() error { return nil }
func (*locatorRows) Scan(d ...any) error {
	if len(d) != 11 {
		return fmt.Errorf("expected 11 columns, got %d", len(d))
	}
	*d[0].(*int64) = 1
	*d[1].(**int64) = nil
	*d[2].(*string) = "dieu"
	*d[3].(*string) = "Điều 1"
	*d[4].(*string) = "Nội dung"
	*d[5].(*int) = 1
	*d[6].(*int) = 1
	*d[7].(*string) = "text"
	*d[8].(*string) = "dieu-1"
	*d[9].(*string) = "tx:1"
	s := "block:2-4"
	*d[10].(**string) = &s
	return nil
}

type locatorTx struct {
	pgx.Tx
	sql string
}

func (tx *locatorTx) Query(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
	tx.sql = sql
	return &locatorRows{}, nil
}
func TestRead_LocatorLoading(t *testing.T) {
	for _, subtree := range []bool{false, true} {
		tx := &locatorTx{}
		var p []Provision
		var err error
		if subtree {
			p, err = readSubtree(context.Background(), tx, 1, "vn:vbpl:1", "dieu-1")
		} else {
			p, err = readNodes(context.Background(), tx, 1, "vn:vbpl:1")
		}
		if err != nil || !strings.Contains(tx.sql, "locator") || len(p) != 1 || p[0].Locator == nil || *p[0].Locator != "block:2-4" {
			t.Fatalf("subtree=%v nodes=%+v err=%v", subtree, p, err)
		}
	}
}
func TestRead_LocatorUnitsRangesAndParts(t *testing.T) {
	locator := "block:2-4"
	p := Provision{ID: 1, Kind: "dieu", Label: "Điều 1", Text: strings.Repeat("văn bản ", 4000), Identity: Identity{Anchor: "dieu-1"}, Locator: &locator}
	units := []Unit{readTextUnit("provision", p, []Provision{p}, p.Text), readOutline([]Provision{p})[0], readRange(readSequence{}, []Provision{p}, 0, 0).units[0]}
	parts, ok := unitParts(units[0])
	if !ok || len(parts) < 2 {
		t.Fatalf("parts=%d ok=%v", len(parts), ok)
	}
	units = append(units, parts...)
	for _, u := range units {
		b, err := json.Marshal(u)
		if err != nil {
			t.Fatal(err)
		}
		var v map[string]any
		if err = json.Unmarshal(b, &v); err != nil {
			t.Fatal(err)
		}
		if v["locator"] != locator {
			t.Fatalf("kind=%s part=%d locator=%v", u.Kind, u.Part, v["locator"])
		}
	}
}
