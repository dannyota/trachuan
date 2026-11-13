package retrieval

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestReadRange_ManyNodes(t *testing.T) {
	const count = 50_000
	nodes := make([]Provision, count)
	for i := range nodes {
		nodes[i] = Provision{ID: int64(i + 1), Identity: Identity{DocumentKey: "vn:1", Anchor: "a"}, Kind: "dieu", Label: "Điều", Text: "Text"}
	}
	start := time.Now()
	seq, err := readRangeContext(t.Context(), readSequence{}, nodes, 0, len(nodes)-1)
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > 20*time.Second {
		t.Fatalf("rendering %d nodes took %s", count, elapsed)
	}
	if len(seq.units) != count || !seq.complete || seq.units[0].Text != "Text" || seq.units[count-1].Text != "Text" {
		t.Fatal("large range lost units")
	}
}

type cancelAfterChecks struct {
	context.Context
	checks int
	after  int
}

func (c *cancelAfterChecks) Err() error {
	c.checks++
	if c.checks >= c.after {
		return context.Canceled
	}
	return nil
}

type manyNodesTx struct {
	readCacheTx
	nodes int
}

func (tx *manyNodesTx) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if strings.Contains(sql, "SELECT id,parent_id,kind") {
		rows := &cacheRows{}
		for i := range tx.nodes {
			rows.values = append(rows.values, []any{int64(i + 1), nil, "dieu", "Điều", "", 1, i + 1, "Text", "a", nil})
		}
		return rows, nil
	}
	return tx.readCacheTx.Query(ctx, sql, args...)
}

func TestReadRange_CancellationStopsRendering(t *testing.T) {
	ctx := &cancelAfterChecks{Context: context.Background(), after: 4}
	e := &Engine{release: Release{Rel: "20991110_1", RetrievalRecipe: Recipe2()}}
	_, _, err := e.readScope(ctx, &manyNodesTx{nodes: 1000}, cursor{Item: ReadItem{Handle: "vn:1"}, WholeDocument: true})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("readScope error=%v, checks=%d; want cancellation", err, ctx.checks)
	}
}

func TestReadTree_MatchesExistingRendering(t *testing.T) {
	parent := int64(1)
	for _, tc := range []struct{ name, parent, first, second string }{
		{"separate", "intro", "first", "second"},
		{"contained", "intro first and second", "first", "second"},
		{"partial", "intro first", "first", "second"},
		{"repeated", "first first", "first", "second"},
		{"overlap", "aaa", "aa", "z"},
		{"empty parent", "", "first", "second"},
		{"empty child", "prefix second", "", "second"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			nodes := []Provision{
				{ID: 1, Kind: "dieu", Label: "Điều 1", Text: tc.parent, Identity: Identity{DocumentKey: "vn:1", Anchor: "a"}},
				{ID: 2, ParentID: &parent, Kind: "khoan", Label: "1", NodeID: "b", Ordinal: 1, Text: tc.first, Identity: Identity{DocumentKey: "vn:1", Anchor: "b"}},
				{ID: 3, ParentID: &parent, Kind: "khoan", Label: "2", NodeID: "c", Ordinal: 2, Text: tc.second, Identity: Identity{DocumentKey: "vn:1", Anchor: "c"}},
			}
			tree, err := newReadTree(t.Context(), nodes)
			if err != nil {
				t.Fatal(err)
			}
			for i, p := range nodes {
				want, wantGaps := readOwnText(nodes, p)
				if err = tree.render(t.Context(), i); err != nil {
					t.Fatal(err)
				}
				got := tree.nodes[i]
				if got.citation != citation(readAncestry(nodes, p)) {
					t.Fatalf("node %d: citation changed", i)
				}
				if got.own != want || got.ambiguous != (len(wantGaps) > 0) {
					t.Fatalf("node %d: got text %q ambiguous %v; want %q gaps %v", i, got.own, got.ambiguous, want, wantGaps)
				}
				wantLeads, wantLeadGaps := readLeads(nodes, p)
				leads, gaps, leadErr := tree.leads(t.Context(), i)
				if leadErr != nil || !reflect.DeepEqual(leads, wantLeads) || !reflect.DeepEqual(gaps, wantLeadGaps) {
					t.Fatalf("node %d: leads differ: %v", i, leadErr)
				}
			}
		})
	}
}

func TestReadRange_ManySiblingsAndLongParent(t *testing.T) {
	const count = 20_000
	parent := int64(1)
	nodes := make([]Provision, count+1)
	nodes[0] = Provision{ID: 1, Kind: "dieu", Label: "Điều", Text: strings.Repeat("x", 100_000)}
	for i := 1; i <= count; i++ {
		nodes[i] = Provision{ID: int64(i + 1), ParentID: &parent, Kind: "khoan", NodeID: fmt.Sprintf("n%05d", i), Ordinal: count - i, Text: fmt.Sprintf("%05d", i)}
	}
	start := time.Now()
	seq, err := readRangeContext(t.Context(), readSequence{}, nodes, 0, count)
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > 20*time.Second {
		t.Fatalf("rendering siblings took %s", elapsed)
	}
	if len(seq.units) != count+1 || !seq.complete || seq.units[0].Text != nodes[0].Text {
		t.Fatal("large tree changed text")
	}
}

