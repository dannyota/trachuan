package retrieval

import (
	"fmt"
	"reflect"
	"testing"
)

func pagingFixture(primary, other int) (Arms, Recipe) {
	a, r := shapeFixture("Nội dung")
	base := a.Primary.Dense[0]
	a.Primary, a.NonCurrent = ArmGroup{}, ArmGroup{}
	r.DenseDepth, r.LexicalDepth = 200, 100
	a.Depths = Depths{200, 100}
	a.Release = "vn-20990101"
	for i := 0; i < primary+other; i++ {
		c := base
		c.ChunkID, c.GroupID = int64(i+1), int64(i+1)
		c.Group = Identity{Country: "vn", DocumentKey: fmt.Sprintf("vn:%03d", i), Anchor: "dieu-1"}
		c.Identity = c.Group
		c.Descendants = append([]Provision(nil), base.Descendants...)
		c.Descendants[0].ID = c.GroupID
		c.Descendants[0].Identity = c.Group
		c.Ancestors = c.Descendants
		g := &a.Primary
		if i >= primary {
			g = &a.NonCurrent
			c.Document.StatusClass = "expired"
			c.Document.IsCurrent = false
		}
		g.Dense = append(g.Dense, c)
		g.Members = append(g.Members, c)
	}
	return a, r
}

func walkPage(t *testing.T, a Arms, r Recipe, req Request, c searchCursor) Response {
	t.Helper()
	if c.V == 0 {
		c.V = 1
		c.Release = a.Release
		c.Digest = requestDigest(req)
		c.AsOf = a.Date
	}
	w, err := buildSearchWalk(a, r, c.FirstLimit, req.Arms)
	if err != nil {
		t.Fatal(err)
	}
	w.filter(req, nil, nil)
	out, err := w.page(a, r, req, c)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func pageIdentities(out Response) []Identity {
	var ids []Identity
	for _, hs := range [][]Hit{out.Hits, out.NonCurrent} {
		for _, h := range hs {
			ids = append(ids, h.Identity)
		}
	}
	return ids
}

func TestSearch_NoFilterUnchanged(t *testing.T) {
	a, r := pagingFixture(25, 20)
	for _, limit := range []int{1, 3, 8, 20, 50} {
		want, err := Shape(a, r, limit, "dense")
		if err != nil {
			t.Fatal(err)
		}
		got := walkPage(t, a, r, Request{Limit: limit, Arms: "dense"}, searchCursor{FirstLimit: limit})
		got.NextCursor = ""
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("limit %d changed first response", limit)
		}
	}
}

func TestPaging_PrimaryThenNonCurrent(t *testing.T) {
	testPagingGroups(t, true, [][2]int{{4, 4}, {6, 2}, {0, 8}})
}
func TestPaging_TailMode(t *testing.T) { testPagingGroups(t, false, [][2]int{{6, 2}, {4, 4}, {0, 8}}) }
func testPagingGroups(t *testing.T, include bool, counts [][2]int) {
	t.Helper()
	a, r := pagingFixture(10, 90)
	a.IncludeNonCurrent = include
	for _, filtered := range []bool{false, true} {
		req := Request{Limit: 8, Arms: "dense"}
		if filtered {
			req.Documents = []string{"missing"}
		}
		c := searchCursor{FirstLimit: 8}
		seen := map[Identity]bool{}
		for page := 0; ; page++ {
			out := walkPage(t, a, r, req, c)
			if len(out.Hits)+len(out.NonCurrent) > 8 {
				t.Fatal("page exceeds limit")
			}
			if page < len(counts) && (len(out.Hits) != counts[page][0] || len(out.NonCurrent) != counts[page][1]) {
				t.Fatalf("page %d counts %d/%d", page, len(out.Hits), len(out.NonCurrent))
			}
			for _, id := range pageIdentities(out) {
				if seen[id] {
					t.Fatal("repeated", id)
				}
				seen[id] = true
			}
			if out.NextCursor == "" {
				break
			}
			var err error
			c, err = decodeSearchCursor(out.NextCursor)
			if err != nil {
				t.Fatal(err)
			}
		}
		if len(seen) != 100 {
			t.Fatalf("walked %d", len(seen))
		}
	}
}

