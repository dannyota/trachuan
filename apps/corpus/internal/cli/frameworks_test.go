package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"

	"os"
	"path/filepath"
	"testing"

	lawembed "danny.vn/trachuan/apps/corpus/internal/stage/embed"
	"danny.vn/trachuan/internal/embedding"

	"github.com/stretchr/testify/require"
)

func TestFrameworks_Usage(t *testing.T) {
	for _, args := range [][]string{{"frameworks"}, {"frameworks", "build"}, {"frameworks", "embed", "20990101"}, {"frameworks", "build", "20990101", "extra"}} {
		var out, errout bytes.Buffer
		require.Equal(t, 2, Run(context.Background(), args, &out, &errout))
		require.Contains(t, errout.String(), "corpus frameworks build <rel>")
	}
}

func startFrameworkEmbedder(t *testing.T) {
	t.Helper()
	recipe := embedding.DocumentRecipe1()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var output any
		if req.URL.Path == "/v1/recipe" {
			output = embedding.Report{ProtocolVersion: 1, RecipeHash: recipe.Hash(), Recipe: recipe}
		} else {
			var input struct {
				Texts []string `json:"texts"`
			}
			if err := json.NewDecoder(req.Body).Decode(&input); err != nil {
				http.Error(w, "invalid request", http.StatusBadRequest)
				return
			}
			counts := make([]int, len(input.Texts))
			data := make([]any, len(input.Texts))
			for i := range input.Texts {
				counts[i] = 2
				vector := make([]float32, recipe.Dimensions)
				vector[0] = 1
				data[i] = map[string]any{"index": i, "token_count": 2, "embedding": vector}
			}
			if req.URL.Path == "/v1/tokenize" {
				output = map[string]any{"recipe_hash": recipe.Hash(), "counts": counts}
			} else {
				output = map[string]any{"recipe_hash": recipe.Hash(), "dimensions": recipe.Dimensions, "data": data}
			}
		}
		_ = json.NewEncoder(w).Encode(output)
	}))
	t.Cleanup(server.Close)
	t.Setenv("CORPUS_EMBEDDER_URL", server.URL)
}

func TestFrameworks_BuildAndNoTextOutput(t *testing.T) {
	startFrameworkEmbedder(t)
	root := t.TempDir()
	t.Chdir(root)
	require.NoError(t, os.MkdirAll("sources", 0700))
	require.NoError(t, os.MkdirAll("source", 0700))
	require.NoError(t, os.MkdirAll("home", 0700))
	t.Setenv("HOME", filepath.Join(root, "home"))
	t.Setenv("TMPDIR", root)
	t.Setenv("FRAMEWORKS_SOURCE_DIR", filepath.Join(root, "source"))
	t.Setenv("FRAMEWORKS_WORK_DIR", filepath.Join(root, "work"))
	data := []byte(`{"catalog":{"groups":[{"id":"aa","title":"licensed title"}]}}`)
	require.NoError(t, os.WriteFile("source/catalog.json", data, 0600))
	registry := fmt.Sprintf("frameworks:\n  - code: fw-alpha\n    version: v1\n    name: Invented\n    publisher: Invented\n    language: en\n    access: licensed\n    ordinal: 1\n    citation_scheme: oscal-catalog\n    parser: nist-oscal-800-53\n    expected_controls: 1\n    files:\n      - path: catalog.json\n        sha256: %x\n        size: %d\n        role: catalog\n        format: json\n", sha256.Sum256(data), len(data))
	require.NoError(t, os.WriteFile("sources/frameworks.yaml", []byte(registry), 0600))
	var out, errout bytes.Buffer
	require.Equal(t, 0, Run(context.Background(), []string{"frameworks", "build", "20990101"}, &out, &errout), errout.String())
	require.Contains(t, out.String(), "embed complete")
	require.NotContains(t, out.String()+errout.String(), "licensed title")
	require.FileExists(t, "work/20990101/chunk/manifest.json")
	require.FileExists(t, "work/20990101/embed/manifest.json")
	artifacts, err := filepath.Glob("work/20990101/embed/artifacts/*")
	require.NoError(t, err)
	require.Len(t, artifacts, 2)
	keys := map[string]bool{}
	for _, dir := range artifacts {
		idx, readErr := lawembed.ReadArtifact(dir)
		require.NoError(t, readErr)
		keys[idx.DocumentKey] = true
		require.Equal(t, embedding.DocumentRecipe1().Hash(), idx.RecipeHash)
		chunkData, readErr := os.ReadFile("work/20990101/chunk/chunks.jsonl")
		require.NoError(t, readErr)
		require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(chunkData)), idx.ChunkSHA256)
		reader, openErr := lawembed.OpenVectors(dir, idx)
		require.NoError(t, openErr)
		require.NoError(t, reader.Close())
	}
	require.True(t, keys["fw:fw-alpha:v1"])
	require.True(t, keys["fwp:fw-alpha:v1"])
	out.Reset()
	errout.Reset()
	require.Equal(t, 0, Run(context.Background(), []string{"frameworks", "build", "20990101"}, &out, &errout), errout.String())
	require.Contains(t, out.String(), "embed skipped")
	require.NoError(t, os.WriteFile("work/20990101/parse/controls.jsonl", []byte("licensed text AA"), 0600))
	out.Reset()
	errout.Reset()
	require.Equal(t, 1, Run(context.Background(), []string{"frameworks", "build", "20990101"}, &out, &errout))
	require.Contains(t, errout.String(), "parse")
	require.NotContains(t, out.String()+errout.String(), "licensed text")
}
