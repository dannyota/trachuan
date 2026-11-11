package retrieval

import (
	"math/rand"
	"runtime"
	"strings"
	"testing"
)

func TestReadMatch_MatchesCountAndIndex(t *testing.T) {
	random := rand.New(rand.NewSource(42))
	for trial := 0; trial < 200; trial++ {
		runes := []rune("abđ ")
		word := func(n int) string {
			r := make([]rune, n)
			for i := range r {
				r[i] = runes[random.Intn(len(runes))]
			}
			return string(r)
		}
		text := word(50)
		patterns := []string{"a", "aa", "aaa", "đ", "ab", "b", word(1), word(2), word(3), word(4)}
		for _, p := range patterns {
			for _, after := range []int{0, random.Intn(len(text) + 1), len(text)} {
				got := findReadChild(text, p, after)
				count := min(strings.Count(text, p), 2)
				if got.count != count || got.first != strings.Index(text, p) {
					t.Fatalf("trial %d pattern %q after %d: got %+v, want count %d at %d", trial, p, after, got, count, strings.Index(text, p))
				}
			}
		}
	}
}

func TestReadTree_LargeChildrenBoundedAllocation(t *testing.T) {
	const children = 9
	const childBytes = 32 << 10
	parent := int64(1)
	nodes := make([]Provision, children+1)
	nodes[0] = Provision{ID: 1, Kind: "dieu", Label: "Điều"}
	var text strings.Builder
	for i := 1; i <= children; i++ {
		child := string(rune('a'+i-1)) + strings.Repeat("x", childBytes-1)
		text.WriteString(child)
		nodes[i] = Provision{ID: int64(i + 1), ParentID: &parent, Ordinal: i, Kind: "khoan", Text: child}
	}
	nodes[0].Text = text.String()
	tree, err := newReadTree(t.Context(), nodes)
	if err != nil {
		t.Fatal(err)
	}
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	if err = tree.render(t.Context(), 0); err != nil {
		t.Fatal(err)
	}
	runtime.ReadMemStats(&after)
	allocated := after.TotalAlloc - before.TotalAlloc
	limit := uint64(8*len(nodes[0].Text) + 128*1024)
	if allocated > limit {
		t.Fatalf("allocated %d bytes for %d bytes of input; limit %d", allocated, len(nodes[0].Text), limit)
	}
	if tree.nodes[0].ambiguous || tree.nodes[0].text != nodes[0].Text || tree.nodes[0].own != "" {
		t.Fatal("contained children changed")
	}
}

func TestReadTree_OrderedRepeatedChildIsAmbiguous(t *testing.T) {
	parent := int64(1)
	nodes := []Provision{
		{ID: 1, Kind: "dieu", Text: "a b a", Identity: Identity{Anchor: "root"}},
		{ID: 2, ParentID: &parent, Kind: "khoan", Ordinal: 1, NodeID: "a", Text: "a", Identity: Identity{Anchor: "a"}},
		{ID: 3, ParentID: &parent, Kind: "khoan", Ordinal: 2, NodeID: "b", Text: "b", Identity: Identity{Anchor: "b"}},
	}
	tree, err := newReadTree(t.Context(), nodes)
	if err != nil {
		t.Fatal(err)
	}
	if err = tree.render(t.Context(), 0); err != nil {
		t.Fatal(err)
	}
	rendered := renderTree(nodes, 1)
	if !rendered.ambiguous || !tree.nodes[0].ambiguous || tree.nodes[0].own != "a b a" {
		t.Fatal("repeated child was treated as unique")
	}
}
