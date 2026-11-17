package mcpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"danny.vn/trachuan/internal/frameworks/serve"
)

func TestMCP_ResponsesAreNoStore(t *testing.T) {
	for _, tc := range []struct {
		name, tool, args, extra, wantCode string
		change                            func(*Config)
	}{
		{name: "status", tool: "corpus_status", args: `{"corpus":"frameworks"}`},
		{name: "search", tool: "search", args: `{"corpus":"frameworks","query":"invented"}`},
		{name: "read", tool: "read", args: `{"corpus":"frameworks","items":[{"handle":"fw:fw-alpha:1#A"}]}`},
		{name: "gaps", tool: "quality_gaps", args: `{"corpus":"frameworks"}`},
		{name: "mappings", tool: "mappings", args: `{"corpus":"frameworks","handle":"fw:fw-alpha:1#A"}`},
		{name: "entitlement unavailable", tool: "corpus_status", args: `{"corpus":"frameworks"}`, change: func(c *Config) {
			c.FrameworkAccess = func(context.Context, int64, []string) (serve.Caller, error) {
				return serve.Caller{AccessUnavailable: true}, nil
			}
		}},
		{name: "invalid input", tool: "search", args: `{"corpus":"frameworks","unknown":true}`, wantCode: "invalid_request"},
		{name: "malformed mappings arguments", tool: "mappings", args: `"invented"`, wantCode: "invalid_request"},
		{name: "release unavailable", tool: "corpus_status", args: `{"corpus":"frameworks"}`, wantCode: "unavailable", change: func(c *Config) { c.FrameworkResolver = nil }},
		{name: "rate limited", tool: "corpus_status", args: `{"corpus":"frameworks"}`, wantCode: "rate_limited", change: func(c *Config) { c.Limiter = &denyingLimiter{} }},
		{name: "mixed handles", tool: "read", args: `{"country":"vn","items":[{"handle":"vn:test#a"},{"handle":"fw:fw-alpha:1#A"}]}`, wantCode: "invalid_request"},
		{name: "hidden tool", tool: "mappings", args: `{"corpus":"frameworks","handle":"fw:fw-alpha:1#A"}`, wantCode: "protocol", change: func(c *Config) { c.Frameworks = false }},
		{name: "arguments casing", tool: "corpus_status", args: `{"corpus":"frameworks"}`, extra: `,"Arguments":{}`},
		{name: "law", tool: "corpus_status", args: `{"country":"vn"}`},
		{name: "law input error", tool: "search", args: `{"country":"vn","unknown":true}`, wantCode: "invalid_request"},
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
			body := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"` + tc.tool + `","arguments":` + tc.args + tc.extra + `}}`
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
			if got := response.Header.Get("Cache-Control"); got != "no-store" {
				t.Errorf("Cache-Control=%q want no-store", got)
			}
			if !strings.HasPrefix(response.Header.Get("Content-Type"), "application/json") {
				t.Error("SDK content type changed")
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
			if tc.name == "arguments casing" && message.Result.Structured["access_set_version"] != "from-engine" {
				t.Fatal("casing variant did not reach the framework engine")
			}
			if tc.name == "entitlement unavailable" && message.Result.Structured["access_unavailable"] != true {
				t.Fatal("missing entitlement fallback marker")
			}
		})
	}
}

func TestNoStore_StreamingAndImplicitResponses(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(map[bool]string{false: "implicit", true: "streaming"}[stream], func(t *testing.T) {
			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Cache-Control", "public, max-age=3600")
				w.Header().Set("Content-Type", "text/event-stream")
				w.Header().Set("X-Mcp-Test", "kept")
				if stream {
					flusher, ok := w.(http.Flusher)
					if !ok {
						t.Error("streaming is unavailable")
						return
					}
					flusher.Flush()
					if _, err := io.WriteString(w, "data: invented\n\n"); err != nil {
						t.Error(err)
					}
				}
			})
			server := httptest.NewServer(noStore(next))
			t.Cleanup(server.Close)
			response, err := server.Client().Get(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = response.Body.Close() }()
			if response.Header.Get("Cache-Control") != "no-store" || response.Header.Get("Content-Type") != "text/event-stream" || response.Header.Get("X-Mcp-Test") != "kept" {
				t.Fatalf("unexpected response headers: %v", response.Header)
			}
			body, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatal(err)
			}
			want := ""
			if stream {
				want = "data: invented\n\n"
			}
			if string(body) != want {
				t.Fatalf("body=%q want=%q", body, want)
			}
		})
	}
}

func TestMCP_UnauthenticatedResponseIsNoStore(t *testing.T) {
	server := httptest.NewServer(New(Config{}))
	t.Cleanup(server.Close)
	response, err := server.Client().Post(server.URL, "application/json", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusUnauthorized || response.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("status=%d Cache-Control=%q", response.StatusCode, response.Header.Get("Cache-Control"))
	}
	if response.Header.Get("WWW-Authenticate") != `Bearer error="invalid_token"` {
		t.Fatal("authentication challenge changed")
	}
}
