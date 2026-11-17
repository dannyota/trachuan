package mcpapi

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"danny.vn/trachuan/internal/frameworks/serve"
)

func TestMCP_ReadMixedCorporaRejected(t *testing.T) {
	h := newHarness(t, nil)
	wantError(t, callTool(t, h, "read", map[string]any{
		"country": "vn", "items": []any{map[string]any{"handle": "vn:test#a"}, map[string]any{"handle": "fw:fw-alpha:1#A"}},
	}), "invalid_request")
	if h.resolver.calls.Load() != 0 {
		t.Fatal("mixed handles reached resolver")
	}
}

func TestMCP_LawReadKeepsMalformedFrameworkItemBehavior(t *testing.T) {
	h := newHarness(t, nil)
	result := callTool(t, h, "read", map[string]any{"country": "vn", "items": []any{map[string]any{"handle": "vn:test#a"}, map[string]any{"handle": "fw:bad"}}})
	if result.IsError || h.resolver.calls.Load() != 1 {
		t.Fatal("malformed framework item changed law read dispatch")
	}
}

func TestFrameworkErrors_FixedMessages(t *testing.T) {
	for _, tc := range []struct {
		err           error
		code, message string
	}{
		{serve.ErrInvalidRequest, "invalid_request", "Invalid request."},
		{serve.ErrInvalidHandle, "invalid_handle", "Invalid handle."},
		{serve.ErrReleaseExpired, "release_expired", "Release expired."},
		{serve.ErrCursorExpired, "cursor_expired", "Cursor expired."},
		{serve.ErrInvalidNext, "invalid_next", "Invalid continuation."},
		{serve.ErrUnavailable, "unavailable", "Service unavailable."},
		{serve.ErrRateLimited, "rate_limited", "Rate limited."},
	} {
		code, message := ErrorMessage(fmt.Errorf("invented private failure: %w", tc.err))
		if code != tc.code || message != tc.message {
			t.Fatalf("code=%s message=%s want=%s", code, message, tc.code)
		}
	}
}

func TestMCP_FrameworkToolsAndClosedSchemas(t *testing.T) {
	h := newHarness(t, func(c *Config) { c.Frameworks = true })
	listed, err := h.session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Tools) != 8 {
		t.Fatalf("tools=%d", len(listed.Tools))
	}
	for _, name := range []string{"corpus_status", "search", "read", "quality_gaps", "mappings"} {
		args := map[string]any{"corpus": "frameworks"}
		if name == "search" {
			args["query"] = "invented safeguard"
		}
		if name == "read" {
			args["items"] = []any{map[string]any{"handle": "fw:fw-alpha:1#A"}}
		}
		if name == "mappings" {
			args["handle"] = "fw:fw-alpha:1#A"
		}
		wantError(t, callTool(t, h, name, args), "unavailable")
		for _, extra := range []string{"country", "unknown"} {
			args[extra] = "vn"
			wantError(t, callTool(t, h, name, args), "invalid_request")
			delete(args, extra)
		}
	}
	guide := resultJSON(t, callTool(t, h, "guide", map[string]any{"country": "vn"}))["guide"].(string)
	for _, term := range []string{"frameworks", "licensed", "text_withheld", "mappings", "reduced"} {
		if !strings.Contains(guide, term) {
			t.Fatalf("guide missing %s", term)
		}
	}
}

func TestMCP_FrameworkFlagOffHidesOptions(t *testing.T) {
	h := newHarness(t, nil)
	listed, err := h.session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range listed.Tools {
		raw, marshalErr := json.Marshal(tool.InputSchema)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		if tool.Name == "mappings" || strings.Contains(string(raw), `"corpus"`) || strings.Contains(string(raw), `"frameworks"`) {
			t.Fatalf("frameworks exposed on %s", tool.Name)
		}
	}
	wantError(t, callTool(t, h, "corpus_status", map[string]any{"corpus": "frameworks"}), "invalid_request")
}

func TestMCP_FrameworkFlagKeepsLawResponses(t *testing.T) {
	a := newHarness(t, nil)
	b := newHarness(t, func(c *Config) { c.Frameworks = true })
	for _, op := range []struct {
		name string
		args map[string]any
	}{
		{"corpus_status", map[string]any{"country": "vn"}},
		{"quality_gaps", map[string]any{"country": "vn"}},
		{"search", map[string]any{"country": "vn", "query": "ngân hàng"}},
		{"search", map[string]any{"country": "vn", "query": "ngân hàng", "detail": "full"}},
		{"read", map[string]any{"country": "vn", "items": []any{map[string]any{"handle": "vn:test#a"}}}},
	} {
		first, err := json.Marshal(callTool(t, a, op.name, op.args))
		if err != nil {
			t.Fatal(err)
		}
		second, err := json.Marshal(callTool(t, b, op.name, op.args))
		if err != nil {
			t.Fatal(err)
		}
		if string(first) != string(second) {
			t.Fatalf("framework flag changed law %s", op.name)
		}
	}
}

func TestMCP_FrameworkRejectsNullFiltersAndBadSelectors(t *testing.T) {
	h := newHarness(t, func(c *Config) { c.Frameworks = true })
	for _, tc := range []struct{ name, args string }{
		{"search", `{"corpus":"frameworks","query":"quartz","frameworks":null}`},
		{"search", `{"corpus":"frameworks","query":"quartz","versions":null}`},
		{"quality_gaps", `{"corpus":"frameworks","versions":null}`},
		{"mappings", `{"corpus":"frameworks","handle":"fw:fw-alpha:1#A","relation":null}`},
		{"mappings", `{"corpus":"frameworks"}`},
		{"mappings", `{"corpus":"frameworks","from":"fw:fw-alpha:1"}`},
		{"mappings", `{"corpus":"frameworks","handle":"fw:fw-alpha:1#A","from":"fw:fw-alpha:1","to":"fw:fw-beta:1"}`},
	} {
		wantError(t, callTool(t, h, tc.name, json.RawMessage(tc.args)), "invalid_request")
	}
}
