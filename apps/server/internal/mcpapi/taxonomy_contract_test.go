package mcpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/internal/retrieval"
)

func TestTaxonomyAndSearchSchemas(t *testing.T) {
	h := newHarness(t, nil)
	listed, err := h.session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tool := range listed.Tools {
		if tool.Name == "taxonomy" {
			found = true
		}
	}
	if !found {
		t.Fatal("taxonomy tool missing")
	}
	result := callTool(t, h, "search", map[string]any{"country": "vn", "query": "ngân hàng", "nodes": []string{"1"}, "cursor": "next"})
	if result.IsError {
		t.Fatal(resultJSON(t, result))
	}
}

func (e *fakeEngine) Taxonomy(_ context.Context, req retrieval.TaxonomyRequest) (retrieval.TaxonomyResult, error) {
	e.calls.Add(1)
	if e.err != nil {
		return retrieval.TaxonomyResult{}, e.err
	}
	return retrieval.TaxonomyResult{Release: "20991111", Nodes: []retrieval.NodeEntry{{Code: "1", Name: "Name", Level: 1, Provisions: 2, Documents: 1}}, Truncated: true}, nil
}
func (e *fakeEngine) TaxonomyNode(context.Context, string) (retrieval.NodePage, error) {
	return retrieval.NodePage{}, e.err
}

func TestTaxonomy_ResultAndErrors(t *testing.T) {
	h := newHarness(t, nil)
	out := resultJSON(t, callTool(t, h, "taxonomy", map[string]any{"country": "vn", "node": "1", "depth": 2, "q": "Name"}))
	if out["release"] != "20991111" || out["truncated"] != true || out["nodes"].([]any)[0].(map[string]any)["name"] != "Name" {
		t.Fatal(out)
	}
	for _, tc := range []struct {
		err  error
		code string
	}{{retrieval.ErrInvalidRequest, "invalid_request"}, {retrieval.ErrNotFound, "not_found"}, {retrieval.ErrReleaseExpired, "release_expired"}} {
		h.engine.err = fmt.Errorf("safe context: %w", tc.err)
		wantError(t, callTool(t, h, "taxonomy", map[string]any{"country": "vn"}), tc.code)
	}
}
func TestSearchCache_FilterAndCursorKeys(t *testing.T) {
	h, cache := cachedHarness(t, time.Now)
	for _, args := range []map[string]any{
		{"country": "vn", "query": "ngân hàng", "nodes": []string{"1"}},
		{"country": "vn", "query": "ngân hàng", "nodes": []string{"2"}},
		{"country": "vn", "query": "ngân hàng", "nodes": []string{"1"}, "cursor": "next"},
		{"country": "vn", "query": "ngân hàng", "documents": []string{"vn:é"}},
		{"country": "vn", "query": "ngân hàng", "documents": []string{"vn:e\u0301"}},
	} {
		if r := callTool(t, h, "search", args); r.IsError {
			t.Fatal(resultJSON(t, r))
		}
	}
	if cache.Search.Stats().Misses != 5 {
		t.Fatal(cache.Search.Stats())
	}
}

func TestTaxonomy_InputBounds(t *testing.T) {
	h := newHarness(t, nil)
	for _, raw := range []string{`{"country":"vn","depth":0}`, `{"country":"vn","depth":4}`, `{"country":"vn","depth":null}`, `{"country":"vn","q":"` + strings.Repeat("đ", 101) + `"}`} {
		wantError(t, callTool(t, h, "taxonomy", json.RawMessage(raw)), "invalid_request")
	}
}

func TestSearch_InvalidFilterStillValidatedForAbstention(t *testing.T) {
	h := newHarness(t, nil)
	h.engine.err = retrieval.ErrInvalidRequest
	wantError(t, callTool(t, h, "search", map[string]any{"country": "vn", "query": "ngan hang", "nodes": []string{"999"}}), "invalid_request")
}

func TestSearch_FilterSchemasAndBounds(t *testing.T) {
	h := newHarness(t, nil)
	listed, err := h.session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range listed.Tools {
		if tool.Name != "search" {
			continue
		}
		raw, _ := json.Marshal(tool.InputSchema)
		var schema struct {
			Properties map[string]struct {
				Description string `json:"description"`
				MaxItems    int    `json:"maxItems"`
			} `json:"properties"`
		}
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Fatal(err)
		}
		for key, limit := range map[string]int{"nodes": 20, "doc_types": 10, "issuers": 10, "documents": 20} {
			p := schema.Properties[key]
			if p.MaxItems != limit || !strings.Contains(p.Description, "ordering preference") || !strings.Contains(p.Description, "never hides") {
				t.Fatalf("%s: %+v", key, p)
			}
			args := searchArgs()
			args[key] = make([]string, limit+1)
			wantError(t, callTool(t, h, "search", args), "invalid_request")
		}
	}
	args := searchArgs()
	args["cursor"] = strings.Repeat("đ", 1025)
	wantError(t, callTool(t, h, "search", args), "invalid_request")
}
