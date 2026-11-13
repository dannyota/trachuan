//go:build e2e

package e2e

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"danny.vn/trachuan/internal/retrieval"
)

func TestSearch_Recipe3MergedArticle(t *testing.T) {
	if os.Getenv("E2E_RECIPE3") != "1" {
		t.Skip("requires E2E_RECIPE3=1 fixture")
	}
	c := client(t)
	login(t, c, "siblings@example.invalid", true)
	server := base(t, "E2E_SERVER_URL")
	token := personalToken(t, c, server)
	call := func(name string, args any, out any) {
		t.Helper()
		body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": name, "arguments": args}})
		if err != nil {
			t.Fatal(err)
		}
		var result struct {
			IsError    bool            `json:"isError"`
			Structured json.RawMessage `json:"structuredContent"`
		}
		oauthRPC(t, c, server, token, string(body), &result)
		if result.IsError || json.Unmarshal(result.Structured, out) != nil {
			t.Fatal("MCP tool failed")
		}
	}
	query := "Văn bản thử nghiệm\nĐiều kiện giả lập cho các khoản.\nKhoản hai có dấu hiệu hợp nhất màu ngọc."
	var full retrieval.SearchResult
	call("search", map[string]any{"country": "vn", "query": query, "detail": "full", "limit": 20}, &full)
	var hit *retrieval.PresentedHit
	for i := range full.Hits {
		if full.Hits[i].Handle == "vn:fixture:siblings#dieu-1" {
			hit = &full.Hits[i]
		}
	}
	if hit == nil {
		t.Fatal("missing merged article")
	}
	if hit.Complete || hit.Next == "" || !strings.Contains(hit.Text, "Khoản đầu") || !strings.Contains(hit.Text, "Khoản hai") {
		t.Fatal("missing merged evidence or continuation")
	}
	complete := map[string]bool{}
	for _, f := range hit.Fragments {
		if f.Complete {
			complete[f.Identity.Anchor] = true
		}
	}
	if !complete["clause-1"] || !complete["clause-2"] || complete["clause-3"] {
		t.Fatal("wrong constituent fragments")
	}
	var compact retrieval.SearchResult
	call("search", map[string]any{"country": "vn", "query": query, "detail": "compact", "limit": 20}, &compact)
	found := false
	for _, h := range compact.Hits {
		if h.Handle == hit.Handle {
			found = true
			if !strings.Contains(h.Snippet, "Khoản hai có dấu hiệu hợp nhất màu ngọc.") || h.Complete {
				t.Fatal("snippet missed winning constituent")
			}
		}
	}
	if !found {
		t.Fatal("compact missed merged article")
	}
	var read retrieval.ReadResult
	call("read", map[string]any{"country": "vn", "release": full.Release, "items": []map[string]string{{"next": hit.Next}}}, &read)
	if len(read.Items) != 1 || read.Items[0].Error != "" {
		t.Fatal("article continuation failed")
	}
	var text string
	for _, u := range read.Items[0].Units {
		text += u.Text
	}
	if !strings.Contains(text, "Khoản ba chỉ có trong bài đọc đầy đủ.") {
		t.Fatal("continuation did not read the whole article")
	}
}
