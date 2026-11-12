package mcpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/credential"
	"danny.vn/trachuan/apps/server/internal/limits"
	"danny.vn/trachuan/apps/server/internal/release"
	"danny.vn/trachuan/apps/server/internal/store"
	"danny.vn/trachuan/internal/retrieval"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type fakeEngine struct {
	request    retrieval.Request
	detail     retrieval.Detail
	read       retrieval.ReadRequest
	err        error
	presented  *retrieval.SearchResult
	presentErr error
	calls      atomic.Int64
}

func (e *fakeEngine) Search(_ context.Context, r retrieval.Request) (retrieval.Response, error) {
	e.calls.Add(1)
	e.request = r
	return retrieval.Response{Release: "20991111"}, e.err
}
func (e *fakeEngine) Present(_ context.Context, _ retrieval.Response, d retrieval.Detail) (retrieval.SearchResult, error) {
	e.calls.Add(1)
	e.detail = d
	if e.presentErr != nil {
		return retrieval.SearchResult{}, e.presentErr
	}
	if e.presented != nil {
		return *e.presented, e.err
	}
	h := retrieval.PresentedHit{Handle: "vn:test#a", Complete: true}
	if d == retrieval.DetailFull {
		h.Text = "verbatim"
	} else {
		h.Snippet = "verbatim"
	}
	return retrieval.SearchResult{Release: "20991111", Hits: []retrieval.PresentedHit{h}}, e.err
}
func (e *fakeEngine) Read(_ context.Context, r retrieval.ReadRequest) (retrieval.ReadResult, error) {
	e.calls.Add(1)
	e.read = r
	return retrieval.ReadResult{Release: r.Release, Items: []retrieval.ItemResult{{Complete: false, Next: "opaque"}, {Complete: true}}}, e.err
}
func (e *fakeEngine) Status(context.Context) (retrieval.CorpusStatus, error) {
	e.calls.Add(1)
	return retrieval.CorpusStatus{Release: "20991111", Documents: 2}, e.err
}
func (e *fakeEngine) Gaps(context.Context, string, int64, int) ([]retrieval.Gap, int64, error) {
	e.calls.Add(1)
	return []retrieval.Gap{{Kind: "missing"}}, 3, e.err
}

type fakeResolver struct {
	engine       *fakeEngine
	err          error
	calls, done  atomic.Int64
	country, rel string
	generation   uint64
}

func (f *fakeResolver) Resolve(_ context.Context, country, rel string) (Lease, error) {
	f.calls.Add(1)
	f.country, f.rel = country, rel
	if f.err != nil {
		return Lease{}, f.err
	}
	return Lease{Engine: f.engine, Release: "20991111", Generation: f.generation, Done: func() { f.done.Add(1) }}, nil
}

type authStore struct{}

func (authStore) GetCredential(context.Context, string) (store.GetCredentialRow, error) {
	return store.GetCredentialRow{AccountID: 7, ExpiresAt: time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)}, nil
}
func (authStore) TouchPersonalToken(context.Context, string) error { return nil }

type authTransport struct{ base http.RoundTripper }

func (a authTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer trc_"+strings.Repeat("A", 43))
	return a.base.RoundTrip(r)
}

type harness struct {
	session  *mcp.ClientSession
	server   *httptest.Server
	resolver *fakeResolver
	engine   *fakeEngine
	logs     *bytes.Buffer
}

