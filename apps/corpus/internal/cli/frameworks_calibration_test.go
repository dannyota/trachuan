package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"danny.vn/trachuan/internal/embedding"
	"github.com/stretchr/testify/require"
)

func TestFrameworkQueryEmbedder_RejectsNonLoopback(t *testing.T) {
	for _, address := range []string{"http://example.invalid:8091", "http://localhost.example.invalid", "https://127.0.0.1:8091", "http://127.0.0.1:8091/path", "http://user:synthetic@127.0.0.1:8091", "http://127.0.0.1:8091?query=private"} {
		t.Setenv("CORPUS_QUERY_EMBEDDER_URL", address)
		_, err := newFrameworkQueryEmbedder()
		require.Error(t, err)
		require.NotContains(t, err.Error(), "private")
	}
}

func TestFrameworkQueryEmbedder_UsesQueryRecipe(t *testing.T) {
	r := embedding.QueryRecipe1()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/v1/recipe" {
			_ = json.NewEncoder(w).Encode(embedding.Report{ProtocolVersion: 1, RecipeHash: r.Hash(), Recipe: r})
			return
		}
		var input struct {
			Hash  string   `json:"recipe_hash"`
			Texts []string `json:"texts"`
		}
		if json.NewDecoder(request.Body).Decode(&input) != nil || input.Hash != r.Hash() || len(input.Texts) != 1 || input.Texts[0] != "invented query" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		values := make([]float32, r.Dimensions)
		values[0] = 1
		_ = json.NewEncoder(w).Encode(map[string]any{"recipe_hash": r.Hash(), "dimensions": r.Dimensions, "data": []any{map[string]any{"index": 0, "token_count": 3, "embedding": values}}})
	}))
	defer server.Close()
	t.Setenv("CORPUS_QUERY_EMBEDDER_URL", server.URL)
	query, err := newFrameworkQueryEmbedder()
	require.NoError(t, err)
	actual, err := query.Recipe(context.Background())
	require.NoError(t, err)
	require.Equal(t, r, actual)
	vector, err := query.Embed(context.Background(), "invented query")
	require.NoError(t, err)
	require.Equal(t, r.Hash(), vector.RecipeHash)
	require.Equal(t, float32(1), vector.Values[0])
}
