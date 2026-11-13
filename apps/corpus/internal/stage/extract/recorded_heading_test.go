package extract

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/vbpl"
)

type recordedHeading struct {
	Key  string          `json:"key"`
	Tree json.RawMessage `json:"tree"`
	HTML string          `json:"html"`
}

func recordedHeadingFixture(t *testing.T) recordedHeading {
	t.Helper()
	data, err := os.ReadFile("testdata/heading-5664.json")
	if err != nil {
		t.Fatal(err)
	}
	var f recordedHeading
	if err = json.Unmarshal(data, &f); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestSelection_RecordedHeadingRules(t *testing.T) {
	f := recordedHeadingFixture(t)
	for _, tc := range []struct {
		name   string
		mutate func([]vbpl.Node, string) ([]vbpl.Node, string)
		rule   int
	}{
		{name: "matching"},
		{name: "rejected", rule: 1, mutate: func(ns []vbpl.Node, s string) ([]vbpl.Node, string) {
			return ns, strings.ReplaceAll(s, "Điều 2.", "Điều 3.")
		}},
		{name: "anchors differ", rule: 2, mutate: func(ns []vbpl.Node, s string) ([]vbpl.Node, string) { return ns, s + "<p>Điều 3. Thi hành</p>" }},
		{name: "article order", rule: 2, mutate: func(ns []vbpl.Node, s string) ([]vbpl.Node, string) {
			ns[0], ns[1] = ns[1], ns[0]
			return ns, s
		}},
		{name: "duplicate suffix", rule: 2, mutate: func(ns []vbpl.Node, s string) ([]vbpl.Node, string) {
			n := ns[0]
			n.ID = "duplicate"
			return append(ns, n), s
		}},
		{name: "changed number", rule: 3, mutate: func(ns []vbpl.Node, s string) ([]vbpl.Node, string) {
			return ns, strings.ReplaceAll(s, "5/12/2000", "6/12/2000")
		}},
		{name: "dropped word", rule: 3, mutate: func(ns []vbpl.Node, s string) ([]vbpl.Node, string) {
			return ns, strings.ReplaceAll(s, "hiệu lực thi hành", "hiệu lực hành")
		}},
		{name: "reordered clauses", rule: 3, mutate: func(ns []vbpl.Node, s string) ([]vbpl.Node, string) {
			start := strings.Index(s, "<p>1. Quyết")
			next := strings.Index(s, "<p>2. Thủ")
			return ns, s[:start] + s[next:] + s[start:next]
		}},
		{name: "missing clause anchor", rule: 4, mutate: func(ns []vbpl.Node, s string) ([]vbpl.Node, string) {
			ns[1].Children[1].Title = "Khoản 3"
			return ns, s
		}},
		{name: "missing point anchor", rule: 4, mutate: func(ns []vbpl.Node, s string) ([]vbpl.Node, string) {
			ns[1].Children[1].Level = "Point"
			ns[1].Children[1].Title = "Điểm a"
			return ns, s
		}},
		{name: "discarded section title", rule: 5, mutate: func(ns []vbpl.Node, s string) ([]vbpl.Node, string) {
			section := recordedSection(t)
			section.Children = ns
			return []vbpl.Node{section}, "<table><tr><td>Bảng " + section.Content + "</td></tr></table>" + s
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			nodes, err := vbpl.DecodeTree(context.Background(), strings.NewReader(string(f.Tree)), vbpl.MaxTreeDepth, vbpl.MaxTreeNodes)
			if err != nil {
				t.Fatal(err)
			}
			html := f.HTML
			if tc.mutate != nil {
				nodes, html = tc.mutate(nodes, html)
			}
			tree, err := json.Marshal(map[string]any{"success": true, "data": wireNodes(nodes)})
			if err != nil {
				t.Fatal(err)
			}
			selected, kept, err := choose(context.Background(), transcriptionBundle(t, string(tree), html), "vn:vbpl:1")
			if err != nil {
				t.Fatal(err)
			}
			if tc.rule != 0 {
				if selected.source != "source_tree" || selected.gap != "tree_headings_partial" || !strings.Contains(selected.reason, "rule "+string(rune('0'+tc.rule))) || len(kept) == 0 {
					t.Fatalf("source=%s gap=%s detail=%s nodes=%d", selected.source, selected.gap, selected.reason, len(kept))
				}
				return
			}
			if selected.source != "html_transcription" || len(kept) != 0 || selected.tree == nil || !selected.tree.SetAside || selected.tree.HeadingOnlyArticles != 1 {
				t.Fatalf("source=%s comparison=%+v nodes=%d", selected.source, selected.tree, len(kept))
			}
			if selected.tree.Rules != [5]ruleCount{{1, 1}, {2, 2}, {1, 1}, {2, 2}, {0, 0}} {
				t.Fatalf("rules=%+v", selected.tree.Rules)
			}
		})
	}
}

func wireNodes(nodes []vbpl.Node) []map[string]any {
	out := make([]map[string]any, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, map[string]any{"id": n.ID, "title": n.Title, "level": n.Level, "pType": n.PType, "content": map[string]string{"content": n.Content}, "children": wireNodes(n.Children)})
	}
	return out
}

