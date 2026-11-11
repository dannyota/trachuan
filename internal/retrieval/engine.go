package retrieval

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"time"

	"danny.vn/trachuan/internal/corpusdb"
	"danny.vn/trachuan/internal/embedding"
	"danny.vn/trachuan/internal/retrieval/lexical"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Open reads and validates the release in a corpus database.
func Open(ctx context.Context, db *pgxpool.Pool, embedder Embedder, options Options) (*Engine, error) {
	if db == nil {
		return nil, errors.New("missing database")
	}
	var r Release
	var dense, lex, retrieval, stats []byte
	err := db.QueryRow(ctx, `SELECT country, rel, schema_version, taxonomy_version, dense_recipe, chunk_recipe, lexical_recipe, retrieval_recipe, lexical_stats, source_manifest_hash, build_input_hash, build_recipe_hash, evaluation_summary, built_at FROM release WHERE singleton`).Scan(&r.Country, &r.Rel, &r.SchemaVersion, &r.TaxonomyVersion, &dense, &r.ChunkRecipe, &lex, &retrieval, &stats, &r.SourceManifestHash, &r.BuildInputHash, &r.BuildRecipeHash, &r.EvaluationSummary, &r.BuiltAt)
	if err != nil {
		return nil, errors.New("read release failed")
	}
	if r.SchemaVersion != corpusdb.SchemaVersion {
		return nil, fmt.Errorf("corpus schema version %d, supported version %d", r.SchemaVersion, corpusdb.SchemaVersion)
	}
	for _, v := range []struct {
		raw []byte
		out any
	}{{dense, &r.DenseRecipe}, {lex, &r.LexicalRecipe}, {retrieval, &r.RetrievalRecipe}, {stats, &r.LexicalStats}} {
		if json.Unmarshal(v.raw, v.out) != nil {
			return nil, errors.New("invalid release metadata")
		}
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
	if r.Country != "vn" || r.Rel == "" || r.SchemaVersion != corpusdb.SchemaVersion || recipe != base || r.LexicalRecipe != lexical.Recipe1() || !validObject(r.ChunkRecipe) || !validObject(r.EvaluationSummary) || !validHash(r.SourceManifestHash) || !validHash(r.BuildInputHash) || !validHash(r.BuildRecipeHash) || r.BuiltAt.IsZero() || s.Version != 1 || s.N < 0 || s.TotalTokens < 0 || s.AvgDL <= 0 || math.IsNaN(s.AvgDL) || math.IsInf(s.AvgDL, 0) || !validHash(s.VocabularySHA256) || !validHash(s.DFSHA256) || s.DistinctTerms < 0 || s.OccupiedDimensions < 0 || s.CollisionTerms != s.DistinctTerms-s.OccupiedDimensions {
		return nil, errors.New("invalid release metadata")
	}
	running, err := embedder.Recipe(ctx)
	if err != nil {
		return nil, errors.New("read embedder recipe failed")
	}
	var typmod int
	err = db.QueryRow(ctx, `SELECT atttypmod FROM pg_attribute WHERE attrelid = 'public.chunk'::regclass AND attname = 'embedding' AND NOT attisdropped`).Scan(&typmod)
	if err != nil {
		return nil, errors.New("read embedding dimensions failed")
	}
	if err = validateDenseRecipe(r.DenseRecipe, running, typmod); err != nil {
		return nil, err
	}
	return &Engine{db: db, embedder: embedder, queryRecipe: running, release: r, options: options}, nil
}
func validHash(s string) bool { return regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(s) }
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

// ErrSearchUnavailable means an engine was opened without a query runtime.
var ErrSearchUnavailable = errors.New("search unavailable")

// OpenForRead validates a corpus without consulting a running embedder.
// Runtime recipe differences do not prevent reading retained source evidence.
func OpenForRead(ctx context.Context, db *pgxpool.Pool, options Options) (*Engine, error) {
	if db == nil || options.Now == nil || (options.Profile != "serving" && options.Profile != "evaluation") {
		return nil, errors.New("invalid read engine dependencies")
	}
	var r Release
	var dense, lex, retrieval, stats []byte
	err := db.QueryRow(ctx, `SELECT country, rel, schema_version, taxonomy_version, dense_recipe, chunk_recipe, lexical_recipe, retrieval_recipe, lexical_stats, source_manifest_hash, build_input_hash, build_recipe_hash, evaluation_summary, built_at FROM release WHERE singleton`).Scan(&r.Country, &r.Rel, &r.SchemaVersion, &r.TaxonomyVersion, &dense, &r.ChunkRecipe, &lex, &retrieval, &stats, &r.SourceManifestHash, &r.BuildInputHash, &r.BuildRecipeHash, &r.EvaluationSummary, &r.BuiltAt)
	if err != nil {
		return nil, fmt.Errorf("read release: %w", err)
	}
	if r.SchemaVersion != corpusdb.SchemaVersion {
		return nil, fmt.Errorf("corpus schema version %d, supported version %d", r.SchemaVersion, corpusdb.SchemaVersion)
	}
	for _, v := range []struct {
		raw []byte
		out any
	}{
		{dense, &r.DenseRecipe}, {lex, &r.LexicalRecipe}, {retrieval, &r.RetrievalRecipe}, {stats, &r.LexicalStats},
	} {
		if err := json.Unmarshal(v.raw, v.out); err != nil {
			return nil, fmt.Errorf("decode release metadata: %w", err)
		}
	}
	recipe := r.RetrievalRecipe
	if recipe.Abstention.Threshold == nil || math.IsNaN(*recipe.Abstention.Threshold) || math.IsInf(*recipe.Abstention.Threshold, 0) || !validHash(recipe.Abstention.CalibrationSHA256) {
		return nil, errors.New("invalid release calibration")
	}
	recipe.Abstention.Threshold = nil
	recipe.Abstention.CalibrationSHA256 = ""
	s := r.LexicalStats
	if r.Country != "vn" || r.Rel == "" || recipe != Recipe1() || !validObject(r.ChunkRecipe) || !validObject(r.EvaluationSummary) || !validHash(r.SourceManifestHash) || !validHash(r.BuildInputHash) || !validHash(r.BuildRecipeHash) || r.BuiltAt.IsZero() || s.Version != 1 || s.N < 0 || s.TotalTokens < 0 || s.AvgDL <= 0 || math.IsNaN(s.AvgDL) || math.IsInf(s.AvgDL, 0) || !validHash(s.VocabularySHA256) || !validHash(s.DFSHA256) || s.DistinctTerms < 0 || s.OccupiedDimensions < 0 || s.CollisionTerms != s.DistinctTerms-s.OccupiedDimensions {
		return nil, errors.New("invalid release metadata")
	}
	l := r.LexicalRecipe
	if l.Version < 1 || l.Algorithm == "" || l.Input == "" || l.Normalization == "" || l.Tokenizer == "" || l.HashAlgorithm == "" || l.Dimensions <= 0 || l.K1 <= 0 || math.IsNaN(l.K1) || math.IsInf(l.K1, 0) || l.B < 0 || l.B > 1 || math.IsNaN(l.B) || l.Query == "" || l.Collision == "" {
		return nil, errors.New("invalid lexical recipe")
	}
	var typmod int
	if err := db.QueryRow(ctx, `SELECT atttypmod FROM pg_attribute WHERE attrelid = 'public.chunk'::regclass AND attname = 'embedding' AND NOT attisdropped`).Scan(&typmod); err != nil {
		return nil, fmt.Errorf("read embedding dimensions: %w", err)
	}
	if err := validateDenseRecipe(r.DenseRecipe, r.DenseRecipe.Query, typmod); err != nil {
		return nil, err
	}
	return &Engine{db: db, release: r, options: options, embedder: readOnlyRuntime{}}, nil
}

// A marker keeps read-only engines distinct from partially constructed engines
// used by trusted evaluation and validation callers.
type readOnlyRuntime struct{}

func (readOnlyRuntime) Recipe(context.Context) (embedding.Recipe, error) {
	return embedding.Recipe{}, ErrSearchUnavailable
}
func (readOnlyRuntime) Embed(context.Context, string) (embedding.Vector, error) {
	return embedding.Vector{}, ErrSearchUnavailable
}
