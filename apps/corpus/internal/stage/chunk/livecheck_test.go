//go:build livecheck

package chunk

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// TestLiveSplitInvariants splits every extract artifact under LIVE_ARTIFACTS
// and checks that every span quotes its source text exactly. Run it by hand:
// LIVE_ARTIFACTS=work/artifacts go test -tags livecheck -run TestLiveSplitInvariants ./apps/corpus/internal/stage/chunk/
func TestLiveSplitInvariants(t *testing.T) {
	root := os.Getenv("LIVE_ARTIFACTS")
	if root == "" {
		t.Skip("LIVE_ARTIFACTS is not set")
	}
	dirs, _ := filepath.Glob(filepath.Join(root, "extract", "*"))
	var docs, empty, errs, chunks, spans, bad int
	gaps := map[string]int{}
	var sizes []int
	for _, dir := range dirs {
		data, err := os.ReadFile(filepath.Join(dir, "provisions.json"))
		if err != nil {
			t.Fatal(err)
		}
		var nodes []Node
		if err = json.Unmarshal(data, &nodes); err != nil {
			t.Fatal(err)
		}
		docs++
		if len(nodes) == 0 {
			empty++
		}
		file, cov, err := Split(nodes, Meta{Key: "vn:vbpl:x", Number: "1/2024/TT-X", Title: "T"}, Recipe1())
		if err != nil {
			errs++
			if errs <= 5 {
				t.Logf("%s: %v", filepath.Base(dir)[:12], err)
			}
			continue
		}
		text := map[string][]rune{}
		for _, n := range nodes {
			text[n.NodeID] = []rune(n.Text)
		}
		for _, g := range cov.Gaps {
			gaps[g.Kind]++
		}
		for _, c := range file.Chunks {
			chunks++
			sizes = append(sizes, c.RoughTokens)
			content := []rune(c.Content)
			for _, s := range c.Spans {
				spans++
				src := text[s.NodeID]
				if s.Start < 0 || s.End > len(src) || s.ContentEnd > len(content) || string(src[s.Start:s.End]) != string(content[s.ContentStart:s.ContentEnd]) {
					bad++
					if bad <= 5 {
						t.Logf("span mismatch in %s chunk %d", filepath.Base(dir)[:12], c.Ordinal)
					}
				}
			}
		}
	}
	sort.Ints(sizes)
	p := func(q float64) int {
		if len(sizes) == 0 {
			return 0
		}
		return sizes[int(q*float64(len(sizes)-1))]
	}
	t.Logf("docs=%d empty=%d errors=%d chunks=%d spans=%d bad_spans=%d rough p50=%d p95=%d max=%d gaps=%v", docs, empty, errs, chunks, spans, bad, p(0.5), p(0.95), p(1), gaps)
	if bad > 0 || errs > 0 {
		t.Fail()
	}
}
