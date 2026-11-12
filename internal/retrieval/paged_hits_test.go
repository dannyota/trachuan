package retrieval

import (
	"reflect"
	"testing"
)

func TestPagedHits_MatchesSearchWalk(t *testing.T) {
	a, r := pagingFixture(30, 20)
	a.Date = Date{Year: 2099, Month: 1, Day: 1}
	a.EffectiveNodes = map[int64][]string{1: {"1.1"}, 5: {"1.1"}, 35: {"1.1"}}
	req := Request{Limit: 8, Arms: "dense", Nodes: []string{"1.1"}}
	got, err := PagedHits(a, r, req, 8, 100)
	if err != nil {
		t.Fatal(err)
	}
	req, err = normalizeFilters(req)
	if err != nil {
		t.Fatal(err)
	}
	c := searchCursor{FirstLimit: 8}
	for i, page := range got {
		want := walkPage(t, a, r, req, c)
		if !reflect.DeepEqual(page, want) {
			t.Fatalf("page %d differs", i)
		}
		if want.NextCursor == "" {
			if i != len(got)-1 {
				t.Fatal("pages after exhaustion")
			}
			return
		}
		c, err = decodeSearchCursor(want.NextCursor)
		if err != nil {
			t.Fatal(err)
		}
		req.Cursor = want.NextCursor
	}
	t.Fatal("walk ended early")
}
