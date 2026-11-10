package chunk

import (
	"strings"
	"testing"
	"unicode"
)

func fixtureTrees() map[string][]Node {
	article, chapter := "article", "chapter"
	lead := "A substantive introduction to these legal obligations."
	one := strings.Repeat("The first clause imposes a legal duty. ", 20)
	two := strings.Repeat("The second clause imposes a different duty. ", 20)
	return map[string][]Node{
		"small":      {{NodeID: "article", Kind: "dieu", Path: "dieu-1", Text: "A substantive article with sufficient text."}},
		"large":      {{NodeID: "article", Kind: "dieu", Path: "dieu-1", Text: lead}, {NodeID: "one", ParentNodeID: &article, Kind: "khoan", Path: "dieu-1/khoan-1", Text: one}, {NodeID: "two", ParentNodeID: &article, Kind: "khoan", Path: "dieu-1/khoan-2", Ordinal: 1, Text: two}},
		"contained":  {{NodeID: "article", Kind: "dieu", Path: "dieu-1", Text: lead + "\n" + one + "\n" + two}, {NodeID: "one", ParentNodeID: &article, Kind: "khoan", Path: "dieu-1/khoan-1", Text: one}, {NodeID: "two", ParentNodeID: &article, Kind: "khoan", Path: "dieu-1/khoan-2", Ordinal: 1, Text: two}},
		"ambiguous":  {{NodeID: "article", Kind: "dieu", Path: "dieu-1", Text: lead + "\n" + one}, {NodeID: "one", ParentNodeID: &article, Kind: "khoan", Path: "dieu-1/khoan-1", Text: one}, {NodeID: "two", ParentNodeID: &article, Kind: "khoan", Path: "dieu-1/khoan-2", Ordinal: 1, Text: two}},
		"outside":    {{NodeID: "chapter", Kind: "chuong", Path: "chuong-1", Text: "A substantive preamble outside the article."}, {NodeID: "article", ParentNodeID: &chapter, Kind: "dieu", Path: "dieu-1", Text: "A substantive article inside the chapter."}},
		"word":       {{NodeID: "article", Kind: "dieu", Path: "dieu-1", Text: strings.Repeat("x", 1100)}},
		"whitespace": {{NodeID: "article", Kind: "dieu", Path: "dieu-1", Text: " \n" + strings.Repeat("word\t", 300) + " \n"}},
		"omitted":    {{NodeID: "a", Kind: "dieu", Path: "dieu-1", Label: "Điều 1", Text: "Điều 1."}, {NodeID: "b", Kind: "dieu", Path: "dieu-2", Text: "Short duty"}},
	}
}

func splitFixture(t *testing.T, name string) (File, Coverage) {
	t.Helper()
	file, coverage, err := Split(fixtureTrees()[name], Meta{Key: "vn:vbpl:1"}, Recipe1())
	if err != nil {
		t.Fatal(err)
	}
	return file, coverage
}

func TestSplit_LargeArticleRecursesIntoClausesWithLead(t *testing.T) {
	file, _ := splitFixture(t, "large")
	if len(file.Chunks) < 2 {
		t.Fatalf("got %d chunks", len(file.Chunks))
	}
	for _, c := range file.Chunks {
		if c.NodeID != "one" && c.NodeID != "two" {
			t.Fatalf("unexpected owner %q", c.NodeID)
		}
		if !strings.Contains(c.Content, "A substantive introduction") {
			t.Fatalf("missing lead in %q", c.Content)
		}
	}
}

func TestSplit_LabelOnlyLeadNotCarried(t *testing.T) {
	nodes := fixtureTrees()["large"]
	nodes[0].Label, nodes[0].Text = "Điều 1", "Điều 1."
	file, _, err := Split(nodes, Meta{Key: "vn:vbpl:1"}, Recipe1())
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range file.Chunks {
		if strings.Contains(c.Content, "Điều 1.") {
			t.Fatal("label carried")
		}
	}
}

func TestSplit_ParentContainsChildrenUsesParentOnce(t *testing.T) {
	article := "a"
	child := "A substantive child provision that must appear once."
	nodes := []Node{{NodeID: article, Kind: "dieu", Path: "dieu-1", Text: "Substantive introduction.\n" + child}, {NodeID: "b", ParentNodeID: &article, Kind: "khoan", Path: "dieu-1/khoan-1", Text: child}}
	file, _, err := Split(nodes, Meta{Key: "vn:vbpl:1"}, Recipe1())
	if err != nil {
		t.Fatal(err)
	}
	if len(file.Chunks) != 1 || strings.Count(file.Chunks[0].Content, child) != 1 {
		t.Fatalf("unexpected chunks: %#v", file.Chunks)
	}
}

func TestSplit_AmbiguousContainmentGap(t *testing.T) {
	_, coverage := splitFixture(t, "ambiguous")
	for _, gap := range coverage.Gaps {
		if gap.Kind == "ambiguous_tree_text" {
			return
		}
	}
	t.Fatal("missing ambiguous_tree_text gap")
}

func TestSplit_LongLineShardsByWords(t *testing.T) {
	file, _ := splitFixture(t, "whitespace")
	if len(file.Chunks) < 2 {
		t.Fatalf("got %d chunks", len(file.Chunks))
	}
	for _, c := range file.Chunks {
		if strings.Contains(c.Content, "\t") || c.RoughTokens > 512 {
			t.Fatalf("invalid shard: %#v", c)
		}
	}
}

