//go:build integration && faultinject

package proof

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/stage/membership"
	"danny.vn/trachuan/apps/corpus/internal/store"
)

func scopeFixture(t *testing.T, source, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "internal", source, "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func scopeEnvelope(t *testing.T, name string) map[string]any {
	t.Helper()
	var v map[string]any
	if err := json.Unmarshal(scopeFixture(t, "vbpl", name), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func scopeServers(t *testing.T) (*httptest.Server, *httptest.Server) {
	t.Helper()
	named := scopeEnvelope(t, "lookup-71-2025-QH15.json")
	central := scopeEnvelope(t, "lookup-71-2025-QH15.json")
	centralData := central["data"].(map[string]any)
	first := centralData["items"].([]any)[0].(map[string]any)
	first["id"], first["docNum"], first["title"], first["agencyIds"] = "67878", "22/2014/TT-NHNN", "central selected", []string{"62"}
	second := scopeEnvelope(t, "lookup-71-2025-QH15.json")["data"].(map[string]any)["items"].([]any)[0].(map[string]any)
	second["id"], second["docNum"], second["title"], second["agencyIds"] = "66518", "143/2026/NĐ-CP", "central selected", []string{"62"}
	second["agencyName"], second["issueDate"] = "Chính phủ", "2026-04-30"
	centralData["items"], centralData["total"] = []any{first, second}, 2
	details := map[string]map[string]any{}
	for _, id := range []string{"67878", "179989", "66518", "999"} {
		response := scopeEnvelope(t, "detail-67878.json")
		data := response["data"].(map[string]any)
		data["id"], data["references"] = id, []any{}
		switch id {
		case "67878":
			data["references"] = []any{map[string]any{"referenceType": "10", "targetDocument": map[string]any{"id": "999", "docNum": "context"}}}
		case "179989":
			data["docNum"], data["agencyName"], data["issueDate"] = "71/2025/QH15", "Quốc hội", "2025-06-14"
		case "66518":
			data["docNum"], data["agencyName"], data["issueDate"] = "143/2026/NĐ-CP", "Chính phủ", "2026-04-30"
		case "999":
			data["docNum"], data["title"] = "999/2026/QD", "Context"
		}
		details[id] = response
	}
	empty := scopeFixture(t, "vbpl", "lookup-142-2026-ND-CP.json")
	tree := scopeFixture(t, "vbpl", "tree-67878.json")
	home := scopeFixture(t, "chinhphu", "home.html")
	search := scopeFixture(t, "chinhphu", "search-number-142-2026-ND-CP.html")
	detail := scopeFixture(t, "chinhphu", "detail-218029.html")
	pdf := scopeFixture(t, "chinhphu", "pdf-142-head.bin")
	var cp *httptest.Server
	cp = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/cpp/files/") {
			_, _ = w.Write(pdf)
			return
		}
		if r.Method == http.MethodPost {
			if err := r.ParseForm(); err != nil {
				http.Error(w, "form", http.StatusBadRequest)
				return
			}
			body := search
			if strings.Contains(r.Form.Get("ctrl_191017_163$txtSearchKeyword"), "143/2026") {
				body = bytes.ReplaceAll(body, []byte("142/2026/NĐ-CP"), []byte("143/2026/NĐ-CP"))
				body = bytes.ReplaceAll(body, []byte("218029"), []byte("218030"))
			}
			_, _ = w.Write(body)
			return
		}
		if r.URL.Query().Get("docid") != "" {
			body := bytes.ReplaceAll(detail, []byte("https://datafiles.chinhphu.vn/cpp/files/"), []byte(cp.URL+"/cpp/files/"))
			if r.URL.Query().Get("docid") == "218030" {
				body = bytes.ReplaceAll(body, []byte("142/2026/NĐ-CP"), []byte("143/2026/NĐ-CP"))
			}
			_, _ = w.Write(body)
			return
		}
		_, _ = w.Write(home)
	}))
	t.Cleanup(cp.Close)

	vb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/doc/all" {
			var request struct {
				Option, Keyword string `json:"-"`
			}
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				http.Error(w, "json", http.StatusBadRequest)
				return
			}
			request.Option, _ = payload["optionDoc"].(string)
			request.Keyword, _ = payload["keyword"].(string)
			switch {
			case request.Option != "docNum":
				_ = json.NewEncoder(w).Encode(central)
			case strings.Contains(request.Keyword, "71/2025"):
				_ = json.NewEncoder(w).Encode(named)
			default:
				_, _ = w.Write(empty)
			}
			return
		}
		if strings.HasPrefix(r.URL.Path, "/doc/provision/tree/") {
			_, _ = w.Write(tree)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/doc/minio/") {
			_, _ = w.Write([]byte(`{"success":true,"data":[]}`))
			return
		}
		if strings.HasSuffix(r.URL.Path, "/diagram") {
			_, _ = w.Write([]byte(`{"success":true,"data":{"documentNamesByType":{}}}`))
			return
		}
		if strings.HasPrefix(r.URL.Path, "/doc/") {
			id := strings.TrimPrefix(r.URL.Path, "/doc/")
			response, ok := details[id]
			if !ok {
				http.NotFound(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode(response)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(vb.Close)
	return vb, cp
}

func TestProof_ScopeRunWithExpansionAndSecondary(t *testing.T) {
	deadline := time.Now().Add(55 * time.Second)
	h := newHarness(t)
	var cancel context.CancelFunc
	h.ctx, cancel = context.WithDeadline(context.Background(), deadline)
	defer cancel()
	vb, cp := scopeServers(t)
	h.gateway.server.Close()
	h.gateway.server = vb
	source := `country: vn
vbpl:
  selection_version: 1
  sweep_agencies: ["62"]
  named_numbers: ["71/2025/QH15"]
chinhphu:
  issuer_policy_version: 1
  central_issuer_forms: ["Chính phủ"]
  named_numbers: ["142/2026/NĐ-CP", "143/2026/NĐ-CP"]
relation_codes:
  "10": {kind: amends_supplements, label: amends, amending: true}
status_codes: {}
`
	if err := os.WriteFile(filepath.Join(h.root, "sources", "vn.yaml"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	h.extra = append(h.extra, "CORPUS_CHINHPHU_URL="+cp.URL, "CORPUS_CHINHPHU_FILE_URL="+cp.URL, "CORPUS_CHINHPHU_LOOPBACK_HTTP=true", "CORPUS_CHINHPHU_RPS=1000")
	h.startWorker("")
	h.command("start", "vn", h.rel, "--until", "fetch")
	h.waitState("paused")
	var id int64
	if err := h.db.QueryRow(h.ctx, `SELECT id FROM run WHERE country='vn' AND rel=$1`, h.rel).Scan(&id); err != nil {
		t.Fatal(err)
	}
	phase, err := membership.ReadPhase(h.ctx, store.New(h.db), id, membership.Final)
	if err != nil {
		t.Fatal(err)
	}
	active := map[string]string{}
	for _, entry := range phase.Active {
		active[entry.DocumentKey] = entry.Class
	}
	for key, class := range map[string]string{"vn:vbpl:67878": "primary", "vn:vbpl:179989": "primary", "vn:vbpl:66518": "primary", "vn:vbpl:999": "relation_context", "vn:chinhphu:218029": "secondary"} {
		if active[key] != class {
			t.Errorf("membership %s=%q, want %q", key, active[key], class)
		}
	}
	if _, ok := active["vn:chinhphu:218030"]; ok {
		t.Error("replaced secondary remains active")
	}
	var count int
	if err = h.db.QueryRow(h.ctx, `SELECT count(*) FROM gap WHERE run_id=$1 AND stage='fetch' AND item_key='vn:chinhphu:218030' AND kind='source_replaced'`, id).Scan(&count); err != nil || count != 1 {
		t.Fatalf("replacement gaps=%d: %v", count, err)
	}
	var path string
	if err = h.db.QueryRow(h.ctx, `SELECT a.path FROM artifact a JOIN run_item i ON i.artifact_id=a.id WHERE i.run_id=$1 AND i.stage='fetch' AND i.item_key='vn:chinhphu:218029' AND i.state='done'`, id).Scan(&path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(h.artifacts, path, "files.json"))
	if err != nil {
		t.Fatal(err)
	}
	var bundle struct {
		Version int               `json:"version"`
		Files   []json.RawMessage `json:"files"`
	}
	if err = json.Unmarshal(data, &bundle); err != nil || bundle.Version != 2 || len(bundle.Files) != 1 {
		t.Fatalf("secondary bundle version=%d files=%d: %v", bundle.Version, len(bundle.Files), err)
	}
}
