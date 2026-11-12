package webapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/credential"
	"danny.vn/trachuan/apps/server/internal/limits"
	"danny.vn/trachuan/apps/server/internal/mcpapi"
	"danny.vn/trachuan/apps/server/internal/release"
	"danny.vn/trachuan/apps/server/internal/store"
	"danny.vn/trachuan/apps/server/internal/websession"
	"danny.vn/trachuan/internal/retrieval"
)

type testEngine struct {
	calls atomic.Int64
	read  retrieval.ReadRequest
	err   error
}

func (e *testEngine) Search(context.Context, retrieval.Request) (retrieval.Response, error) {
	e.calls.Add(1)
	return retrieval.Response{Release: "20991110_1"}, e.err
}
func (e *testEngine) Present(context.Context, retrieval.Response, retrieval.Detail) (retrieval.SearchResult, error) {
	return retrieval.SearchResult{Release: "20991110_1", Hits: []retrieval.PresentedHit{{Handle: "vn:1#a", Doc: "vn:1", Snippet: "verbatim", Complete: true}}}, e.err
}
func (e *testEngine) Read(_ context.Context, r retrieval.ReadRequest) (retrieval.ReadResult, error) {
	e.read = r
	return retrieval.ReadResult{Release: r.Release, Nodes: map[string]retrieval.NodeRef{"1": {Name: "Name", Level: 1}}, Items: []retrieval.ItemResult{{Doc: "vn:1", Complete: true, Document: &retrieval.DocumentEntry{Title: "title", TextSource: "source_tree", TextComplete: true}, Units: []retrieval.Unit{{Kind: "outline", Anchor: "a"}, {Kind: "provision", Text: "source"}}}}}, e.err
}
func (e *testEngine) Status(context.Context) (retrieval.CorpusStatus, error) {
	return retrieval.CorpusStatus{Documents: 9007199254740993, Provisions: 3}, e.err
}
func (e *testEngine) Gaps(context.Context, string, int64, int) ([]retrieval.Gap, int64, error) {
	return nil, 0, e.err
}

type testResolver struct {
	engine mcpapi.Engine
	err    error
	done   atomic.Int64
}

func (r *testResolver) Resolve(context.Context, string, string) (mcpapi.Lease, error) {
	if r.err != nil {
		return mcpapi.Lease{}, r.err
	}
	return mcpapi.Lease{Engine: r.engine, Release: "20991110_1", Generation: 1, Done: func() { r.done.Add(1) }}, nil
}

type sessionStore struct {
	websession.Store
	now time.Time
}

func (s sessionStore) GetSession(context.Context, string) (store.GetSessionRow, error) {
	return store.GetSessionRow{ID: 1, AccountID: 7, CsrfSecret: strings.Repeat("B", 42) + "A", CreatedAt: s.now, LastSeenAt: s.now, ExpiresAt: s.now.Add(time.Hour)}, nil
}

type bearerStore struct{ now time.Time }

func (s bearerStore) GetCredential(context.Context, string) (store.GetCredentialRow, error) {
	return store.GetCredentialRow{AccountID: 7, ExpiresAt: s.now.Add(time.Hour)}, nil
}
func (bearerStore) TouchPersonalToken(context.Context, string) error { return nil }

type harness struct {
	web, mcp http.Handler
	engine   *testEngine
	resolver *testResolver
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	now := time.Date(2099, 11, 10, 0, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	e := &testEngine{}
	r := &testResolver{engine: e}
	caches := mcpapi.NewCaches(clock)
	caches.ActivateRelease("20991110_1", 1)
	cfg := mcpapi.Config{Resolver: r, Limiter: limits.New(clock), Clock: clock, Caches: caches}
	sessions := &websession.Manager{Store: sessionStore{now: now}, Clock: clock, PublicURL: "https://example.test"}
	verifier := &credential.Verifier{Store: bearerStore{now: now}, Clock: clock, Audience: "https://example.test/mcp"}
	return &harness{web: New(Config{Tools: cfg, Sessions: sessions}), mcp: credential.Middleware(verifier, mcpapi.New(cfg)), engine: e, resolver: r}
}
func webRequest(method, path, body string) *http.Request {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.AddCookie(&http.Cookie{Name: "__Host-session", Value: strings.Repeat("A", 43)})
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "https://example.test")
	r.Header.Set("X-CSRF-Token", strings.Repeat("B", 42)+"A")
	return r
}
func webCall(h *harness, method, path, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	h.web.ServeHTTP(w, webRequest(method, path, body))
	return w
}