func TestSplit_OversizedWordStaysIntact(t *testing.T) {
	file, _ := splitFixture(t, "word")
	if len(file.Chunks) != 1 || file.Chunks[0].Content != strings.Repeat("x", 1100) {
		t.Fatalf("unexpected shards: %#v", file.Chunks)
	}
}

func TestSplit_NonArticleTextChunked(t *testing.T) {
	file, _ := splitFixture(t, "outside")
	if len(file.Chunks) != 2 || file.Chunks[0].NodeID != "chapter" || file.Chunks[1].NodeID != "article" {
		t.Fatalf("unexpected chunks: %#v", file.Chunks)
	}
}

func TestSplit_SpanSubstringsMatchSource(t *testing.T) {
	for name, nodes := range fixtureTrees() {
		t.Run(name, func(t *testing.T) {
			file, coverage, err := Split(nodes, Meta{Key: "vn:vbpl:1"}, Recipe1())
			if err != nil {
				t.Fatal(err)
			}
			source := make(map[string][]rune)
			accounted := make(map[string][]bool)
			for _, n := range nodes {
				source[n.NodeID] = []rune(n.Text)
				accounted[n.NodeID] = make([]bool, len([]rune(n.Text)))
			}
			mark := func(id string, start, end int) {
				t.Helper()
				flags, ok := accounted[id]
				if !ok || start < 0 || end < start || end > len(flags) {
					t.Fatalf("invalid range %s [%d,%d)", id, start, end)
				}
				for i := start; i < end; i++ {
					flags[i] = true
				}
			}
			for _, c := range file.Chunks {
				content := []rune(c.Content)
				for _, s := range c.Spans {
					mark(s.NodeID, s.Start, s.End)
					if s.ContentStart < 0 || s.ContentEnd < s.ContentStart || s.ContentEnd > len(content) {
						t.Fatalf("invalid content range: %#v", s)
					}
					if string(source[s.NodeID][s.Start:s.End]) != string(content[s.ContentStart:s.ContentEnd]) {
						t.Fatalf("span mismatch: %#v", s)
					}
				}
			}
			for _, o := range coverage.Omissions {
				mark(o.NodeID, o.Start, o.End)
			}
			for _, r := range coverage.CoveredRanges {
				mark(r.NodeID, r.Start, r.End)
			}
			for _, r := range coverage.WhitespaceRanges {
				mark(r.NodeID, r.Start, r.End)
			}
			for id, chars := range source {
				for i, r := range chars {
					if !unicode.IsSpace(r) && !accounted[id][i] {
						t.Fatalf("unaccounted %s offset %d", id, i)
					}
				}
			}
		})
	}
}

func TestPrefix_FieldsCapsAndDateFallback(t *testing.T) {
	chapter, section := "chapter", "section"
	nodes := []Node{{NodeID: chapter, Kind: "chuong", Path: "chuong-1", Label: "Chương I", Heading: strings.Repeat("h", 240)}, {NodeID: section, ParentNodeID: &chapter, Kind: "muc", Path: "muc-1", Label: "Mục 1", Heading: "Section heading"}, {NodeID: "article", ParentNodeID: &section, Kind: "dieu", Path: "dieu-1", Text: "A substantive article with sufficient text."}}
	meta := Meta{Key: "vn:vbpl:1", Number: strings.Repeat("n", 140), Title: strings.Repeat("t", 240), EffFrom: "invalid", IssueDate: "2024-01-01T20:00:00Z"}
	file, _, err := Split(nodes, meta, Recipe1())
	if err != nil {
		t.Fatal(err)
	}
	if len(file.Chunks) != 1 {
		t.Fatalf("got %d chunks", len(file.Chunks))
	}
	prefix := file.Chunks[0].ContextPrefix
	for _, want := range []string{strings.Repeat("n", 128), strings.Repeat("t", 220), "Chương I", "Mục 1 Section heading", "Ban hành: 2024-01-02"} {
		if !strings.Contains(prefix, want) {
			t.Fatalf("prefix missing %q: %q", want, prefix)
		}
	}
	if strings.Contains(prefix, strings.Repeat("n", 129)) || strings.Contains(prefix, strings.Repeat("t", 221)) {
		t.Fatalf("prefix exceeds cap: %q", prefix)
	}
}

func TestChunkStage_InputHashChangesWithFetchMetadata(t *testing.T) {
	first := inputHash("vn:vbpl:1", "fetch-one", "extract")
	second := inputHash("vn:vbpl:1", "fetch-two", "extract")
	if first == second {
		t.Fatal("fetch metadata change did not change input hash")
	}
}

func TestPrefix_UsesBuildDateNormalizer(t *testing.T) {
	for _, raw := range []string{"2020-07-10T00:00:00", "2020-07-10T01:00:00", "2020-07-10T07:00:00", "2020-07-10T17:00:00"} {
		if got := civilDate(raw); got != "2020-07-10" {
			t.Errorf("civilDate(%q) = %q", raw, got)
		}
	}
	if got := Recipe1().DateNormalizer; got != "corpus-normalizer/3" {
		t.Errorf("date normalizer = %q", got)
	}
}
