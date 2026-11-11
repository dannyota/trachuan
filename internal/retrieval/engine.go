package retrieval

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"slices"
	"time"

	"danny.vn/trachuan/internal/corpusdb"
	"danny.vn/trachuan/internal/retrieval/lexical"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Open reads and validates the release in a corpus database.
func Open(ctx context.Context, db *pgxpool.Pool, embedder Embedder, options Options) (*Engine, error) {
	r, err := ReadRelease(ctx, db)
	if err != nil {
		// Keep validation errors such as an unsupported schema version; only a
		// vanished database is reclassified.
		if gone := databaseFailure("read release failed", err); errors.Is(gone, ErrReleaseGone) {
			return nil, gone
		}
		return nil, err
	}
	return openEngine(ctx, db, embedder, r, options, false)
}

// OpenDraft validates explicit metadata for trusted calibration callers.
func OpenDraft(ctx context.Context, db *pgxpool.Pool, embedder Embedder, release Release, options Options) (*Engine, error) {
	return openEngine(ctx, db, embedder, release, options, true)
}

func openEngine(ctx context.Context, db *pgxpool.Pool, embedder Embedder, r Release, options Options, draft bool) (*Engine, error) {
	if db == nil || embedder == nil || options.Now == nil || (options.Profile != "serving" && options.Profile != "evaluation") {
		return nil, errors.New("invalid engine dependencies")
	}
	recipe := r.RetrievalRecipe
	base := Recipe1()
	if draft {
		if recipe.Abstention.Threshold != nil || recipe.Abstention.CalibrationSHA256 != "" {
			return nil, errors.New("invalid draft calibration")
		}
	} else {
		if recipe.Abstention.Threshold == nil || math.IsNaN(*recipe.Abstention.Threshold) || math.IsInf(*recipe.Abstention.Threshold, 0) || !validHash(recipe.Abstention.CalibrationSHA256) {
			return nil, errors.New("invalid release calibration")
		}
		recipe.Abstention.Threshold = nil
		recipe.Abstention.CalibrationSHA256 = ""
	}
	s := r.LexicalStats
	if r.Country != "vn" || r.Rel == "" || !slices.Contains(corpusdb.SupportedSchemaVersions(), r.SchemaVersion) || recipe != base || r.LexicalRecipe != lexical.Recipe1() || !validObject(r.ChunkRecipe) || !validObject(r.EvaluationSummary) || !validHash(r.SourceManifestHash) || !validHash(r.BuildInputHash) || !validHash(r.BuildRecipeHash) || r.BuiltAt.IsZero() || s.Version != 1 || s.N < 0 || s.TotalTokens < 0 || s.AvgDL <= 0 || math.IsNaN(s.AvgDL) || math.IsInf(s.AvgDL, 0) || !validHash(s.VocabularySHA256) || !validHash(s.DFSHA256) || s.DistinctTerms < 0 || s.OccupiedDimensions < 0 || s.CollisionTerms != s.DistinctTerms-s.OccupiedDimensions {
		return nil, errors.New("invalid release metadata")
	}
	running, err := embedder.Recipe(ctx)
	if err != nil {
		return nil, fmt.Errorf("read embedder recipe failed: %w", err)
	}
	typmod, err := readEmbeddingDimensions(ctx, db)
	if err != nil {
		return nil, err
	}
	if err = validateDenseRecipe(r.DenseRecipe, running, typmod); err != nil {
		return nil, err
	}
	engine := &Engine{db: db, embedder: embedder, queryRecipe: running, release: r, options: options}
	engine.numberSegments, err = engine.loadNumberSegments(ctx)
	if err != nil {
		return nil, err
	}
	return engine, nil
}

type dimensionReader interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func readEmbeddingDimensions(ctx context.Context, db dimensionReader) (int, error) {
	var typmod int
	err := db.QueryRow(ctx, `SELECT atttypmod FROM pg_attribute WHERE attrelid = 'public.chunk'::regclass AND attname = 'embedding' AND NOT attisdropped`).Scan(&typmod)
	if err != nil {
		return 0, databaseFailure("read embedding dimensions failed", err)
	}
	return typmod, nil
}

var hashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func validHash(s string) bool { return hashPattern.MatchString(s) }
func validObject(b []byte) bool {
	var v map[string]json.RawMessage
	return json.Unmarshal(b, &v) == nil && v != nil
}

// Release returns the metadata pinned to this engine.
func (e *Engine) Release() Release {
	r := e.release
	r.ChunkRecipe = append(json.RawMessage(nil), r.ChunkRecipe...)
	r.EvaluationSummary = append(json.RawMessage(nil), r.EvaluationSummary...)
	if r.RetrievalRecipe.Abstention.Threshold != nil {
		v := *r.RetrievalRecipe.Abstention.Threshold
		r.RetrievalRecipe.Abstention.Threshold = &v
	}
	return r
}

func (e *Engine) deadline() time.Duration {
	if e.options.Profile == "evaluation" {
		return 60 * time.Second
	}
	return 2 * time.Second
}
