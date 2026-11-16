package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/classify"
	"danny.vn/trachuan/apps/corpus/internal/config"
	"danny.vn/trachuan/apps/corpus/internal/stage/build"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"danny.vn/trachuan/apps/corpus/internal/taxonomy"
	"github.com/jackc/pgx/v5/pgxpool"
)

type screenTarget struct {
	runner       *classify.Runner
	runID        int64
	taxonomyHash string
	documents    []classify.Document
	families     []classify.Family
	selection    *classify.DocumentSelection
	close        func()
}

func resolvedPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve classification path: %w", err)
	}
	var suffix []string
	for {
		resolved, resolveErr := filepath.EvalSymlinks(abs)
		if resolveErr == nil {
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			return resolved, nil
		}
		if !os.IsNotExist(resolveErr) {
			return "", fmt.Errorf("resolve classification path: %w", resolveErr)
		}
		suffix = append(suffix, filepath.Base(abs))
		parent := filepath.Dir(abs)
		if parent == abs {
			return "", fmt.Errorf("classification path has no existing parent")
		}
		abs = parent
	}
}

func submissionDirectory(cfg config.Config) (string, error) {
	path := os.Getenv("CORPUS_SUBMISSIONS_DIR")
	if path == "" || cfg.CheckpointDir == "" {
		return "", fmt.Errorf("CORPUS_SUBMISSIONS_DIR and CORPUS_CHECKPOINT_DIR are required")
	}
	root, err := resolvedPath(path)
	if err != nil {
		return "", err
	}
	for _, excluded := range []string{cfg.CheckpointDir, cfg.Artifacts} {
		parent, resolveErr := resolvedPath(excluded)
		if resolveErr != nil {
			return "", resolveErr
		}
		rel, relErr := filepath.Rel(parent, root)
		if relErr != nil {
			return "", fmt.Errorf("compare submission directory: %w", relErr)
		}
		if rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
			return "", fmt.Errorf("CORPUS_SUBMISSIONS_DIR must be outside checkpoints and checkpointed artifacts")
		}
	}
	return root, nil
}

func openSubmissionRunner(ctx context.Context, provider bool) (*classify.Runner, *pgxpool.Pool, config.Config, error) {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return nil, nil, cfg, err
	}
	dir, err := submissionDirectory(cfg)
	if err != nil {
		return nil, nil, cfg, err
	}
	db, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, nil, cfg, fmt.Errorf("invalid workshop database configuration")
	}
	runner := &classify.Runner{Repository: &classify.PGRepository{Pool: db}, IntentDir: dir}
	if provider {
		client, clientErr := classify.NewClient()
		if clientErr != nil {
			db.Close()
			return nil, nil, cfg, clientErr
		}
		runner.Client = client
	}
	return runner, db, cfg, nil
}

func prepareScreen(ctx context.Context, o classify.Options) (screenTarget, error) {
	var labels taxonomy.Labels
	var labelsHash string
	if o.LabelsFile != "" {
		var err error
		labels, labelsHash, err = classify.ReadLabels(o.LabelsFile)
		if err != nil {
			return screenTarget{}, err
		}
		if labels.Country != o.Country {
			return screenTarget{}, fmt.Errorf("labeled set country differs from target release")
		}
	}
	runner, workshop, cfg, err := openSubmissionRunner(ctx, true)
	if err != nil {
		return screenTarget{}, err
	}
	fail := func(e error) (screenTarget, error) { workshop.Close(); return screenTarget{}, e }
	row, err := store.New(workshop).GetRun(ctx, store.GetRunParams{Country: o.Country, Rel: o.Release})
	if err != nil {
		return fail(fmt.Errorf("read classification run: %w", err))
	}
	name, err := build.DatabaseName(o.Country, o.Release)
	if err != nil {
		return fail(err)
	}
	if cfg.BuilderDatabaseURL == "" {
		return fail(fmt.Errorf("CORPUS_BUILDER_DATABASE_URL is required"))
	}
	poolConfig, err := pgxpool.ParseConfig(cfg.BuilderDatabaseURL)
	if err != nil {
		return fail(fmt.Errorf("invalid classification release database configuration"))
	}
	poolConfig.ConnConfig.Database = name
	poolConfig.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	release, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return fail(fmt.Errorf("open classification release database: %w", err))
	}
	failRelease := func(e error) (screenTarget, error) { release.Close(); return fail(e) }
	var taxonomyHash string
	err = release.QueryRow(ctx, `SELECT taxonomy_version FROM release WHERE singleton AND country=$1 AND rel=$2`, o.Country, o.Release).Scan(&taxonomyHash)
	if err != nil {
		return failRelease(fmt.Errorf("read classification release taxonomy: %w", err))
	}
	if _, err = classify.NewRecipe(o.Recipe.Model, taxonomyHash); err != nil {
		return failRelease(err)
	}
	families, err := classify.LoadFamilies(ctx, release)
	if err != nil {
		return failRelease(err)
	}
	var documents []classify.Document
	var selection *classify.DocumentSelection
	if o.LabelsFile == "" {
		documents, err = classify.LoadDocuments(ctx, release, workshop, artifact.Store{Root: cfg.Artifacts}, o.Documents, o.Seed)
	} else {
		documents, err = classify.LoadSelectedDocuments(ctx, release, workshop, artifact.Store{Root: cfg.Artifacts}, labels)
		selection = &classify.DocumentSelection{LabelsHash: labelsHash}
		for _, d := range documents {
			selection.DocumentKeys = append(selection.DocumentKeys, d.Key)
		}
	}
	if err != nil {
		return failRelease(err)
	}
	release.Close()
	return screenTarget{runner: runner, runID: row.ID, taxonomyHash: taxonomyHash, documents: documents, families: families, selection: selection, close: workshop.Close}, nil
}
