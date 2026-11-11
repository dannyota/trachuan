package mcpapi

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"danny.vn/trachuan/internal/retrieval"
)

func TestMCP_SearchInputBoundaries(t *testing.T) {
	h := newHarness(t, nil)
	for _, extra := range []string{`"as_of":""`, `"as_of":null`, `"in_force_only":null`, `"query":"\u0000"`, `"query":"` + strings.Repeat("đ", 2001) + `"`, `"limit":50.0000000000000001`} {
		t.Run(extra[:min(len(extra), 35)], func(t *testing.T) {
			wantError(t, callTool(t, h, "search", json.RawMessage(`{"country":"vn","query":"ngân hàng",`+extra+`}`)), "invalid_request")
		})
	}
	if h.resolver.calls.Load() != 0 {
		t.Fatal("invalid input reached resolver")
	}
}
func TestMCP_ReadRejectsUnknownFieldsBeforeResolve(t *testing.T) {
	h := newHarness(t, nil)
	for _, items := range []string{`null`, `[{"handle":"vn:test","extra":true}]`, `[{"handle":3}]`} {
		wantError(t, callTool(t, h, "read", json.RawMessage(`{"country":"vn","items":`+items+`}`)), "invalid_request")
	}
	if h.resolver.calls.Load() != 0 {
		t.Fatal("invalid read input reached resolver")
	}
}
func TestMCP_DiacriticsKeepsEngineMessage(t *testing.T) {
	h := newHarness(t, nil)
	r := callTool(t, h, "search", map[string]any{"country": "vn", "query": "ngan hang"})
	out := resultJSON(t, r)
	if r.IsError || out["reason"] != "diacritics_required" || out["message"] != "Vui lòng nhập câu hỏi tiếng Việt có dấu hoặc số hiệu văn bản." || out["abstain"] != true || h.engine.calls.Load() != 0 || h.resolver.done.Load() != 1 {
		t.Fatal(out)
	}
}
func TestMCP_StatelessJSONAndProtocolErrors(t *testing.T) {
	h := newHarness(t, nil)
	for _, tt := range []struct {
		name, method, body string
		status             int
	}{
		{"get", "GET", "", 405}, {"delete", "DELETE", "", 405},
		{"subscription", "POST", `{"jsonrpc":"2.0","id":1,"method":"subscriptions/listen"}`, 400},
		{"initialize", "POST", `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`, 200},
		{"unknown", "POST", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"private-input","arguments":{}}}`, 200},
		{"malformed", "POST", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":123,"arguments":{"query":"private-input"}}}`, 200},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(tt.method, "/mcp", strings.NewReader(tt.body))
			r.Header.Set("Authorization", "Bearer trc_"+strings.Repeat("A", 43))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Accept", "application/json, text/event-stream")
			r.Header.Set("Mcp-Session-Id", "private-input")
			w := httptest.NewRecorder()
			h.server.Config.Handler.ServeHTTP(w, r)
			if w.Code != tt.status {
				t.Fatalf("status %d: %s", w.Code, w.Body.String())
			}
			if w.Header().Get("Mcp-Session-Id") != "" || strings.Contains(w.Header().Get("Content-Type"), "event-stream") {
				t.Fatal("stateful or streaming response")
			}
			if bytes.Contains(w.Body.Bytes(), []byte("private-input")) || strings.Contains(h.logs.String(), "private-input") {
				t.Fatal("protocol leaked input")
			}
			if tt.name == "unknown" {
				var out map[string]any
				if json.Unmarshal(w.Body.Bytes(), &out) != nil || out["error"] == nil || out["result"] != nil {
					t.Fatal("protocol error became tool result")
				}
			}
		})
	}
}
func TestEncodeResult_PreservesNumbersAndOmitsOnlyEmptyFields(t *testing.T) {
	r, _, err := encodeResult(map[string]any{"count": int64(9007199254740993), "flag": false, "empty": "", "nil": nil, "items": []any{map[string]any{"complete": false, "next": ""}}}, false)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(r.StructuredContent)
	if !bytes.Contains(b, []byte(`"count":9007199254740993`)) || !bytes.Contains(b, []byte(`"flag":false`)) || !bytes.Contains(b, []byte(`"complete":false`)) || bytes.Contains(b, []byte(`"empty"`)) || bytes.Contains(b, []byte(`"nil"`)) {
		t.Fatalf("changed output: %s", b)
	}
}
func TestMCP_SearchLimitMaximum(t *testing.T) {
	h := newHarness(t, nil)
	r := callTool(t, h, "search", map[string]any{"country": "vn", "query": "ngân hàng", "limit": 50})
	if r.IsError || h.engine.request.Limit != 50 {
		t.Fatal(resultJSON(t, r))
	}
}
func TestMCP_LeaseReleasedOnEveryEngineError(t *testing.T) {
	h := newHarness(t, nil)
	h.engine.err = retrieval.ErrUnavailable
	for _, name := range []string{"search", "read", "corpus_status", "quality_gaps"} {
		args := map[string]any{"country": "vn", "query": "ngân hàng"}
		if name != "search" {
			delete(args, "query")
		}
		if name == "read" {
			args["items"] = []any{map[string]any{"handle": "vn:test"}}
		}
		wantError(t, callTool(t, h, name, args), "unavailable")
	}
	if h.resolver.done.Load() != 4 {
		t.Fatal("lease leak")
	}
}

