package retrieval

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func webCursor() cursor {
	return cursor{V: 1, Release: "20991110_1", Item: ReadItem{Handle: "vn:1"}, PageCap: 2_000_000, PartSize: 1_000_000, WholeDocument: true}
}

func collectWebPages(t *testing.T, seq readSequence, c cursor) []Unit {
	t.Helper()
	var units []Unit
	for page := 0; page < 30; page++ {
		result, _, err := readPage(seq, c, c.PageCap, true)
		if err != nil || result.Error != "" {
			t.Fatalf("page error: %v %s", err, result.Error)
		}
		used := 0
		if result.Document != nil {
			b, _ := json.Marshal(result.Document)
			used += len(b)
			if page != 0 {
				t.Fatal("repeated document entry")
			}
		}
		for _, u := range result.Units {
			b, _ := json.Marshal(u)
			if len(b) > c.PartSize {
				t.Fatalf("part exceeds byte cap: %d", len(b))
			}
			used += len(b)
		}
		if used > c.PageCap {
			t.Fatalf("page exceeds byte cap: %d", used)
		}
		units = append(units, result.Units...)
		if result.Next == "" {
			if !result.Complete {
				t.Fatal("incomplete last page")
			}
			return units
		}
		if result.Complete || len(result.Units) == 0 {
			t.Fatal("continuation did not advance")
		}
		next, err := decodeNext(result.Next)
		if err != nil {
			t.Fatal(err)
		}
		if next.Unit == c.Unit && next.Part == c.Part {
			t.Fatal("cursor did not advance")
		}
		c = next
	}
	t.Fatal("chain did not terminate")
	return nil
}

func TestDocuments_OversizedLeadAndProvisionAdvance(t *testing.T) {
	for _, kind := range []string{"lead", "provision"} {
		t.Run(kind, func(t *testing.T) {
			text := strings.Repeat("Điều kiện áp dụng. ", 120)
			seq := readSequence{doc: "vn:1", document: &DocumentEntry{Title: "title"}, units: []Unit{{Kind: kind, Text: text}}, complete: true}
			c := webCursor()
			c.PageCap, c.PartSize = 2000, 1000
			units := collectWebPages(t, seq, c)
			var joined strings.Builder
			for i, u := range units {
				if u.Part != i+1 || u.Parts != len(units) {
					t.Fatal("unstable parts")
				}
				joined.WriteString(u.Text)
			}
			if len(units) < 2 || joined.String() != text {
				t.Fatal("parts changed source text")
			}
		})
	}
}

func TestDocuments_OutlineLongerThanPageContinues(t *testing.T) {
	seq := readSequence{doc: "vn:1", document: &DocumentEntry{Title: "title"}, complete: true}
	for i := 0; i < 8; i++ {
		seq.units = append(seq.units, Unit{Kind: "outline", Heading: strings.Repeat("đ", 200)})
	}
	c := webCursor()
	c.PageCap, c.PartSize = 2000, 1000
	units := collectWebPages(t, seq, c)
	if len(units) != 8 {
		t.Fatalf("outline units: %d", len(units))
	}
}

func TestDocuments_TokensBindModeCapsAndKey(t *testing.T) {
	c := webCursor()
	c.Unit = 1
	req := ReadRequest{Release: c.Release, PageCap: c.PageCap, PartSize: c.PartSize, WholeDocument: true, DocumentKey: "vn:1"}
	for _, change := range []func(*ReadRequest){func(r *ReadRequest) { r.WholeDocument = false }, func(r *ReadRequest) { r.PageCap-- }, func(r *ReadRequest) { r.PartSize-- }, func(r *ReadRequest) { r.DocumentKey = "vn:2" }} {
		bad := req
		change(&bad)
		if _, code := requestCursor(ReadItem{Next: encodeNext(c)}, bad); code != "invalid_next" {
			t.Fatalf("accepted cross-scope token: %s", code)
		}
	}
	if _, code := requestCursor(ReadItem{Next: encodeNext(c)}, req); code != "" {
		t.Fatal(code)
	}
	mcp := c
	mcp.PageCap = 0
	mcp.PartSize = 0
	mcp.WholeDocument = false
	if _, code := requestCursor(ReadItem{Next: encodeNext(mcp)}, req); code != "invalid_next" {
		t.Fatal("accepted MCP token")
	}
}

func TestDocuments_OversizedEntryIsTerminal(t *testing.T) {
	c := webCursor()
	c.PageCap, c.PartSize = 2000, 1000
	seq := readSequence{doc: "vn:1", document: &DocumentEntry{Title: strings.Repeat("đ", 900)}, units: []Unit{{Kind: "outline", Heading: strings.Repeat("a", 500)}}, complete: true}
	result, cost, err := readPage(seq, c, c.PageCap, true)
	if err != nil || result.Error != "unit_too_large" || result.Next != "" || result.Document != nil || cost != 0 {
		t.Fatal("oversized entry plus first unit escaped cap")
	}
}

type documentHashTx struct{ readCacheTx }

func (tx *documentHashTx) QueryRow(_ context.Context, _ string, _ ...any) pgx.Row {
	tx.queries++
	end := time.Date(2099, 1, 2, 0, 0, 0, 0, time.UTC)
	return cacheRow{int64(1), "number", "title", "law", "issuer", nil, nil, &end, "", "in_force", "source", true, false, "source_tree", true, strings.Repeat("a", 64)}
}

