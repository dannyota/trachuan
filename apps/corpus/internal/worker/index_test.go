package worker

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/config"
	"danny.vn/trachuan/apps/corpus/internal/evaluation"
	"danny.vn/trachuan/apps/corpus/internal/workflow"
	"danny.vn/trachuan/internal/embedding"
)

func TestIndexDeps_ConfiguresBuildAndEval(t *testing.T) {
	cfg := config.Config{BuilderDatabaseURL: "postgres://builder", BuilderTemplate: "template", PgDump: "pg_dump_path", PgRestore: "pg_restore_path", DatabaseDiskPath: "disk", DiskStartGiB: 4, DiskMinGiB: 2, EmbedderURL: "http://document", QueryEmbedderURL: "http://query"}
	suite := evaluation.Suite{Hashes: evaluation.FixtureHashes{Legacy: "legacy"}}
	instant := time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC)
	deps := indexDeps(cfg, suite, func() time.Time { return instant })
	for _, b := range []struct {
		name                               string
		clock                              func() time.Time
		url, template, dump, restore, disk string
		before, during                     uint64
		suite                              evaluation.Suite
	}{
		{"build", deps.Build.Now, deps.Build.BuilderURL, deps.Build.Template, deps.Build.PgDump, deps.Build.PgRestore, deps.Build.DatabaseDiskPath, deps.Build.MinFreeBeforeBytes, deps.Build.MinFreeDuringBytes, deps.Build.Suite},
		{"eval build", deps.Eval.Build.Now, deps.Eval.Build.BuilderURL, deps.Eval.Build.Template, deps.Eval.Build.PgDump, deps.Eval.Build.PgRestore, deps.Eval.Build.DatabaseDiskPath, deps.Eval.Build.MinFreeBeforeBytes, deps.Eval.Build.MinFreeDuringBytes, deps.Eval.Build.Suite},
	} {
		if b.clock == nil || !b.clock().Equal(instant) || b.url != cfg.BuilderDatabaseURL || b.template != cfg.BuilderTemplate || b.dump != cfg.PgDump || b.restore != cfg.PgRestore || b.disk != cfg.DatabaseDiskPath || b.before != 4294967296 || b.during != 2147483648 || b.suite.Hashes != suite.Hashes {
			t.Errorf("%s config incomplete", b.name)
		}
	}
	if deps.Document == nil || deps.Build.Embedder == nil || deps.Eval.Build.Embedder == nil || deps.Eval.Embedder == nil || deps.Eval.Suite.Hashes != suite.Hashes {
		t.Fatal("missing embedder or suite")
	}
	for _, e := range []any{deps.Build.Embedder, deps.Eval.Build.Embedder, deps.Eval.Embedder} {
		v, ok := e.(retryEmbedder)
		if !ok {
			t.Fatalf("query embedder type = %T, want retryEmbedder", e)
		}
		q, ok := v.embedder.(embedding.Query)
		if !ok || q.Client == nil || q.RecipeHash != embedding.QueryRecipe1().Hash() {
			t.Error("invalid query embedder")
			continue
		}
		timeout := reflect.ValueOf(q.Client).Elem().FieldByName("http").Elem().FieldByName("Timeout").Int()
		if time.Duration(timeout) != 2*time.Second {
			t.Errorf("query timeout = %v, want 2s", time.Duration(timeout))
		}
	}
}

func TestIndexDeps_RequiresValidKnowledgeFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sources"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"vn-corrections.yaml", "vn-heading-exclusions.yaml"} {
		if err := os.WriteFile(filepath.Join(dir, "sources", name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(dir)
	cfg := config.Config{BuilderDatabaseURL: "postgres://builder", DatabaseDiskPath: "disk"}
	if _, err := IndexDeps(context.Background(), cfg); err == nil || !strings.Contains(err.Error(), "load knowledge") {
		t.Fatalf("missing file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sources", "vn-knowledge.yaml"), []byte("- { id: x }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := IndexDeps(context.Background(), cfg); err == nil || !strings.Contains(err.Error(), "load knowledge") {
		t.Fatalf("invalid file: %v", err)
	}
}

const oneKnowledgeEntry = `- id: "decision-mau"
  kind: "review_decision"
  target: { finding: "sha256:1111111111111111111111111111111111111111111111111111111111111111" }
  claim: { reason_code: "expected", reason: "Why it needs no change." }
  depends: { fingerprint: "sha256:2222222222222222222222222222222222222222222222222222222222222222", semantics: 1, first_rel: "20991118_1" }
  reviewed: { date: "2099-11-18", by: "primary", method: "human" }
`

func TestLoadSourceInputs_WiresKnowledgeWithoutDatabase(t *testing.T) {
	for name, tc := range map[string]struct {
		content string
		empty   bool
	}{"one entry": {oneKnowledgeEntry, false}, "no entries": {"[]\n", true}} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.Mkdir(filepath.Join(dir, "sources"), 0o700); err != nil {
				t.Fatal(err)
			}
			for _, file := range []string{"vn-corrections.yaml", "vn-heading-exclusions.yaml", "vn-heading-reviews.yaml", "vn-ocr-waivers.yaml"} {
				if err := os.WriteFile(filepath.Join(dir, "sources", file), nil, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(dir, "sources", "vn-knowledge.yaml"), []byte(tc.content), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "sources", "vn-reports.yaml"), []byte("[]\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			t.Chdir(dir)
			var deps workflow.Deps
			if err := loadSourceInputs(&deps, config.Config{}); err != nil {
				t.Fatal(err)
			}
			if deps.Build.Knowledge.Empty() != tc.empty || deps.Eval.Build.Knowledge.Empty() != tc.empty {
				t.Fatalf("build empty=%v eval empty=%v, want %v", deps.Build.Knowledge.Empty(), deps.Eval.Build.Knowledge.Empty(), tc.empty)
			}
		})
	}
}

const oneReportRecord = `- origin: "eval"
  origin_id: "case-2099-1"
  kind: "wrong_text"
  target: { key: "vn:vbpl:1", anchor: "" }
  claim: { code: "reported" }
`

func TestLoadSourceInputs_WiresReports(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sources"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{"vn-corrections.yaml", "vn-heading-exclusions.yaml", "vn-heading-reviews.yaml", "vn-ocr-waivers.yaml", "vn-knowledge.yaml"} {
		if err := os.WriteFile(filepath.Join(dir, "sources", file), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(dir)
	var deps workflow.Deps
	if err := loadSourceInputs(&deps, config.Config{}); err == nil || !strings.Contains(err.Error(), "load reports") {
		t.Fatalf("missing file: %v", err)
	}
	reports := filepath.Join(dir, "sources", "vn-reports.yaml")
	if err := os.WriteFile(reports, []byte("- { origin: x }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := loadSourceInputs(&deps, config.Config{}); err == nil || !strings.Contains(err.Error(), "load reports") {
		t.Fatalf("invalid file: %v", err)
	}
	if err := os.WriteFile(reports, []byte(oneReportRecord), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := loadSourceInputs(&deps, config.Config{}); err != nil {
		t.Fatal(err)
	}
	if deps.Build.Reports.Empty() || deps.Eval.Build.Reports.Empty() {
		t.Fatal("reports are not wired into build and eval")
	}
}