func TestReadTree_CancellationDuringRendering(t *testing.T) {
	parent := int64(1)
	nodes := make([]Provision, 1001)
	nodes[0] = Provision{ID: 1, Text: "intro"}
	for i := 1; i < len(nodes); i++ {
		nodes[i] = Provision{ID: int64(i + 1), ParentID: &parent, Ordinal: i, Text: "child"}
	}
	tree, err := newReadTree(context.Background(), nodes)
	if err != nil {
		t.Fatal(err)
	}
	ctx := &cancelAfterChecks{Context: context.Background(), after: 20}
	if err = tree.render(ctx, 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
	rendered := 0
	for _, n := range tree.nodes {
		if n.rendered {
			rendered++
		}
	}
	if rendered == 0 || rendered >= len(nodes) {
		t.Fatalf("rendered %d of %d nodes", rendered, len(nodes))
	}
}

func TestReadTree_SiblingOrderMatchesRendering(t *testing.T) {
	random := rand.New(rand.NewSource(42))
	nodes := make([]Provision, 3000)
	indices := make([]int, len(nodes))
	for i := range nodes {
		nodes[i] = Provision{ID: int64(i + 1), Ordinal: random.Intn(100) - 50, NodeID: fmt.Sprintf("n%04d-%s", random.Intn(1000), strings.Repeat("đ", random.Intn(3)))}
		indices[i] = i
	}
	tree, err := newReadTree(t.Context(), nodes)
	if err != nil {
		t.Fatal(err)
	}
	want := append([]int(nil), indices...)
	sort.SliceStable(want, func(i, j int) bool {
		a, b := nodes[want[i]], nodes[want[j]]
		if a.Ordinal != b.Ordinal {
			return a.Ordinal < b.Ordinal
		}
		return a.NodeID < b.NodeID
	})
	got, err := tree.orderChildren(t.Context(), indices)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatal("sibling order changed", err)
	}
}

func TestReadTree_ManyContainedChildrenMatchExistingRendering(t *testing.T) {
	parent := int64(1)
	nodes := []Provision{{ID: 1, Kind: "dieu", Label: "Điều", Identity: Identity{Anchor: "parent"}}}
	var text strings.Builder
	text.WriteString("lead ")
	for i := 1; i <= 15; i++ {
		child := fmt.Sprintf("<%d>", i)
		text.WriteString(child + " ")
		nodes = append(nodes, Provision{ID: int64(i + 1), ParentID: &parent, Kind: "khoan", Ordinal: i, NodeID: fmt.Sprintf("n%02d", i), Text: child, Identity: Identity{Anchor: child}})
	}
	nodes[0].Text = text.String()
	tree, err := newReadTree(t.Context(), nodes)
	if err != nil {
		t.Fatal(err)
	}
	if err = tree.render(t.Context(), 0); err != nil {
		t.Fatal(err)
	}
	want, gaps := readOwnText(nodes, nodes[0])
	if tree.nodes[0].own != want || tree.nodes[0].ambiguous != (len(gaps) > 0) {
		t.Fatal("contained children changed parent")
	}
	leads, leadGaps, err := tree.leads(t.Context(), len(nodes)-1)
	expected, expectedGaps := readLeads(nodes, nodes[len(nodes)-1])
	if err != nil || !reflect.DeepEqual(leads, expected) || !reflect.DeepEqual(leadGaps, expectedGaps) {
		t.Fatal("contained children changed lead", err)
	}
}

func TestReadTree_NestedRenderingMatchesExisting(t *testing.T) {
	random := rand.New(rand.NewSource(17))
	words := []string{"", "a", "b", "aa", "a b", "a b a", "intro a b end", "b a", "điều luật"}
	one, two := int64(1), int64(2)
	for trial := 0; trial < 500; trial++ {
		nodes := []Provision{
			{ID: 1, Kind: "dieu", Label: "Điều 1", Text: words[random.Intn(len(words))], Identity: Identity{Anchor: "a"}},
			{ID: 2, ParentID: &one, Kind: "khoan", Label: "1", Ordinal: 1, NodeID: "b", Text: words[random.Intn(len(words))], Identity: Identity{Anchor: "b"}},
			{ID: 3, ParentID: &two, Kind: "diem", Label: "a", Ordinal: 1, NodeID: "c", Text: words[random.Intn(len(words))], Identity: Identity{Anchor: "c"}},
			{ID: 4, ParentID: &one, Kind: "khoan", Label: "2", Ordinal: 2, NodeID: "d", Text: words[random.Intn(len(words))], Identity: Identity{Anchor: "d"}},
		}
		tree, err := newReadTree(t.Context(), nodes)
		if err != nil {
			t.Fatal(err)
		}
		for i, p := range nodes {
			if err = tree.render(t.Context(), i); err != nil {
				t.Fatal(err)
			}
			got := tree.nodes[i]
			rendered := renderTree(readDescendants(nodes, p.ID), p.ID)
			own, gaps := readOwnText(nodes, p)
			if got.text != string(rendered.text) || got.own != own || got.ambiguous != (len(gaps) > 0) || got.citation != citation(readAncestry(nodes, p)) {
				t.Fatalf("trial %d node %d: tree output differs", trial, i)
			}
			leads, leadGaps, leadErr := tree.leads(t.Context(), i)
			want, wantGaps := readLeads(nodes, p)
			if leadErr != nil || !reflect.DeepEqual(leads, want) || !reflect.DeepEqual(leadGaps, wantGaps) {
				t.Fatalf("trial %d node %d: leads differ: %v", trial, i, leadErr)
			}
		}
	}
}