func recordedSection(t *testing.T) vbpl.Node {
	t.Helper()
	data, err := os.ReadFile("testdata/heading-section.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Node json.RawMessage `json:"node"`
	}
	if err = json.Unmarshal(data, &f); err != nil {
		t.Fatal(err)
	}
	nodes, err := vbpl.DecodeTree(context.Background(), strings.NewReader(`{"success":true,"data":[`+string(f.Node)+`]}`), vbpl.MaxTreeDepth, vbpl.MaxTreeNodes)
	if err != nil {
		t.Fatal(err)
	}
	return nodes[0]
}

func TestSelection_RecordedAllHeadingArticlesWithSection(t *testing.T) {
	f := recordedHeadingFixture(t)
	nodes, err := vbpl.DecodeTree(context.Background(), strings.NewReader(string(f.Tree)), vbpl.MaxTreeDepth, vbpl.MaxTreeNodes)
	if err != nil {
		t.Fatal(err)
	}
	for i := range nodes {
		nodes[i].Content = nodes[i].Title
		nodes[i].Children = nil
	}
	section := recordedSection(t)
	section.Children = nodes
	rows, _, err := extractTree(context.Background(), []vbpl.Node{section})
	if err != nil {
		t.Fatal(err)
	}
	if headingOnly(rows) {
		t.Fatal("fixture passes the legacy whole-tree test")
	}
	tree, err := json.Marshal(map[string]any{"success": true, "data": wireNodes([]vbpl.Node{section})})
	if err != nil {
		t.Fatal(err)
	}
	for _, withText := range []bool{true, false} {
		html := ""
		if withText {
			html = "<p>" + section.Content + "</p>" + f.HTML
		}
		dir := transcriptionBundle(t, string(tree), html)
		if !withText {
			addCheckedPDF(t, dir, ocrFixture(t, true, 0))
		}
		selected, kept, err := chooseWithOCR(context.Background(), dir, "vn:vbpl:1", ocrFixture(t, true, 0), AcceptedEngines(), "checker")
		if err != nil {
			t.Fatal(err)
		}
		if withText {
			if selected.source != "html_transcription" || selected.tree == nil || selected.tree.HeadingOnlyArticles != 2 {
				t.Fatalf("selection=%+v", selected)
			}
		} else if selected.source != "source_tree" || selected.gap != "tree_headings_only" || selected.consulted || len(kept) == 0 {
			t.Fatalf("selection=%+v nodes=%d", selected, len(kept))
		}
	}
}

func TestSelection_RecordedArticleLeadIncludesChildren(t *testing.T) {
	f := recordedHeadingFixture(t)
	nodes, err := vbpl.DecodeTree(context.Background(), strings.NewReader(string(f.Tree)), vbpl.MaxTreeDepth, vbpl.MaxTreeNodes)
	if err != nil {
		t.Fatal(err)
	}
	nodes[1].Content = nodes[1].Title
	tree, err := json.Marshal(map[string]any{"success": true, "data": wireNodes(nodes)})
	if err != nil {
		t.Fatal(err)
	}
	selected, _, err := choose(context.Background(), transcriptionBundle(t, string(tree), f.HTML), "vn:vbpl:1")
	if err != nil || selected.source != "html_transcription" {
		t.Fatalf("selection=%+v err=%v", selected, err)
	}
}

func TestSelection_RecordedAmbiguousSubtreeCannotHideChangedChild(t *testing.T) {
	f := recordedHeadingFixture(t)
	nodes, err := vbpl.DecodeTree(context.Background(), strings.NewReader(string(f.Tree)), vbpl.MaxTreeDepth, vbpl.MaxTreeNodes)
	if err != nil {
		t.Fatal(err)
	}
	// The parent holds only one child. Chunk preserves the unmatched child as a
	// separate span, so agreement must not silently drop that child's text.
	nodes[1].Content = nodes[1].Title + "<br>" + nodes[1].Children[0].Content
	nodes[1].Children[1].Content = strings.ReplaceAll(nodes[1].Children[1].Content, "chịu trách nhiệm", "không chịu trách nhiệm")
	tree, err := json.Marshal(map[string]any{"success": true, "data": wireNodes(nodes)})
	if err != nil {
		t.Fatal(err)
	}
	s, _, err := choose(context.Background(), transcriptionBundle(t, string(tree), f.HTML), "vn:vbpl:1")
	if err != nil || s.source != "source_tree" || s.gap != "tree_headings_partial" || !strings.Contains(s.reason, "rule 3") {
		t.Fatalf("source=%s gap=%s detail=%s err=%v", s.source, s.gap, s.reason, err)
	}
}