func TestPaging_ChangingLimit(t *testing.T) {
	a, r := pagingFixture(30, 70)
	c := searchCursor{FirstLimit: 3}
	seen := map[Identity]bool{}
	for page := 0; ; page++ {
		limits := []int{3, 8, 1, 50}
		out := walkPage(t, a, r, Request{Limit: limits[page%len(limits)], Arms: "dense"}, c)
		for _, id := range pageIdentities(out) {
			if seen[id] {
				t.Fatal("repeated", id)
			}
			seen[id] = true
		}
		if out.NextCursor == "" {
			break
		}
		var err error
		c, err = decodeSearchCursor(out.NextCursor)
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(seen) != 100 {
		t.Fatalf("walked %d", len(seen))
	}
}

func TestPaging_AbstainCarried(t *testing.T) {
	for _, primary := range []int{0, 1} {
		for _, threshold := range []float64{0, 1} {
			for _, filtered := range []bool{false, true} {
				a, r := pagingFixture(primary, 20)
				r.Abstention.Threshold = &threshold
				want, err := Shape(a, r, 8, "dense")
				if err != nil {
					t.Fatal(err)
				}
				req := Request{Limit: 8, Arms: "dense"}
				if filtered {
					req.Documents = []string{"missing"}
				}
				c := searchCursor{FirstLimit: 8}
				for {
					out := walkPage(t, a, r, req, c)
					if out.Abstain != want.Abstain || out.Reason != want.Reason || out.Message != want.Message {
						t.Fatal("abstention changed")
					}
					if out.NextCursor == "" {
						break
					}
					c, err = decodeSearchCursor(out.NextCursor)
					if err != nil {
						t.Fatal(err)
					}
				}
			}
		}
	}
}

func hydrationFixture(other int) (Arms, Recipe) {
	a, r := pagingFixture(65, other)
	for _, g := range []*ArmGroup{&a.Primary} {
		for _, arm := range [][]Candidate{g.Dense, g.Members} {
			for i := 0; i < 64; i++ {
				arm[i].Group.DocumentKey = "A"
				arm[i].Group.Anchor = fmt.Sprintf("a%d", i+1)
			}
		}
	}
	return a, r
}

func TestPaging_HydrationCounterexample(t *testing.T)        { testHydrationWalk(t, false) }
func TestFilter_FirstPageKeepsTodaysInsideHits(t *testing.T) { testHydrationWalk(t, true) }
func testHydrationWalk(t *testing.T, filtered bool) {
	t.Helper()
	for _, other := range []int{2, 8} {
		t.Run(fmt.Sprintf("non_current_%d", other), func(t *testing.T) { testHydrationPage(t, filtered, other) })
	}
}

func testHydrationPage(t *testing.T, filtered bool, other int) {
	t.Helper()
	a, r := hydrationFixture(other)
	want, err := Shape(a, r, 8, "dense")
	if err != nil {
		t.Fatal(err)
	}
	req := Request{Limit: 8, Arms: "dense"}
	if filtered {
		req.Documents = []string{"A", a.Primary.Dense[64].Group.DocumentKey}
	}
	c := searchCursor{FirstLimit: 8}
	out := walkPage(t, a, r, req, c)
	if len(out.Hits) != 6 || len(out.NonCurrent) != 2 {
		t.Fatal("first page group counts changed")
	}
	for i, h := range out.Hits {
		if h.Identity.DocumentKey != "A" || h.Identity.Anchor != fmt.Sprintf("a%d", i+1) {
			t.Fatal("first page crossed hydration window", h.Identity)
		}
	}
	if !reflect.DeepEqual(pageIdentities(out), pageIdentities(want)) {
		t.Fatal("first page changed", pageIdentities(out))
	}
	seen := map[Identity]bool{}
	for {
		for _, id := range pageIdentities(out) {
			if seen[id] {
				t.Fatal("repeated", id)
			}
			seen[id] = true
		}
		if out.NextCursor == "" {
			break
		}
		c, err = decodeSearchCursor(out.NextCursor)
		if err != nil {
			t.Fatal(err)
		}
		out = walkPage(t, a, r, req, c)
	}
	if len(seen) != 65+other || !seen[a.Primary.Dense[64].Group] {
		t.Fatalf("lost hits: %d", len(seen))
	}
}
