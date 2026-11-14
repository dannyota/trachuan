package releasecheck

import (
	"context"
	"fmt"

	"danny.vn/trachuan/internal/corpusdb"
	"danny.vn/trachuan/internal/embedding"
	"danny.vn/trachuan/internal/retrieval"
	"danny.vn/trachuan/internal/retrieval/lexical"
)

type Report struct {
	SchemaVersions   []int              `json:"schema_versions"`
	DenseQueryHash   string             `json:"dense_query_hash"`
	RunningQuery     embedding.Report   `json:"running_query"`
	LexicalRecipe    lexical.Recipe     `json:"lexical_recipe"`
	RetrievalRecipes []retrieval.Recipe `json:"retrieval_recipes"`
}

func Describe(ctx context.Context, client *embedding.Client) (Report, error) {
	running, err := client.Recipe(ctx)
	if err != nil {
		return Report{}, fmt.Errorf("read running query recipe: %w", err)
	}
	return Report{corpusdb.SupportedSchemaVersions(), embedding.QueryRecipe1().Hash(), running,
		lexical.Recipe1(), []retrieval.Recipe{retrieval.Recipe2(), retrieval.Recipe3()}}, nil
}
