//go:build integration && faultinject

package proof

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/evaluation"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/build"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"danny.vn/trachuan/internal/embedding"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func scopeBuildServer(t *testing.T, base *httptest.Server) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/doc/provision/tree/67878" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true,"data":[{"key":"effect","id":"effect","title":"Điều 1. Hiệu lực thi hành","ptype":5,"level":"Article","orderIndex":1,"isLeaf":true,"content":{"title":"Điều 1","content":"Điều 1. Hiệu lực thi hành<br>Thông tư này có hiệu lực thi hành từ ngày 01 tháng 7 năm 2026. Thông tư này thay thế cho Nghị định số 143/2026/NĐ-CP ngày 30 tháng 4 năm 2026 của Chính phủ."}}],"total":1}`))
			return
		}
		if r.URL.Path == "/doc/67878" || r.URL.Path == "/doc/66518" {
			rec := httptest.NewRecorder()
			base.Config.Handler.ServeHTTP(rec, r)
			var response map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
				http.Error(w, "fixture", 500)
				return
			}
			data := response["data"].(map[string]any)
			data["effTo"] = nil
			if r.URL.Path == "/doc/67878" {
				data["issueDate"], data["effFrom"] = "2026-01-01", "2098-01-01"
				data["effStatus"] = map[string]any{"code": "CCHL", "name": "Chưa có hiệu lực"}
			} else {
				data["docType"] = map[string]any{"name": "Nghị định", "code": "ND"}
				data["issueDate"], data["effFrom"] = "2026-04-30", "2026-05-01"
				data["effStatus"] = map[string]any{"code": "CHL", "name": "Còn hiệu lực"}
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
			return
		}
		base.Config.Handler.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	return server
}

func scopeEvalFixtures(h *harness) string {
	h.t.Helper()
	row, err := store.New(h.db).GetRun(h.ctx, store.GetRunParams{Country: "vn", Rel: h.rel})
	if err != nil {
		h.t.Fatal(err)
	}
	var snapshot stage.Snapshot
	if err = json.Unmarshal(row.Snapshot, &snapshot); err != nil {
		h.t.Fatal(err)
	}
	inv, err := build.LoadInventory(h.ctx, store.New(h.db), stage.Run{ID: row.ID, Country: "vn", Rel: h.rel, Snapshot: snapshot}, "embed")
	if err != nil {
		h.t.Fatal(err)
	}
	cases := []evaluation.Case{{ID: "positive", Question: "hiệu lực", ExpectedCitations: []evaluation.Expectation{{DocNumber: "22/2014/TT-NHNN"}}}, {ID: "reject1", Question: "abc", ExpectAbstain: true}, {ID: "reject2", Question: "def", ExpectAbstain: true}, {ID: "reject3", Question: "ghi", ExpectAbstain: true}}
	legacy, err := json.Marshal(cases)
	if err != nil {
		h.t.Fatal(err)
	}
	hash := fmt.Sprintf("%x", sha256.Sum256(legacy))
	var fetchHash string
	for _, item := range inv.Items {
		if item.Stage == "fetch" && item.ItemKey == "vn:vbpl:67878" && item.ArtifactSHA256 != nil {
			fetchHash = *item.ArtifactSHA256
		}
	}
	if fetchHash == "" {
		h.t.Fatal("missing actor fetch artifact")
	}
	m := evaluation.IdentityMap{Version: 2, LegacySHA256: hash, Entries: []evaluation.MapEntry{{CaseID: "positive", CitationIndex: 0, State: "resolved", ExpectedNumber: "22/2014/TT-NHNN", Reason: "proof document", ReviewedAt: "2026-11-09", Evidence: []evaluation.Evidence{{DocumentKey: "vn:vbpl:67878", DocumentNumber: "22/2014/TT-NHNN", FetchSHA256: fetchHash, QuotedSpan: "central selected"}}, Alternatives: []evaluation.Alternative{{Country: "vn", DocumentKey: "vn:vbpl:67878", DocumentNumber: "22/2014/TT-NHNN", Kind: "document"}}}}}
	dir := filepath.Join(h.root, "fixtures")
	if err = os.Mkdir(dir, 0700); err != nil {
		h.t.Fatal(err)
	}
	write := func(name string, v any) {
		h.t.Helper()
		data, e := json.Marshal(v)
		if e != nil {
			h.t.Fatal(e)
		}
		if e = os.WriteFile(filepath.Join(dir, name), data, 0600); e != nil {
			h.t.Fatal(e)
		}
	}
	if err = os.WriteFile(filepath.Join(dir, "legacy.json"), legacy, 0600); err != nil {
		h.t.Fatal(err)
	}
	development := []byte("[]\n")
	if err = os.WriteFile(filepath.Join(dir, "development.json"), development, 0600); err != nil {
		h.t.Fatal(err)
	}
	write("fixtures.json", map[string]any{
		"sha256": hash, "case_count": len(cases), "source_path": "test", "source_repository": "test", "source_commit": "test", "origin": "test",
		"development": map[string]any{"sha256": fmt.Sprintf("%x", sha256.Sum256(development)), "case_count": 0, "origin": "test"},
	})
	write("old-product.json", map[string]any{"legacy_sha256": hash, "rejection_ids": []string{"reject1", "reject2", "reject3"}})
	write("identity-map.json", m)
	write("validity-cases.json", []map[string]any{{"number": "1/2099/TT-TEST", "issue_date": "2099-01-01", "current": true, "anchor": "Điều 1", "quote": "test"}})
	if err = os.WriteFile(filepath.Join(dir, "inventory.sha256"), []byte(inv.IdentityHash()), 0600); err != nil {
		h.t.Fatal(err)
	}
	suite, err := evaluation.LoadSuiteFS(os.DirFS(dir), ".")
	if err != nil {
		h.t.Fatal(err)
	}
	identities, err := build.IdentityInventory(inv, artifact.Store{Root: h.artifacts})
	if err != nil {
		h.t.Fatal(err)
	}
	if issues, e := suite.Map.Validate(suite.Legacy, suite.Hashes.Legacy, identities); e != nil || len(issues) != 0 {
		h.t.Fatalf("validate proof inventory: %v %v", issues, e)
	}
	return dir
}

