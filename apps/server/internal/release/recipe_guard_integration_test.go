//go:build integration

package release

import (
	"context"
	"encoding/json"
	"testing"

	"danny.vn/trachuan/internal/embedding"
	"danny.vn/trachuan/internal/retrieval"
)

func TestResolver_TestRecipeRequiresLoopback(t *testing.T) {
	resolver, _, db, _ := corpusFixture(t)
	recipe := embedding.TestRecipe()
	dense := retrieval.DenseRecipe{Version: 1, Model: recipe.Model, Dimensions: recipe.Dimensions, Document: recipe, DocumentHash: recipe.Hash(), Query: recipe, QueryHash: embedding.TestRecipeHash}
	data, err := json.Marshal(dense)
	if err != nil {
		t.Fatal(err)
	}
	if _, execErr := db.Exec(context.Background(), `UPDATE release SET dense_recipe=$1`, data); execErr != nil {
		t.Fatal(execErr)
	}
	resolver.EmbedderFor = func(string, embedding.Recipe) retrieval.Embedder { return recipeEmbedder{recipe: recipe} }
	refused := 0
	resolver.OnTestRecipeRefused = func() { refused++ }
	if _, resolveErr := resolver.Resolve(context.Background(), "vn", ""); resolveErr == nil {
		t.Fatal("test recipe accepted on public deployment")
	}
	if refused != 1 {
		t.Fatalf("refusal count = %d", refused)
	}
	resolver.AllowTestRecipe = true
	lease, err := resolver.Resolve(context.Background(), "vn", "")
	if err != nil {
		t.Fatal(err)
	}
	lease.Done()
}
