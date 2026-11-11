package worker

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/config"
	"danny.vn/trachuan/apps/corpus/internal/evaluation"
	"danny.vn/trachuan/apps/corpus/internal/stage/build"
	"danny.vn/trachuan/apps/corpus/internal/stage/eval"
	"danny.vn/trachuan/apps/corpus/internal/workflow"
	"danny.vn/trachuan/internal/embedding"
)

// IndexDeps loads fixtures and pins the build tool versions for stage recipes.
func IndexDeps(ctx context.Context, cfg config.Config) (workflow.Deps, error) {
	if cfg.BuilderDatabaseURL == "" {
		return workflow.Deps{}, fmt.Errorf("CORPUS_BUILDER_DATABASE_URL is required to freeze build recipes")
	}
	if cfg.DatabaseDiskPath == "" {
		return workflow.Deps{}, fmt.Errorf("CORPUS_DATABASE_DISK_PATH is required to freeze build recipes")
	}
	suite, err := evaluation.VNSuite()
	if cfg.EvalFixtures != "" {
		suite, err = evaluation.LoadSuiteFS(os.DirFS(cfg.EvalFixtures), ".")
	}
	if err != nil {
		return workflow.Deps{}, fmt.Errorf("load evaluation fixtures: %w", err)
	}
	corrections, err := build.ReadCorrections("sources/vn-corrections.yaml")
	if err != nil {
		return workflow.Deps{}, fmt.Errorf("load corrections: %w", err)
	}
	taxonomy, err := build.ReadTaxonomy("taxonomy", cfg.PhapdienPackage)
	if err != nil {
		return workflow.Deps{}, fmt.Errorf("load taxonomy: %w", err)
	}
	deps := indexDeps(cfg, suite, time.Now)
	deps.Build.Taxonomy = taxonomy
	deps.Eval.Build.Taxonomy = taxonomy
	deps.Build.Corrections = corrections
	deps.Eval.Build.Corrections = corrections
	versions, err := build.InspectVersions(ctx, deps.Build)
	if err != nil {
		return workflow.Deps{}, fmt.Errorf("inspect build versions: %w", err)
	}
	deps.Build.Versions = versions
	deps.Eval.Build.Versions = versions
	return deps, nil
}

func indexDeps(cfg config.Config, suite evaluation.Suite, now func() time.Time) workflow.Deps {
	expected := embedding.QueryRecipe1()
	query := embedding.Query{Client: embedding.NewClient(cfg.QueryEmbedderURL, &http.Client{Timeout: 2 * time.Second}), RecipeHash: expected.Hash(), ExpectedRecipe: &expected}
	b := build.Config{BuilderURL: cfg.BuilderDatabaseURL, Template: cfg.BuilderTemplate, PgDump: cfg.PgDump, PgRestore: cfg.PgRestore, DatabaseDiskPath: cfg.DatabaseDiskPath, MinFreeBeforeBytes: uint64(cfg.DiskStartGiB) << 30, MinFreeDuringBytes: uint64(cfg.DiskMinGiB) << 30, Embedder: query, Suite: suite, Now: now}
	return workflow.Deps{Document: embedding.NewClient(cfg.EmbedderURL, nil), Build: b, Eval: eval.Config{Build: b, Suite: suite, Embedder: query}}
}
