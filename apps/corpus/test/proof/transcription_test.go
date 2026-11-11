//go:build integration && faultinject

package proof

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/internal/embedding"
	"danny.vn/trachuan/internal/retrieval"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5/pgxpool"
)

type transcriptionEmbedder struct{}

func (transcriptionEmbedder) Recipe(context.Context) (embedding.Recipe, error) {
	return embedding.QueryRecipe1(), nil
}
func (transcriptionEmbedder) Embed(context.Context, string) (embedding.Vector, error) {
	v := make([]float32, 1024)
	v[0] = 1
	return embedding.Vector{Values: v, TokenCount: 1, RecipeHash: embedding.QueryRecipe1().Hash()}, nil
}

func transcriptionServer(t *testing.T, base *httptest.Server) *httptest.Server {
	t.Helper()
	body := "<html><body><p>Điều 1. Phạm vi áp dụng</p><p>Quy định này áp dụng cho hoạt động quản lý nguồn nước và bảo vệ tài nguyên thiên nhiên. " + strings.Repeat("Cơ quan thực hiện trách nhiệm theo quy định của pháp luật. ", 6) + "</p><p>Điều 2. Trách nhiệm thực hiện</p><p>Đơn vị thực hiện quy định về quản lý nguồn nước.</p></body></html>"
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/doc/provision/tree/67878":
			_, _ = w.Write([]byte(`{"success":true,"data":[]}`))
		case "/doc/minio/buckets/vbpl/folders/67878/files":
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": []any{map[string]any{"fileName": "67878_content.html", "fullPath": "67878/67878_content.html", "size": len([]byte(body)), "presignedUrl": server.URL + "/67878_content.html"}}})
		case "/67878_content.html":
			_, _ = w.Write([]byte(body))
		case "/doc/67878":
			rec := httptest.NewRecorder()
			base.Config.Handler.ServeHTTP(rec, r)
			var response map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
				http.Error(w, "fixture", 500)
				return
			}
			data := response["data"].(map[string]any)
			data["issueDate"], data["effFrom"], data["effTo"] = "2020-01-01", "2020-01-01", nil
			data["effStatus"] = map[string]any{"code": "CHL", "name": "Còn hiệu lực"}
			data["isConsolidatedDocument"] = false
			_ = json.NewEncoder(w).Encode(response)
		default:
			base.Config.Handler.ServeHTTP(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func TestProof_TranscriptionDocumentIsSearchable(t *testing.T) {
	h := newHarness(t)
	var cancel context.CancelFunc
	h.ctx, cancel = context.WithTimeout(context.Background(), 55*time.Second)
	defer cancel()
	vb, cp := scopeServers(t)
	h.gateway.server.Close()
	h.gateway.server = transcriptionServer(t, vb)
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
status_codes:
  "CHL": {class: in_force, current: true}
  "CCHL": {class: not_yet, current: false}
  "HHL1P": {class: partial, current: true}
`
	if err := os.WriteFile(filepath.Join(h.root, "sources", "vn.yaml"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	h.extra = append(h.extra, "CORPUS_CHINHPHU_URL="+cp.URL, "CORPUS_CHINHPHU_FILE_URL="+cp.URL, "CORPUS_CHINHPHU_LOOPBACK_HTTP=true", "CORPUS_CHINHPHU_RPS=1000", "CORPUS_CHINHPHU_IN_FLIGHT=4")
	h.startWorker("")
	h.command("start", "vn", h.rel, "--until", "relate")
	h.waitState("paused")
	h.stopWorker()
	dir := scopeEvalFixtures(h)
	builder := testpg.NewBuilder(t)
	doc, query := fakeEmbedder(t, embedding.DocumentRecipe1()), fakeEmbedder(t, embedding.QueryRecipe1())
	h.extra = append(h.extra, "CORPUS_BUILDER_DATABASE_URL="+builder.URL, "CORPUS_BUILDER_TEMPLATE="+builder.Template, "CORPUS_DATABASE_DISK_PATH="+h.root, "CORPUS_EMBEDDER_URL="+doc.URL, "CORPUS_QUERY_EMBEDDER_URL="+query.URL, "CORPUS_EVAL_FIXTURES="+dir)
	h.registerIndexCleanup(builder.URL)
	h.startWorker("")
	h.command("start", "vn", h.rel)
	h.waitIndexState()
	cfg, err := pgxpool.ParseConfig(builder.URL)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.Database = "corpus_vn_" + h.rel
	pool, err := pgxpool.NewWithConfig(h.ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var textSource string
	var complete bool
	if err = pool.QueryRow(h.ctx, `SELECT text_source,text_complete FROM document WHERE key='vn:vbpl:67878'`).Scan(&textSource, &complete); err != nil || textSource != "html_transcription" || !complete {
		t.Fatalf("document source=%q complete=%v: %v", textSource, complete, err)
	}
	engine, err := retrieval.Open(h.ctx, pool, transcriptionEmbedder{}, retrieval.Options{Now: time.Now, Profile: "evaluation"})
	if err != nil {
		t.Fatal(err)
	}
	req := retrieval.Request{Query: "quản lý nguồn nước", Mode: "exact", Arms: "lexical", Limit: 20}
	response, err := engine.Search(h.ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	result, err := engine.Present(h.ctx, response, retrieval.DetailFull)
	if err != nil {
		t.Fatal(err)
	}
	for _, hit := range result.Hits {
		if hit.Doc == "vn:vbpl:67878" && strings.Contains(hit.Citation, "Điều 1") && strings.Contains(hit.Text, "quản lý nguồn nước") && hit.Locator != nil && strings.HasPrefix(*hit.Locator, "block:") {
			entry := result.Documents[hit.Doc]
			if entry.TextSource != "html_transcription" || !entry.TextComplete {
				t.Fatalf("document entry: %+v", entry)
			}
			return
		}
	}
	t.Fatalf("transcription article not found: %+v", result.Hits)
}