func TestProof_ScopeRunBuildsAndEvaluates(t *testing.T) {
	deadline := time.Now().Add(55 * time.Second)
	h := newHarness(t)
	var cancel context.CancelFunc
	h.ctx, cancel = context.WithDeadline(context.Background(), deadline)
	defer cancel()
	vb, cp := scopeServers(t)
	h.gateway.server.Close()
	h.gateway.server = scopeBuildServer(t, vb)
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
  "CCHL": {class: not_yet, current: false}
  "CHL": {class: in_force, current: true}
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
	open := func() *pgxpool.Pool {
		t.Helper()
		p, e := pgxpool.NewWithConfig(h.ctx, cfg)
		if e != nil {
			t.Fatal(e)
		}
		return p
	}
	pool := open()
	count := func(query string, args ...any) int {
		t.Helper()
		var n int
		if e := pool.QueryRow(h.ctx, query, args...).Scan(&n); e != nil {
			t.Fatal(e)
		}
		return n
	}
	if n := count(`SELECT count(*) FROM document WHERE key=$1`, "vn:chinhphu:218030"); n != 0 {
		t.Errorf("replaced secondary documents=%d", n)
	}
	if n := count(`SELECT count(*) FROM coverage_gap WHERE document_key=$1 AND kind='source_replaced'`, "vn:chinhphu:218030"); n != 1 {
		t.Errorf("replacement gaps=%d", n)
	}
	if n := count(`SELECT count(*) FROM document d WHERE d.key=$1 AND d.class='secondary' AND d.source='chinhphu' AND NOT EXISTS (SELECT 1 FROM provision p WHERE p.document_id=d.id)`, "vn:chinhphu:218029"); n != 1 {
		t.Errorf("admitted secondary=%d", n)
	}
	if n := count(`SELECT count(*) FROM document WHERE key=$1 AND class='relation_context'`, "vn:vbpl:999"); n != 1 {
		t.Errorf("relation context=%d", n)
	}
	if n := count(`SELECT count(*) FROM document WHERE key=$1 AND status_class='in_force' AND status_basis='derived_from_text' AND eff_from='2026-07-01'`, "vn:vbpl:67878"); n != 1 {
		t.Errorf("rule A actor=%d", n)
	}
	if n := count(`SELECT count(*) FROM document WHERE key=$1 AND eff_to='2026-07-01' AND status_basis='derived_from_text'`, "vn:vbpl:66518"); n != 1 {
		t.Errorf("rule B target=%d", n)
	}
	for key, layer := range map[string]string{"vn:vbpl:67878": "A", "vn:vbpl:66518": "B"} {
		if n := count(`SELECT count(*) FROM validity_decision v JOIN document d ON d.id=v.document_id WHERE d.key=$1 AND v.layer=$2`, key, layer); n == 0 {
			t.Errorf("missing decision %s for %s", layer, key)
		}
	}
	decisions := func() string {
		t.Helper()
		var rows string
		if e := pool.QueryRow(h.ctx, `SELECT COALESCE(jsonb_agg(to_jsonb(v) ORDER BY v.document_id,v.ordinal), '[]'::jsonb)::text FROM validity_decision v`).Scan(&rows); e != nil {
			t.Fatal(e)
		}
		return rows
	}
	before := decisions()
	var buildPath, evalPath string
	for stageName, path := range map[string]*string{"build": &buildPath, "eval": &evalPath} {
		if err = h.db.QueryRow(h.ctx, `SELECT a.path FROM artifact a JOIN run_item i ON i.artifact_id=a.id JOIN run r ON r.id=i.run_id WHERE r.country='vn' AND r.rel=$1 AND i.stage=$2 AND i.state='done'`, h.rel, stageName).Scan(path); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = artifact.VerifyFiles(filepath.Join(h.artifacts, buildPath), "validity-review.json", "provenance.json"); err != nil {
		t.Fatalf("build manifest: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(h.artifacts, evalPath, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var report evaluation.Report
	if err = json.Unmarshal(data, &report); err != nil {
		t.Fatal(err)
	}
	for _, class := range []string{"primary", "secondary", "relation_context"} {
		if report.DocumentsByClass[class] < 1 {
			t.Errorf("report documents_by_class[%s]=%d", class, report.DocumentsByClass[class])
		}
	}
	pool.Close()
	conn, err := pgx.Connect(h.ctx, builder.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(context.Background()) }()
	if !testpg.DropCorpusDatabase(t, conn, "corpus_vn_"+h.rel, "vn/"+h.rel) {
		t.Fatal("database was not dropped")
	}
	out := filepath.Join(h.root, "diagnostic")
	h.command("eval", "vn", h.rel, "--out", out)
	pool = open()
	defer pool.Close()
	if after := decisions(); before == "[]" || after != before {
		t.Errorf("validity decisions changed after restore: before=%s after=%s", before, after)
	}
	restored, err := os.ReadFile(filepath.Join(out, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(restored, &report); err != nil || report.DatabaseInstanceKind != "restored" {
		t.Fatalf("restored report: %s: %v", report.DatabaseInstanceKind, err)
	}
}
