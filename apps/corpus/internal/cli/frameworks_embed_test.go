package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"danny.vn/trachuan/internal/embedding"
	"danny.vn/trachuan/internal/frameworks/build"
	"github.com/stretchr/testify/require"
)

func TestFrameworkEmbedder_RecipeMismatchNamesFieldsOnly(t *testing.T) {
	recipe := embedding.DocumentRecipe1()
	recipe.Model = "invented-private-model"
	calls := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Path)
		_ = json.NewEncoder(w).Encode(embedding.Report{ProtocolVersion: 1, RecipeHash: recipe.Hash(), Recipe: recipe})
	}))
	defer server.Close()
	t.Setenv("CORPUS_EMBEDDER_URL", server.URL)
	service := newFrameworkEmbedder()
	err := service.Check(context.Background())
	require.ErrorContains(t, err, "model")
	require.NotContains(t, err.Error(), "invented-private-model")
	require.Equal(t, []string{"/v1/recipe"}, calls)
}

func TestFrameworkEmbedder_RejectsInputs(t *testing.T) {
	recipe := embedding.DocumentRecipe1()
	requests := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.Path)
		if r.URL.Path == "/v1/recipe" {
			_ = json.NewEncoder(w).Encode(embedding.Report{ProtocolVersion: 1, RecipeHash: recipe.Hash(), Recipe: recipe})
			return
		}
		var body struct {
			Texts []string `json:"texts"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		counts := make([]int, len(body.Texts))
		for i := range counts {
			counts[i] = recipe.TokenCap + 1
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"recipe_hash": recipe.Hash(), "counts": counts})
	}))
	defer server.Close()
	t.Setenv("CORPUS_EMBEDDER_URL", server.URL)
	service := newFrameworkEmbedder()
	require.NoError(t, service.Check(context.Background()))
	result, err := service.Build(context.Background(), "fw:invented:v1", []build.Chunk{{Ordinal: 0, Content: "long"}, {Ordinal: 1, Content: strings.Repeat("x", 1048576)}}, strings.Repeat("a", 64))
	require.NoError(t, err)
	require.Len(t, result.Rejected, 2)
	require.Equal(t, "embedding_input_too_long", result.Rejected[0].Kind)
	require.Equal(t, "embedding_input_bytes_exceeded", result.Rejected[1].Kind)
	require.Nil(t, result.Rejected[1].TokenCount)
	require.Equal(t, []string{"/v1/recipe", "/v1/tokenize"}, requests)
}

func TestFrameworkEmbedder_RejectsRemoteURL(t *testing.T) {
	for _, address := range []string{"http://example.invalid:8092", "http://localhost.example.invalid", "https://127.0.0.1:8092", "http://127.0.0.1:8092/path", "http://user:pass@127.0.0.1:8092"} {
		t.Setenv("CORPUS_EMBEDDER_URL", address)
		require.Error(t, newFrameworkEmbedder().Check(context.Background()))
	}
}
