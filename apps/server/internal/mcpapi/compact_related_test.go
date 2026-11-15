package mcpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"danny.vn/trachuan/internal/retrieval"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCP_CompactRelated(t *testing.T) {
	for _, cached := range []bool{false, true} {
		t.Run(map[bool]string{false: "uncached", true: "cached"}[cached], func(t *testing.T) {
			var caches *Caches
			if cached {
				caches = NewCaches(time.Now)
				caches.ActivateRelease("20991111", 1)
			}
			h := newHarness(t, func(c *Config) { c.OmitCompactRelated = true; c.Caches = caches })
			doc := retrieval.DocumentEntry{Related: []retrieval.Pointer{{Dir: "in", Kind: "repeals", InRelease: true}}, RelatedCounts: map[string]int{"in:repeals": 2}}
			h.engine.presented = &retrieval.SearchResult{Release: "20991111", Documents: map[string]retrieval.DocumentEntry{"vn:one": doc, "vn:two": doc}, Hits: []retrieval.PresentedHit{{Handle: "vn:one#a", Doc: "vn:one", Complete: true}}, NonCurrent: []retrieval.PresentedHit{{Handle: "vn:two#b", Doc: "vn:two", Complete: true}}}
			args := map[string]any{"country": "vn", "query": "ngân hàng"}
			for i := 0; i < 2; i++ {
				out := resultJSON(t, callTool(t, h, "search", args))
				for key, value := range out["documents"].(map[string]any) {
					entry := value.(map[string]any)
					if _, exists := entry["related"]; exists || entry["related_counts"].(map[string]any)["in:repeals"] != float64(2) {
						t.Fatalf("%s: %v", key, entry)
					}
				}
			}
			if cached && caches.Search.Stats().Hits == 0 {
				t.Fatal("cache hit not exercised")
			}
			args["detail"] = "full"
			full := resultJSON(t, callTool(t, h, "search", args))
			for _, value := range full["documents"].(map[string]any) {
				if _, ok := value.(map[string]any)["related"]; !ok {
					t.Fatal(value)
				}
			}
			if cached {
				ops := NewOperations(Config{Resolver: h.resolver, Caches: caches, OmitCompactRelated: true})
				raw, err := ops.Search(context.Background(), json.RawMessage(`{"country":"vn","query":"ngân hàng"}`))
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Contains(raw, []byte(`"related":`)) {
					t.Fatalf("web response lost pointers: %s", raw)
				}
			}
		})
	}
}

func TestMCP_CompactRelatedKeepMatchesPresenter(t *testing.T) {
	h := newHarness(t, nil)
	h.engine.presented = &retrieval.SearchResult{Release: "20991111", Documents: map[string]retrieval.DocumentEntry{"vn:one": {Related: []retrieval.Pointer{{Dir: "in", Kind: "repeals"}}, RelatedCounts: map[string]int{"in:repeals": 1}}}}
	want, _, err := encodeResult(h.engine.presented, false)
	if err != nil {
		t.Fatal(err)
	}
	got := callTool(t, h, "search", map[string]any{"country": "vn", "query": "ngân hàng"})
	if text := got.Content[0].(*mcp.TextContent).Text; text != want.Content[0].(*mcp.TextContent).Text {
		t.Fatalf("got %s want %s", text, want.Content[0].(*mcp.TextContent).Text)
	}
}

func TestMCP_ReadRelatedWithOmit(t *testing.T) {
	h := newHarness(t, func(c *Config) { c.OmitCompactRelated = true })
	h.engine.readResult = &retrieval.ReadResult{Release: "20991111", Items: []retrieval.ItemResult{{Doc: "vn:one", Document: &retrieval.DocumentEntry{Related: []retrieval.Pointer{{Dir: "in", Kind: "repeals"}}, RelatedCounts: map[string]int{"in:repeals": 1}}, Complete: true}}}
	out := resultJSON(t, callTool(t, h, "read", map[string]any{"country": "vn", "items": []any{map[string]any{"handle": "vn:one"}}}))
	doc := out["items"].([]any)[0].(map[string]any)["document"].(map[string]any)
	if _, ok := doc["related"]; !ok {
		t.Fatal(doc)
	}
	baseline := newHarness(t, nil)
	baseline.engine.readResult = h.engine.readResult
	args := map[string]any{"country": "vn", "items": []any{map[string]any{"handle": "vn:one"}}}
	want := callTool(t, baseline, "read", args).Content[0].(*mcp.TextContent).Text
	got := callTool(t, h, "read", args).Content[0].(*mcp.TextContent).Text
	if got != want {
		t.Fatalf("read changed: got %s want %s", got, want)
	}
}