func TestMCP_ProtocolErrorPreservesRequestID(t *testing.T) {
	h := newHarness(t, nil)
	r := httptest.NewRequest("POST", "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":17,"method":"subscriptions/listen"}`))
	r.Header.Set("Authorization", "Bearer trc_"+strings.Repeat("A", 43))
	r.Header.Set("Accept", "application/json, text/event-stream")
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.server.Config.Handler.ServeHTTP(w, r)
	var out struct {
		ID    int `json:"id"`
		Error struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal(w.Body.Bytes(), &out) != nil || out.ID != 17 || out.Error.Code != -32601 {
		t.Fatalf("lost protocol identity: %s", w.Body.String())
	}
}

func TestMCP_SearchSchemaAcceptsRecipeValues(t *testing.T) {
	h := newHarness(t, nil)
	h.engine.presented = &retrieval.SearchResult{
		Release: "20991111",
		Documents: map[string]retrieval.DocumentEntry{"vn:test": {
			Number: "1/2099/QH1", Issued: &retrieval.Date{Year: 2099, Month: 11, Day: 11},
			Status: retrieval.Status{Current: false}, Related: []retrieval.Pointer{{Dir: "in", Kind: "repeals", InRelease: false}},
		}},
		Hits: []retrieval.PresentedHit{{Handle: "vn:test#a", Doc: "vn:test", Complete: false, Text: "verbatim", Next: "opaque", Relations: []retrieval.Relation{{SourceResolved: false, TargetResolved: false}}}},
	}
	r := callTool(t, h, "search", map[string]any{"country": "vn", "query": "ngân hàng", "detail": "full"})
	out := resultJSON(t, r)
	if r.IsError {
		t.Fatal(out)
	}
	doc := out["documents"].(map[string]any)["vn:test"].(map[string]any)
	if doc["issued"] != "2099-11-11" || doc["status"].(map[string]any)["current"] != false {
		t.Fatal(doc)
	}
	if doc["related"].([]any)[0].(map[string]any)["in_release"] != false {
		t.Fatal("lost false pointer flag")
	}
}

func TestMCP_QueryValidationFixedMessage(t *testing.T) {
	h := newHarness(t, nil)
	r := callTool(t, h, "search", map[string]any{"country": "vn", "query": ""})
	wantError(t, r, "invalid_request")
	out := resultJSON(t, r)
	if out["error"].(map[string]any)["message"] != "Invalid request." {
		t.Fatal("query validation returned the wrong fixed message")
	}
}