func mcpCall(t *testing.T, h *harness, name, args string) []byte {
	t.Helper()
	body := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":%q,"arguments":%s}}`, name, args)
	r := httptest.NewRequest("POST", "/mcp", strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer trc_"+strings.Repeat("A", 43))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept", "application/json, text/event-stream")
	w := httptest.NewRecorder()
	h.mcp.ServeHTTP(w, r)
	var out struct {
		Result struct {
			StructuredContent json.RawMessage `json:"structuredContent"`
		} `json:"result"`
	}
	if json.Unmarshal(w.Body.Bytes(), &out) != nil || len(out.Result.StructuredContent) == 0 {
		t.Fatalf("MCP response %d: %s", w.Code, w.Body.String())
	}
	return out.Result.StructuredContent
}
func assertEqual(t *testing.T, name, path, args string) {
	t.Helper()
	h := newHarness(t)
	for i := 0; i < 3; i++ {
		w := webCall(h, "POST", path, args)
		want := mcpCall(t, h, name, args)
		if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), want) {
			t.Fatalf("different response %d: %s / %s", w.Code, w.Body.String(), want)
		}
	}
	if name == "search" && h.engine.calls.Load() != 1 {
		t.Fatal("search cache not shared")
	}
	if h.resolver.done.Load() != 6 {
		t.Fatal("lease leaked on cache hit")
	}
}
func TestWeb_SearchEqualsMCP(t *testing.T) {
	assertEqual(t, "search", "/api/v1/search", `{"country":"vn","query":"ngân hàng"}`)
}
func TestWeb_ReadEqualsMCP(t *testing.T) {
	assertEqual(t, "read", "/api/v1/read", `{"country":"vn","release":"20991110_1","items":[{"handle":"vn:1#a"}]}`)
}
func TestWeb_StatusEqualsMCP(t *testing.T) {
	h := newHarness(t)
	w := webCall(h, "GET", "/api/v1/status", "")
	want := mcpCall(t, h, "corpus_status", `{"country":"vn"}`)
	if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), want) {
		t.Fatalf("different status: %s / %s", w.Body.String(), want)
	}
}
func TestDocuments_IsReadWithWebCaps(t *testing.T) {
	h := newHarness(t)
	w := webCall(h, "GET", "/api/v1/documents/vn:1?release=20991110_1", "")
	r := h.engine.read
	if w.Code != 200 || r.PageCap != 2_000_000 || r.PartSize != 1_000_000 || !r.WholeDocument || r.DocumentKey != "vn:1" || r.Release != "20991110_1" || len(r.Items) != 1 || r.Items[0].Handle != "vn:1" {
		t.Fatalf("document request %d: %+v", w.Code, r)
	}
	var page struct {
		Release string               `json:"release"`
		Item    retrieval.ItemResult `json:"item"`
	}
	if !strings.Contains(w.Body.String(), `"nodes":{"1":{"level":1,"name":"Name"}}`) {
		t.Fatal(w.Body.String())
	}
	if json.Unmarshal(w.Body.Bytes(), &page) != nil || page.Release != "20991110_1" || page.Item.Doc != "vn:1" || !page.Item.Complete {
		t.Fatal(w.Body.String())
	}
}
func TestDocuments_EntryThenOutlineThenText(t *testing.T) {
	h := newHarness(t)
	w := webCall(h, "GET", "/api/v1/documents/vn:1", "")
	s := w.Body.String()
	if w.Code != 200 || strings.Index(s, `"document"`) > strings.Index(s, `"units"`) || strings.Index(s, `"outline"`) > strings.Index(s, `"provision"`) {
		t.Fatal(s)
	}
}
func TestDocuments_ReleaseExpired(t *testing.T) {
	h := newHarness(t)
	h.resolver.err = release.ErrExpired
	w := webCall(h, "GET", "/api/v1/documents/vn:1?release=20990101", "")
	if w.Code != 409 || !strings.Contains(w.Body.String(), `"code":"release_expired"`) {
		t.Fatal(w.Code, w.Body.String())
	}
}
func TestWeb_LimitsSharedWithMCP(t *testing.T) {
	for _, tc := range []struct {
		name, path, args string
		count            int
	}{{"search", "/api/v1/search", `{"country":"vn","query":"ngân hàng"}`, 60}, {"read", "/api/v1/read", `{"country":"vn","items":[]}`, 120}} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			for i := 0; i < tc.count; i++ {
				if i%2 == 0 {
					w := webCall(h, "POST", tc.path, tc.args)
					if w.Code != 200 {
						t.Fatal(w.Code)
					}
				} else if bytes.Contains(mcpCall(t, h, tc.name, tc.args), []byte(`"error"`)) {
					t.Fatal("early MCP denial")
				}
			}
			w := webCall(h, "POST", tc.path, tc.args)
			if w.Code != 429 || w.Header().Get("Retry-After") == "" {
				t.Fatal("web did not share bucket", w.Code)
			}
			if !bytes.Contains(mcpCall(t, h, tc.name, tc.args), []byte(`"code":"rate_limited"`)) {
				t.Fatal("MCP did not share bucket")
			}
			if tc.name == "read" {
				for _, path := range []string{"/api/v1/status", "/api/v1/documents/vn:1"} {
					if limited := webCall(h, "GET", path, ""); limited.Code != 429 {
						t.Fatal("GET route did not share read bucket", path, limited.Code)
					}
				}
			}
		})
	}
}
