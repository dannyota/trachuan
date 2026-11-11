package retrieval

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"danny.vn/trachuan/internal/corpusdb"
	"danny.vn/trachuan/internal/embedding"
	"danny.vn/trachuan/internal/retrieval/lexical"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DenseRecipe describes the document and query embedding runtimes.
type DenseRecipe struct {
	Version      int              `json:"version"`
	Model        string           `json:"model"`
	Dimensions   int              `json:"dimensions"`
	Document     embedding.Recipe `json:"document"`
	DocumentHash string           `json:"document_hash"`
	Query        embedding.Recipe `json:"query"`
	QueryHash    string           `json:"query_hash"`
}

// Release holds the metadata pinned to a corpus database.
type Release struct {
	Country            string
	Rel                string
	SchemaVersion      int
	TaxonomyVersion    string
	DenseRecipe        DenseRecipe
	ChunkRecipe        json.RawMessage
	LexicalRecipe      lexical.Recipe
	RetrievalRecipe    Recipe
	LexicalStats       lexical.StatsSummary
	SourceManifestHash string
	BuildInputHash     string
	BuildRecipeHash    string
	EvaluationSummary  json.RawMessage
	BuiltAt            time.Time
}

// ReadRelease reads the singleton metadata and validates its schema and JSON.
// Open additionally checks recipe compatibility and the query embedder report.
func ReadRelease(ctx context.Context, db *pgxpool.Pool) (Release, error) {
	if db == nil {
		return Release{}, errors.New("missing database")
	}
	var r Release
	var dense, lex, retrieval, stats []byte
	err := db.QueryRow(ctx, `SELECT country, rel, schema_version, taxonomy_version, dense_recipe, chunk_recipe, lexical_recipe, retrieval_recipe, lexical_stats, source_manifest_hash, build_input_hash, build_recipe_hash, evaluation_summary, built_at FROM release WHERE singleton`).Scan(&r.Country, &r.Rel, &r.SchemaVersion, &r.TaxonomyVersion, &dense, &r.ChunkRecipe, &lex, &retrieval, &stats, &r.SourceManifestHash, &r.BuildInputHash, &r.BuildRecipeHash, &r.EvaluationSummary, &r.BuiltAt)
	if err != nil {
		return Release{}, fmt.Errorf("read release failed: %w", err)
	}
	if r.SchemaVersion != corpusdb.SchemaVersion {
		return Release{}, fmt.Errorf("corpus schema version %d, supported version %d", r.SchemaVersion, corpusdb.SchemaVersion)
	}
	for _, v := range []struct {
		raw []byte
		out any
	}{{dense, &r.DenseRecipe}, {lex, &r.LexicalRecipe}, {retrieval, &r.RetrievalRecipe}, {stats, &r.LexicalStats}} {
		if json.Unmarshal(v.raw, v.out) != nil {
			return Release{}, errors.New("invalid release metadata")
		}
	}
	return r, nil
}

func validateDenseRecipe(d DenseRecipe, running embedding.Recipe, typmod int) error {
	if d.Version != 1 || d.Model == "" || d.Dimensions <= 0 || d.Model != d.Query.Model || d.Model != d.Document.Model || d.Dimensions != d.Query.Dimensions || d.Dimensions != d.Document.Dimensions || d.Dimensions != typmod || d.Query != running || d.QueryHash != d.Query.Hash() || d.DocumentHash != d.Document.Hash() {
		return errors.New("invalid dense recipe")
	}
	return nil
}
