package retrieval

import (
	"encoding/json"
	"errors"
	"time"

	"danny.vn/trachuan/internal/embedding"
	"danny.vn/trachuan/internal/retrieval/lexical"
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

func validateDenseRecipe(d DenseRecipe, running embedding.Recipe, typmod int) error {
	if d.Version != 1 || d.Model == "" || d.Dimensions <= 0 || d.Model != d.Query.Model || d.Model != d.Document.Model || d.Dimensions != d.Query.Dimensions || d.Dimensions != d.Document.Dimensions || d.Dimensions != typmod || d.Query != running || d.QueryHash != d.Query.Hash() || d.DocumentHash != d.Document.Hash() {
		return errors.New("invalid dense recipe")
	}
	return nil
}