func newHarness(t *testing.T, change func(*Config)) *harness {
	t.Helper()
	e := &fakeEngine{}
	r := &fakeResolver{engine: e, generation: 1}
	logs := &bytes.Buffer{}
	cfg := Config{Resolver: r, Limiter: limits.New(time.Now), Logger: slog.New(slog.NewTextHandler(logs, nil)), Clock: time.Now}
	if change != nil {
		change(&cfg)
	}
	v := &credential.Verifier{Store: authStore{}, Audience: "https://example.test/mcp", Clock: func() time.Time { return time.Date(2098, 1, 1, 0, 0, 0, 0, time.UTC) }}
	ts := httptest.NewServer(credential.Middleware(v, New(cfg)))
	t.Cleanup(ts.Close)
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: ts.URL, HTTPClient: &http.Client{Transport: authTransport{http.DefaultTransport}}, DisableStandaloneSSE: true, MaxRetries: -1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return &harness{session: session, server: ts, resolver: r, engine: e, logs: logs}
}
func callTool(t *testing.T, h *harness, name string, args any) *mcp.CallToolResult {
	t.Helper()
	r, err := h.session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func resultJSON(t *testing.T, r *mcp.CallToolResult) map[string]any {
	t.Helper()
	if len(r.Content) != 1 {
		t.Fatalf("content blocks %d", len(r.Content))
	}
	text, ok := r.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatal("non-text content")
	}
	b, err := json.Marshal(r.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if text.Text != string(b) {
		t.Fatalf("text and structured JSON differ: %s / %s", text.Text, b)
	}
	var out map[string]any
	if err = json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}
func wantError(t *testing.T, r *mcp.CallToolResult, code string) {
	t.Helper()
	out := resultJSON(t, r)
	e, ok := out["error"].(map[string]any)
	if !r.IsError || !ok || e["code"] != code {
		t.Fatalf("got %+v, want %s", out, code)
	}
}

func TestMCP_ToolsListedWithSchemas(t *testing.T) {
	h := newHarness(t, nil)
	r, err := h.session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Tools) != 7 {
		t.Fatalf("tools %d", len(r.Tools))
	}
	names := map[string]bool{}
	for _, tool := range r.Tools {
		names[tool.Name] = true
		if tool.InputSchema == nil || tool.OutputSchema == nil {
			t.Fatal("missing schema")
		}
	}
	for _, name := range []string{"guide", "corpus_status", "taxonomy", "search", "read", "quality_gaps", "report_issue"} {
		if !names[name] {
			t.Fatal("missing " + name)
		}
	}
}
func TestMCP_SearchCompactDefault(t *testing.T) {
	h := newHarness(t, nil)
	r := callTool(t, h, "search", map[string]any{"country": "vn", "query": "ngân hàng"})
	out := resultJSON(t, r)
	if r.IsError || out["release"] != "20991111" || h.engine.request.Limit != 20 || h.engine.detail != "compact" {
		t.Fatalf("unexpected result: %+v", out)
	}
	hit := out["hits"].([]any)[0].(map[string]any)
	if hit["snippet"] != "verbatim" || hit["complete"] != true || hit["text"] != nil {
		t.Fatal(hit)
	}
	if h.resolver.done.Load() != 1 {
		t.Fatal("lease not released")
	}
}
func TestMCP_SearchFullDetail(t *testing.T) {
	h := newHarness(t, nil)
	r := callTool(t, h, "search", map[string]any{"country": "vn", "query": "ngân hàng", "detail": "full", "as_of": "2020-02-29", "in_force_only": false})
	out := resultJSON(t, r)
	if r.IsError || h.engine.request.Limit != 8 || h.engine.detail != "full" || h.engine.request.AsOf.Day != 29 || *h.engine.request.InForceOnly {
		t.Fatal(out)
	}
	hit := out["hits"].([]any)[0].(map[string]any)
	if hit["text"] != "verbatim" || hit["snippet"] != nil {
		t.Fatal(hit)
	}
}
func TestMCP_SearchRejectsUnknownFieldsAndBadLimit(t *testing.T) {
	h := newHarness(t, nil)
	for _, extra := range []string{`"limit":51`, `"limit":0`, `"limit":-1`, `"limit":1.5`, `"detail":"other"`, `"as_of":"2020-02-30"`, `"as_of":"0000-01-01"`, `"country":"us"`, `"query":""`, `"query":null`, `"limit":null`, `"Query":"private"`} {
		args := json.RawMessage(`{"country":"vn","query":"ngân hàng",` + extra + `}`)
		wantError(t, callTool(t, h, "search", args), "invalid_request")
	}
	if h.resolver.calls.Load() != 0 {
		t.Fatal("invalid inputs reached resolver")
	}
}
func TestMCP_ReadBatchAndNext(t *testing.T) {
	h := newHarness(t, nil)
	r := callTool(t, h, "read", map[string]any{"country": "vn", "release": "20991111", "items": []any{map[string]any{"handle": "vn:test#a"}, map[string]any{"next": "opaque"}}})
	out := resultJSON(t, r)
	if r.IsError || h.resolver.rel != "20991111" || h.engine.read.Release != "20991111" || h.engine.read.Items[1].Next != "opaque" || len(out["items"].([]any)) != 2 || h.resolver.done.Load() != 1 {
		t.Fatal(out)
	}
	items := make([]retrieval.ReadItem, 9)
	wantError(t, callTool(t, h, "read", map[string]any{"country": "vn", "items": items}), "invalid_request")
	if h.resolver.calls.Load() != 1 {
		t.Fatal("oversized batch reached resolver")
	}
}
func TestMCP_ReleaseExpired(t *testing.T) {
	for _, tt := range []struct{ name, tool string }{
		{"resolver", "search"}, {"search", "search"}, {"presentation", "search"},
		{"read", "read"}, {"status", "corpus_status"}, {"gaps", "quality_gaps"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, nil)
			switch tt.name {
			case "resolver":
				h.resolver.err = release.ErrExpired
			case "presentation":
				h.engine.presentErr = fmt.Errorf("metadata failure: %w", retrieval.ErrReleaseGone)
			default:
				h.engine.err = fmt.Errorf("database failure: %w", retrieval.ErrReleaseGone)
			}
			args := map[string]any{"country": "vn"}
			if tt.tool == "search" {
				args["query"] = "ngân hàng"
				args["release"] = "20990101"
			}
			if tt.tool == "read" {
				args["items"] = []any{map[string]any{"handle": "vn:test"}}
			}
			wantError(t, callTool(t, h, tt.tool, args), "release_expired")
			if tt.name != "resolver" && h.resolver.done.Load() != 1 {
				t.Fatal("error leaked lease")
			}
		})
	}
}

type denyingLimiter struct{ class limits.Class }

