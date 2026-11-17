package mcpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"danny.vn/trachuan/internal/frameworks/serve"
)

func TestMCP_FrameworkResponsesAreNoStore(t *testing.T) {
	for _, tc := range []struct {
		name, tool, args, wantHeader, wantCode string
		change                                 func(*Config)
	}{
		{name: "status", tool: "corpus_status", args: `{"corpus":"frameworks"}`, wantHeader: "no-store"},
		{name: "search", tool: "search", args: `{"corpus":"frameworks","query":"invented"}`, wantHeader: "no-store"},
		{name: "read", tool: "read", args: `{"corpus":"frameworks","items":[{"handle":"fw:fw-alpha:1#A"}]}`, wantHeader: "no-store"},
		{name: "gaps", tool: "quality_gaps", args: `{"corpus":"frameworks"}`, wantHeader: "no-store"},
		{name: "mappings", tool: "mappings", args: `{"corpus":"frameworks","handle":"fw:fw-alpha:1#A"}`, wantHeader: "no-store"},
		{name: "entitlement unavailable", tool: "corpus_status", args: `{"corpus":"frameworks"}`, wantHeader: "no-store", change: func(c *Config) {
			c.FrameworkAccess = func(context.Context, int64, []string) (serve.Caller, error) {
				return serve.Caller{AccessUnavailable: true}, nil
			}
		}},
		{name: "invalid input", tool: "search", args: `{"corpus":"frameworks","unknown":true}`, wantHeader: "no-store", wantCode: "invalid_request"},
		{name: "malformed mappings arguments", tool: "mappings", args: `"invented"`, wantHeader: "no-store", wantCode: "invalid_request"},
		{name: "release unavailable", tool: "corpus_status", args: `{"corpus":"frameworks"}`, wantHeader: "no-store", wantCode: "unavailable", change: func(c *Config) { c.FrameworkResolver = nil }},
		{name: "rate limited", tool: "corpus_status", args: `{"corpus":"frameworks"}`, wantHeader: "no-store", wantCode: "rate_limited", change: func(c *Config) { c.Limiter = &denyingLimiter{} }},
		{name: "mixed handles", tool: "read", args: `{"country":"vn","items":[{"handle":"vn:test#a"},{"handle":"fw:fw-alpha:1#A"}]}`, wantHeader: "no-store", wantCode: "invalid_request"},
		{name: "hidden tool", tool: "mappings", args: `{"corpus":"frameworks","handle":"fw:fw-alpha:1#A"}`, wantHeader: "no-store", wantCode: "protocol", change: func(c *Config) { c.Frameworks = false }},
		{name: "law", tool: "corpus_status", args: `{"country":"vn"}`, wantHeader: "no-cache, no-transform"},
		{name: "law input error", tool: "search", args: `{"country":"vn","unknown":true}`, wantHeader: "no-cache, no-transform", wantCode: "invalid_request"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, func(c *Config) {
				c.Frameworks = true
				c.FrameworkResolver = &frameworkAdapter{}
				c.FrameworkAccess = func(context.Context, int64, []string) (serve.Caller, error) { return serve.Caller{}, nil }
				if tc.change != nil {
					tc.change(c)
				}
			})
			body := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"` + tc.tool + `","arguments":` + tc.args + `}}`
			r, err := http.NewRequestWithContext(t.Context(), "POST", h.server.URL, strings.NewReader(body))
			if err != nil {
				t.Fatal(err)
			}
			r.Header.Set("Accept", "application/json, text/event-stream")
			r.Header.Set("Content-Type", "application/json")
			client := &http.Client{Transport: authTransport{http.DefaultTransport}}
			response, err := client.Do(r)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = response.Body.Close() }()
			if got := response.Header.Get("Cache-Control"); got != tc.wantHeader {
				t.Errorf("Cache-Control=%q want=%q", got, tc.wantHeader)
			}
			data, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatal(err)
			}
			var message struct {
				Error  json.RawMessage `json:"error"`
				Result struct {
					IsError    bool           `json:"isError"`
					Structured map[string]any `json:"structuredContent"`
				} `json:"result"`
			}
			if json.Unmarshal(data, &message) != nil {
				t.Fatal("invalid MCP response")
			}
			if tc.wantCode == "protocol" {
				if message.Error == nil {
					t.Fatal("missing protocol error")
				}
			} else if tc.wantCode != "" {
				failure, _ := message.Result.Structured["error"].(map[string]any)
				if !message.Result.IsError || failure["code"] != tc.wantCode {
					t.Fatalf("unexpected tool error: %s", data)
				}
			} else if message.Result.IsError || message.Error != nil {
				t.Fatalf("unexpected failure: %s", data)
			}
			if tc.name == "entitlement unavailable" && message.Result.Structured["access_unavailable"] != true {
				t.Fatal("missing entitlement fallback marker")
			}
		})
	}
}
