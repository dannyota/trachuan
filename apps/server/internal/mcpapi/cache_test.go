package mcpapi

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/internal/retrieval"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func cachedHarness(t *testing.T, clock func() time.Time) (*harness, *Caches) {
	t.Helper()
	caches := NewCaches(clock)
	caches.ActivateRelease("20991111", 1)
	h := newHarness(t, func(c *Config) { c.Caches = caches; c.Clock = clock })
	return h, caches
}
func searchArgs() map[string]any { return map[string]any{"country": "vn", "query": "ngân hàng"} }
func wireText(t *testing.T, r *mcp.CallToolResult) string {
	t.Helper()
	resultJSON(t, r)
	return r.Content[0].(*mcp.TextContent).Text
}
func TestSearchCache_HitIsByteIdenticalToMiss(t *testing.T) {
	h, c := cachedHarness(t, time.Now)
	miss := wireText(t, callTool(t, h, "search", searchArgs()))
	args := searchArgs()
	args["limit"] = 20
	args["detail"] = "compact"
	args["in_force_only"] = true
	args["release"] = "20991111"
	hit := wireText(t, callTool(t, h, "search", args))
	if miss != hit || h.engine.calls.Load() != 2 || c.Search.Stats().Hits != 1 {
		t.Fatal("cache bytes or defaults differ")
	}
	if h.resolver.done.Load() != 2 {
		t.Fatal("cache bypassed lease resolution")
	}
}
func TestSearchCache_MidnightInVietnamMisses(t *testing.T) {
	now := time.Date(2099, 1, 1, 16, 59, 59, 0, time.UTC)
	h, c := cachedHarness(t, func() time.Time { return now })
	callTool(t, h, "search", searchArgs())
	now = now.Add(time.Second)
	callTool(t, h, "search", searchArgs())
	if h.engine.calls.Load() != 4 || c.Search.Stats().Misses != 2 {
		t.Fatal("Vietnam midnight did not miss")
	}
}
func TestSearchCache_ReleaseSwitchDropsAndMisses(t *testing.T) {
	h, c := cachedHarness(t, time.Now)
	callTool(t, h, "search", searchArgs())
	if c.Search.Stats().Bytes == 0 {
		t.Fatal("miss not stored using lease release")
	}
	c.ActivateRelease("20991112", 2)
	if c.Search.Stats().Bytes != 0 {
		t.Fatal("switch retained search")
	}
	callTool(t, h, "search", searchArgs()) // An old lease can finish, but cannot store.
	if c.Search.Stats().Bytes != 0 {
		t.Fatal("old lease stored after switch")
	}
	c.ActivateRelease("20991111", 3)
	h.resolver.generation = 3
	callTool(t, h, "search", searchArgs())
	if h.engine.calls.Load() != 6 {
		t.Fatal("old value survived activation")
	}
}
func TestCache_HitChargesLimitsAndChecksCredentials(t *testing.T) {
	h, c := cachedHarness(t, time.Now)
	callTool(t, h, "search", searchArgs())
	// All sixty calls, including cache hits, consume the search bucket.
	for i := 1; i < 60; i++ {
		if callTool(t, h, "search", searchArgs()).IsError {
			t.Fatal("early limit")
		}
	}
	wantError(t, callTool(t, h, "search", searchArgs()), "rate_limited")
	if c.Search.Stats().Hits != 59 {
		t.Fatal(c.Search.Stats())
	}
	w := httptest.NewRecorder()
	h.server.Config.Handler.ServeHTTP(w, httptest.NewRequest("POST", "/mcp", strings.NewReader(`{}`)))
	if w.Code != 401 || c.Search.Stats().Hits != 59 {
		t.Fatal("anonymous request reached cache")
	}
}
func TestSearchCache_ErrorsNotCached(t *testing.T) {
	h, c := cachedHarness(t, time.Now)
	for _, stage := range []string{"search", "present"} {
		h.engine.err = nil
		h.engine.presentErr = nil
		if stage == "search" {
			h.engine.err = retrieval.ErrEmbedder
		} else {
			h.engine.presentErr = retrieval.ErrUnavailable
		}
		for i := 0; i < 2; i++ {
			if !callTool(t, h, "search", searchArgs()).IsError {
				t.Fatal("error lost")
			}
		}
		if c.Search.Stats().Bytes != 0 {
			t.Fatal("error cached")
		}
	}
	h.engine.presentErr = nil
	callTool(t, h, "search", searchArgs())
	if c.Search.Stats().Bytes == 0 {
		t.Fatal("recovery not cached")
	}
}
func TestCache_NoKeyOrTextInLogsOrMetrics(t *testing.T) {
	h, c := cachedHarness(t, time.Now)
	args := searchArgs()
	args["query"] = "ngân hàng private-cache-marker"
	callTool(t, h, "search", args)
	callTool(t, h, "search", args)
	metrics, _ := json.Marshal(c.Search.Stats())
	for _, s := range []string{h.logs.String(), string(metrics), fmt.Sprint(c.Search.Stats())} {
		if strings.Contains(s, "private-cache-marker") || strings.Contains(s, "query") || strings.Contains(s, "key") {
			t.Fatal("cache data exposed")
		}
	}
	var fields map[string]any
	_ = json.Unmarshal(metrics, &fields)
	if len(fields) != 4 {
		t.Fatal("unexpected metric labels")
	}
}
