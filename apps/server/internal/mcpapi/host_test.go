package mcpapi

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestMCP_LoopbackListenerAcceptsPublicHost(t *testing.T) {
	h := newHarness(t, nil)
	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"proxy","version":"1"}}}`
	r, err := http.NewRequest("POST", h.server.URL, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	r.Host = "trachuan.vn"
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept", "application/json, text/event-stream")
	resp, err := (&http.Client{Transport: authTransport{http.DefaultTransport}}).Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(b), `"serverInfo"`) {
		t.Fatalf("status %d body %s", resp.StatusCode, b)
	}
}