func (d *denyingLimiter) Allow(_ int64, c limits.Class) (bool, time.Duration) {
	d.class = c
	return false, 1500 * time.Millisecond
}
func TestMCP_RateLimited(t *testing.T) {
	d := &denyingLimiter{}
	h := newHarness(t, func(c *Config) { c.Limiter = d })
	for _, name := range []string{"search", "guide", "read", "corpus_status", "quality_gaps"} {
		r := callTool(t, h, name, map[string]any{"country": "vn"})
		wantError(t, r, "rate_limited")
		out := resultJSON(t, r)
		if out["error"].(map[string]any)["message"] != "Rate limited. Retry in 2 seconds." {
			t.Fatal(out)
		}
		want := limits.Read
		if name == "search" {
			want = limits.Search
		}
		if d.class != want {
			t.Fatal("wrong bucket")
		}
	}
	if h.resolver.calls.Load() != 0 {
		t.Fatal("denial reached resolver")
	}
}
func TestMCP_BodyCapAndBatchRejected(t *testing.T) {
	h := newHarness(t, nil)
	for _, tt := range []struct {
		name, body string
		status     int
	}{
		{"oversized", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"guide","arguments":{"country":"vn"}}}` + strings.Repeat(" ", 65536), 413},
		{"batch", `[{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"guide","arguments":{"country":"vn"}}}]`, 400},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for _, chunked := range []bool{false, true} {
				r := httptest.NewRequest("POST", "/mcp", strings.NewReader(tt.body))
				if chunked {
					r.ContentLength = -1
				}
				r.Header.Set("Authorization", "Bearer trc_"+strings.Repeat("A", 43))
				r.Header.Set("Accept", "application/json, text/event-stream")
				r.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				h.server.Config.Handler.ServeHTTP(w, r)
				if w.Code != tt.status {
					t.Fatalf("status %d: %s", w.Code, w.Body.String())
				}
			}
		})
	}
	if h.resolver.calls.Load() != 0 {
		t.Fatal("rejected input reached resolver")
	}
}
func TestMCP_NoQueryOrTokenInLogsOrErrors(t *testing.T) {
	h := newHarness(t, nil)
	private := "private-query-token"
	for cause, code := range map[error]string{fmt.Errorf("%s: %w", private, retrieval.ErrEmbedderBusy): "busy", fmt.Errorf("%s: %w", private, retrieval.ErrEmbedder): "inference_failed", fmt.Errorf("%s: %w", private, retrieval.ErrUnavailable): "unavailable", errors.New(private): "unavailable"} {
		h.engine.err = cause
		r := callTool(t, h, "search", map[string]any{"country": "vn", "query": "ngân hàng " + private})
		wantError(t, r, code)
		b, _ := json.Marshal(r)
		if strings.Contains(string(b), private) || strings.Contains(h.logs.String(), private) || strings.Contains(h.logs.String(), strings.Repeat("A", 43)) {
			t.Fatal("sensitive error or log")
		}
	}
	r := callTool(t, h, "search", json.RawMessage(`{"country":"vn","query":{"private-query-token":true}}`))
	wantError(t, r, "invalid_request")
	if strings.Contains(h.logs.String(), private) {
		t.Fatal("unsafe log")
	}
}
func TestGuide_ContentRules(t *testing.T) {
	h := newHarness(t, nil)
	r := callTool(t, h, "guide", map[string]any{"country": "vn"})
	out := resultJSON(t, r)
	guide, _ := out["guide"].(string)
	for _, rule := range []string{"search", "documents", "hits", "read", "<doc>#<anchor>", "status", "warnings", "repeals", "replaces", "suspends", "amends_supplements", "in:implements", "next", "last page", "snippet is not the provision", "[…]", "direction", "outside the release", "number lookup", "complete", "Vietnamese with diacritics", "citation", "number"} {
		if !strings.Contains(guide, rule) {
			t.Fatalf("guide missing %q", rule)
		}
	}
}
func TestMCP_MissingPrincipalFailsClosed(t *testing.T) {
	w := httptest.NewRecorder()
	New(Config{}).ServeHTTP(w, httptest.NewRequest("POST", "/mcp", strings.NewReader(`{}`)))
	if w.Code != 401 {
		t.Fatalf("status %d", w.Code)
	}
}
func TestMCP_MetadataTools(t *testing.T) {
	h := newHarness(t, nil)
	for _, name := range []string{"corpus_status", "quality_gaps"} {
		r := callTool(t, h, name, map[string]any{"country": "vn"})
		if r.IsError {
			t.Fatal(resultJSON(t, r))
		}
		out := resultJSON(t, r)
		b, _ := json.Marshal(out)
		if bytes.Contains(b, []byte("null")) {
			t.Fatal("null output")
		}
	}
	if h.resolver.done.Load() != 2 {
		t.Fatal("metadata leases leaked")
	}
}

func TestServer_ProductDisplayName(t *testing.T) {
	info := newHarness(t, nil).session.InitializeResult().ServerInfo
	if info.Name != "trachuan" || info.Title != "Tra Chuẩn" {
		t.Fatalf("server info: name %q title %q", info.Name, info.Title)
	}
}
