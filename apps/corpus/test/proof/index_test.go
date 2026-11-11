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
	"os/exec"
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

func fakeEmbedder(t *testing.T, recipe embedding.Recipe) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/recipe" {
			_ = json.NewEncoder(w).Encode(embedding.Report{ProtocolVersion: 1, RecipeHash: recipe.Hash(), Recipe: recipe})
			return
		}
		var request struct {
			Texts []string `json:"texts"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid request", 400)
			return
		}
		if r.URL.Path == "/v1/tokenize" {
			counts := make([]int, len(request.Texts))
			for i := range counts {
				counts[i] = 1
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"recipe_hash": recipe.Hash(), "counts": counts})
			return
		}
		if r.URL.Path == "/v1/embed" {
			rows := make([]map[string]any, len(request.Texts))
			for i := range rows {
				vector := make([]float32, recipe.Dimensions)
				vector[0] = 1
				rows[i] = map[string]any{"index": i, "token_count": 1, "embedding": vector}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"recipe_hash": recipe.Hash(), "dimensions": recipe.Dimensions, "data": rows})
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(server.Close)
	return server
}

func (h *harness) indexSetup() {
	h.t.Helper()
	h.startWorker("")
	h.command("start", "vn", h.rel, "--until", "relate")
	h.waitState("paused")
	h.stopWorker()
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
	cases := []evaluation.Case{{ID: "positive", Question: "ngân hàng", ExpectedCitations: []evaluation.Expectation{{DocNumber: "1/2026"}}}, {ID: "reject1", Question: "abc", ExpectAbstain: true}, {ID: "reject2", Question: "def", ExpectAbstain: true}, {ID: "reject3", Question: "ghi", ExpectAbstain: true}}
	legacy, err := json.Marshal(cases)
	if err != nil {
		h.t.Fatal(err)
	}
	hash := fmt.Sprintf("%x", sha256.Sum256(legacy))
	invHash := inv.IdentityHash()
	var fetchHash string
	for _, item := range inv.Items {
		if item.Stage == "fetch" && item.ItemKey == "vn:vbpl:1" && item.ArtifactSHA256 != nil {
			fetchHash = *item.ArtifactSHA256
		}
	}
	if fetchHash == "" {
		h.t.Fatal("missing proof fetch artifact")
	}
	m := evaluation.IdentityMap{Version: 2, LegacySHA256: hash, Entries: []evaluation.MapEntry{{CaseID: "positive", CitationIndex: 0, State: "resolved", ExpectedNumber: "1/2026", Reason: "proof document", ReviewedAt: "2026-11-09", Evidence: []evaluation.Evidence{{DocumentKey: "vn:vbpl:1", DocumentNumber: "1/2026", FetchSHA256: fetchHash, QuotedSpan: "Document 1"}}, Alternatives: []evaluation.Alternative{{Country: "vn", DocumentKey: "vn:vbpl:1", DocumentNumber: "1/2026", Kind: "document"}}}}}
	dir := filepath.Join(h.root, "fixtures")
	if err = os.Mkdir(dir, 0700); err != nil {
		h.t.Fatal(err)
	}
	write := func(name string, value any) {
		h.t.Helper()
		data, e := json.Marshal(value)
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
	if err = os.WriteFile(filepath.Join(dir, "inventory.sha256"), []byte(invHash), 0600); err != nil {
		h.t.Fatal(err)
	}
	suite, err := evaluation.LoadSuiteFS(os.DirFS(dir), ".")
	if err != nil {
		h.t.Fatalf("load proof fixtures: %v", err)
	}
	identities, err := build.IdentityInventory(inv, artifact.Store{Root: h.artifacts})
	if err != nil {
		h.t.Fatal(err)
	}
	if issues, e := suite.Map.Validate(suite.Legacy, suite.Hashes.Legacy, identities); e != nil || len(issues) != 0 {
		h.t.Fatalf("validate proof inventory: %v %v", issues, e)
	}
	builder := testpg.NewBuilder(h.t)
	doc := fakeEmbedder(h.t, embedding.DocumentRecipe1())
	query := fakeEmbedder(h.t, embedding.QueryRecipe1())
	h.extra = append(h.extra, "CORPUS_BUILDER_DATABASE_URL="+builder.URL, "CORPUS_BUILDER_TEMPLATE="+builder.Template, "CORPUS_DATABASE_DISK_PATH="+h.root, "CORPUS_EMBEDDER_URL="+doc.URL, "CORPUS_QUERY_EMBEDDER_URL="+query.URL, "CORPUS_EVAL_FIXTURES="+dir)
	h.registerIndexCleanup(builder.URL)
	h.startWorker("")
	h.command("start", "vn", h.rel)
	h.waitIndexState()
}
func (h *harness) waitIndexState() {
	h.t.Helper()
	deadline := time.Now().Add(55 * time.Second)
	for time.Now().Before(deadline) {
		if err := h.ctx.Err(); err != nil {
			h.reportBuildState(err)
		}
		var state, message string
		if err := h.db.QueryRow(h.ctx, `SELECT state, COALESCE(message, '') FROM run WHERE country='vn' AND rel=$1`, h.rel).Scan(&state, &message); err != nil {
			h.t.Fatal(err)
		}
		if state == "evaluated" {
			return
		}
		if state == "failed" {
			var itemMessage *string
			if err := h.db.QueryRow(h.ctx, `SELECT message FROM run_item WHERE run_id=(SELECT id FROM run WHERE country='vn' AND rel=$1) AND state='failed' ORDER BY stage, item_key LIMIT 1`, h.rel).Scan(&itemMessage); err == nil && itemMessage != nil {
				h.t.Fatalf("index run failed: %s; item: %s", message, *itemMessage)
			}
			h.t.Fatalf("index run failed: %s", message)
		}
		time.Sleep(10 * time.Millisecond)
	}
	h.reportBuildState(fmt.Errorf("index run exceeded 55 seconds"))
}

func (h *harness) reportBuildState(cause error) {
	h.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var state, key, message string
	var generation int
	var artifact bool
	err := h.db.QueryRow(ctx, `SELECT i.state, i.item_key, i.generation, i.artifact_id IS NOT NULL, COALESCE(i.message,'') FROM run_item i JOIN run r ON r.id=i.run_id WHERE r.country='vn' AND r.rel=$1 AND i.stage='build'`, h.rel).Scan(&state, &key, &generation, &artifact, &message)
	if err != nil {
		h.t.Fatalf("%v; inspect build item: %v", cause, err)
	}
	h.t.Fatalf("%v; build item state=%s key=%s generation=%d artifact=%t message=%s", cause, state, key, generation, artifact, message)
}
func TestProof_IndexStagesAfterIngest(t *testing.T) {
	deadline := time.Now().Add(55 * time.Second)
	h := newHarness(t)
	var cancel context.CancelFunc
	h.ctx, cancel = context.WithDeadline(context.Background(), deadline)
	defer cancel()
	h.indexSetup()
	cmd := exec.CommandContext(h.ctx, os.Getenv("CORPUS_PROOF_BINARY"), "status", "vn", h.rel)
	cmd.Dir, cmd.Env = h.root, h.env("")
	statusJSON, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("status: %v: %s", err, statusJSON)
	}
	var status struct {
		QualityPass     *bool `json:"quality_pass"`
		ReleaseEligible *bool `json:"release_eligible"`
	}
	if err = json.Unmarshal(statusJSON, &status); err != nil || status.QualityPass == nil || status.ReleaseEligible == nil || *status.ReleaseEligible {
		t.Fatalf("status quality: %s: %v", statusJSON, err)
	}
	var n int
	if err = h.db.QueryRow(h.ctx, `SELECT count(*) FROM run_item WHERE stage='eval' AND state='done'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("eval item count %d: %v", n, err)
	}
	var path string
	if err = h.db.QueryRow(h.ctx, `SELECT a.path FROM artifact a JOIN run_item i ON i.artifact_id=a.id WHERE i.stage='eval' AND i.state='done'`).Scan(&path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(h.artifacts, path, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var report evaluation.Report
	if err = json.Unmarshal(data, &report); err != nil || report.Status != "complete" {
		t.Fatalf("report status %q: %v", report.Status, err)
	}
}
func TestProof_LostCorpusDatabaseRestored(t *testing.T) {
	deadline := time.Now().Add(55 * time.Second)
	h := newHarness(t)
	var cancel context.CancelFunc
	h.ctx, cancel = context.WithDeadline(context.Background(), deadline)
	defer cancel()
	h.indexSetup()
	builderURL := ""
	for _, v := range h.extra {
		const prefix = "CORPUS_BUILDER_DATABASE_URL="
		if len(v) > len(prefix) && v[:len(prefix)] == prefix {
			builderURL = v[len(prefix):]
		}
	}
	conn, err := pgx.Connect(h.ctx, builderURL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(context.Background()) }()
	count := func() int {
		t.Helper()
		cfg, e := pgxpool.ParseConfig(builderURL)
		if e != nil {
			t.Fatal(e)
		}
		cfg.ConnConfig.Database = "corpus_vn_" + h.rel
		p, e := pgxpool.NewWithConfig(h.ctx, cfg)
		if e != nil {
			t.Fatal(e)
		}
		defer p.Close()
		var n int
		if e = p.QueryRow(h.ctx, `SELECT count(*) FROM chunk`).Scan(&n); e != nil {
			t.Fatal(e)
		}
		return n
	}
	before := count()
	if before <= 0 {
		t.Fatalf("expected chunks before restore, got %d", before)
	}
	if !testpg.DropCorpusDatabase(t, conn, "corpus_vn_"+h.rel, "vn/"+h.rel) {
		t.Fatal("database was not dropped")
	}
	out := filepath.Join(h.root, "diagnostic")
	h.command("eval", "vn", h.rel, "--out", out)
	if after := count(); after != before {
		t.Fatalf("chunk count %d != %d", after, before)
	}
	data, err := os.ReadFile(filepath.Join(out, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var report evaluation.Report
	if err = json.Unmarshal(data, &report); err != nil || report.DatabaseInstanceKind != "restored" {
		t.Fatalf("restored report: %s %v", report.DatabaseInstanceKind, err)
	}
}

func (h *harness) registerIndexCleanup(builderURL string) {
	h.t.Helper()
	h.t.Cleanup(func() {
		if h.worker != nil {
			h.stopWorker()
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		conn, err := pgx.Connect(ctx, builderURL)
		if err != nil {
			h.t.Errorf("connect for index cleanup: %v", err)
			return
		}
		defer func() { _ = conn.Close(context.Background()) }()
		rows, err := conn.Query(ctx, `SELECT datname, shobj_description(oid, 'pg_database') FROM pg_database WHERE datdba=(SELECT oid FROM pg_roles WHERE rolname=current_user) AND (datname=$1 OR datname LIKE 'corpus_tmp_vn_%')`, "corpus_vn_"+h.rel)
		if err != nil {
			h.t.Errorf("list index databases: %v", err)
			return
		}
		var names []string
		for rows.Next() {
			var name string
			var raw *string
			if err = rows.Scan(&name, &raw); err != nil {
				rows.Close()
				h.t.Errorf("read index database marker: %v", err)
				return
			}
			if raw == nil {
				continue
			}
			var marker struct {
				RunKey     string `json:"run_key"`
				InputHash  string `json:"build_input_hash"`
				RecipeHash string `json:"build_recipe_hash"`
				Instance   string `json:"instance"`
			}
			if json.Unmarshal([]byte(*raw), &marker) != nil || marker.RunKey != "vn/"+h.rel || len(marker.InputHash) != 64 || len(marker.RecipeHash) != 64 || (marker.Instance != "built" && marker.Instance != "restored") {
				continue
			}
			if name != "corpus_vn_"+h.rel && name != "corpus_tmp_vn_"+marker.InputHash[:24] {
				continue
			}
			names = append(names, name)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			h.t.Errorf("list index database markers: %v", err)
			return
		}
		for _, name := range names {
			testpg.DropCorpusDatabase(h.t, conn, name, "vn/"+h.rel)
		}
	})
}