func TestDocuments_WholeScopeAndCacheStaySeparateFromMCP(t *testing.T) {
	now := func() time.Time { return time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC) }
	cache := &memoryCache{}
	e := &Engine{release: Release{Rel: "20991110_1", RetrievalRecipe: Recipe2()}, options: Options{Now: now, Sequences: cache}}
	tx := &documentHashTx{}
	c := webCursor()
	cold, _, save, err := e.readItem(t.Context(), tx, c, false, c.PageCap, true)
	if err != nil || cold.FetchSHA256 != strings.Repeat("a", 64) || cold.Error != "" || cold.Document == nil || !cold.Document.Status.Current || len(cold.Units) != 2 || cold.Units[0].Kind != "outline" || cold.Units[1].Kind != "provision" || cold.Units[1].Text != strings.TrimSpace(strings.Repeat("điều luật ", 12000)) {
		t.Fatal("whole document omitted or reordered text", err)
	}
	if save == nil {
		t.Fatal("missing cache save")
	}
	save()
	queries := tx.queries
	warm, _, _, err := e.readItem(t.Context(), tx, c, false, c.PageCap, true)
	if err != nil || !reflect.DeepEqual(cold, warm) || tx.queries != queries {
		t.Fatal("cached whole document differs", err)
	}
	if warm.FetchSHA256 != cold.FetchSHA256 {
		t.Fatal("cache lost fetch hash")
	}
	paged := c
	paged.PageCap, paged.PartSize = 64_000, 32_000
	first, _, savePage, err := e.readItem(t.Context(), tx, paged, false, paged.PageCap, true)
	if err != nil || first.Next == "" || first.FetchSHA256 != cold.FetchSHA256 || savePage == nil {
		t.Fatal("first page missing hash or continuation", err)
	}
	savePage()
	continued, code := readCursor(ReadItem{Next: first.Next}, c.Release)
	if code != "" {
		t.Fatal(code)
	}
	queries = tx.queries
	second, _, _, err := e.readItem(t.Context(), tx, continued, true, continued.PageCap, true)
	if err != nil || second.FetchSHA256 != cold.FetchSHA256 || tx.queries != queries {
		t.Fatal("cached continuation lost hash or queried the database", err)
	}
	// The same document handle remains outline-only with MCP's original budgets.
	mcp := cursor{V: 1, Release: c.Release, Item: c.Item}
	outlined, _, _, err := e.readItem(t.Context(), tx, mcp, false, ReadCallCap, true)
	if err != nil || len(outlined.Units) != 1 || outlined.Units[0].Kind != "outline" || tx.queries == queries {
		t.Fatal("MCP reused whole-document cache", err)
	}
	changed := c
	changed.PartSize = 100_000
	split, _, _, err := e.readItem(t.Context(), tx, changed, false, changed.PageCap, true)
	if err != nil || len(split.Units) <= 2 || split.Units[1].Part != 1 {
		t.Fatal("part-size change reused cached parts", err)
	}
}

func TestDocuments_MultipleProvisionsAcrossPages(t *testing.T) {
	source := map[string]string{
		"a": strings.Repeat("Điều kiện áp dụng. ", 60) + "x",
		"b": strings.Repeat("Nghĩa vụ cụ thể. ", 60) + "x",
	}
	nodes := []Provision{
		{ID: 1, Identity: Identity{DocumentKey: "vn:1", Anchor: "a"}, Kind: "dieu", Text: " " + source["a"] + " "},
		{ID: 2, Identity: Identity{DocumentKey: "vn:1", Anchor: "b"}, Kind: "dieu", Text: " " + source["b"] + " "},
	}
	seq := readRange(readSequence{doc: "vn:1", document: &DocumentEntry{Title: "title"}, units: readOutline(nodes), complete: true}, nodes, 0, len(nodes)-1)
	c := cursor{V: 1, Release: "20991110_1", Item: ReadItem{Handle: "vn:1"}, PageCap: 1024, PartSize: 512, WholeDocument: true}
	joined := map[string]string{}
	partCounts := map[string]int{}
	pages := 0
	for ; pages < 10; pages++ {
		page, _, err := readPage(seq, c, c.PageCap, true)
		if err != nil || page.Error != "" {
			t.Fatalf("page error: %v %s", err, page.Error)
		}
		for _, u := range page.Units {
			if u.Kind == "provision" {
				joined[u.Anchor] += u.Text
				partCounts[u.Anchor]++
			}
		}
		if page.Next == "" {
			if !page.Complete {
				t.Fatal("incomplete chain")
			}
			break
		}
		c, err = decodeNext(page.Next)
		if err != nil {
			t.Fatal(err)
		}
	}
	if pages == 0 || pages == 10 {
		t.Fatal("expected multiple advancing pages")
	}
	for anchor, want := range source {
		if partCounts[anchor] < 2 {
			t.Fatalf("anchor %s was not split", anchor)
		}
		if joined[anchor] != want {
			t.Fatalf("anchor %s: got %d bytes, want %d", anchor, len(joined[anchor]), len(want))
		}
	}
}
