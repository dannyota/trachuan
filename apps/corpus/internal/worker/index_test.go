package worker

import (
	"reflect"
	"testing"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/config"
	"danny.vn/trachuan/apps/corpus/internal/evaluation"
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
		q, ok := e.(embedding.Query)
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
