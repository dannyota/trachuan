//go:build integration

package app

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"danny.vn/trachuan/internal/embedding"
	"danny.vn/trachuan/internal/frameworks/build"
)

func frameworkFixtureVector(text string) []float32 {
	v := make([]float32, embedding.QueryRecipe1().Dimensions)
	if strings.Contains(strings.ToLower(text), "nebula") {
		v[1] = 1
	} else {
		v[0] = 1
	}
	return v
}

type frameworkFixtureQuery struct{}

func (frameworkFixtureQuery) Recipe(context.Context) (embedding.Recipe, error) {
	return embedding.QueryRecipe1(), nil
}

func (frameworkFixtureQuery) Embed(_ context.Context, text string) (embedding.Vector, error) {
	return embedding.Vector{Values: frameworkFixtureVector(text), TokenCount: 4, RecipeHash: embedding.QueryRecipe1().Hash()}, nil
}

type frameworkFixtureEmbedder struct{}

func (frameworkFixtureEmbedder) Recipe() string              { return "invented-frameworks/1" }
func (frameworkFixtureEmbedder) Check(context.Context) error { return nil }

func (frameworkFixtureEmbedder) Build(_ context.Context, _ string, chunks []build.Chunk, _ string) (build.EmbedResult, error) {
	var rows []build.Vector
	var vectors []byte
	for _, c := range chunks {
		text := c.Content + " " + c.ContextPrefix
		hash := sha256.Sum256([]byte(text))
		values := frameworkFixtureVector(text)
		rows = append(rows, build.Vector{Ordinal: c.Ordinal, TokenCount: 4, InputSHA256: hex.EncodeToString(hash[:]), Values: values})
		for _, value := range values {
			vectors = binary.LittleEndian.AppendUint32(vectors, math.Float32bits(value))
		}
	}
	raw, err := json.Marshal(rows)
	return build.EmbedResult{Index: raw, Vectors: vectors, Rows: len(rows)}, err
}

func (s frameworkFixtureEmbedder) Validate(ctx context.Context, dir, key string, chunks []build.Chunk, hash string) error {
	_, err := s.Read(ctx, dir, key, chunks, hash)
	return err
}

func (frameworkFixtureEmbedder) Read(_ context.Context, dir, _ string, chunks []build.Chunk, _ string) ([]build.Vector, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "index.json"))
	if err != nil {
		return nil, fmt.Errorf("read invented index: %w", err)
	}
	var rows []build.Vector
	if json.Unmarshal(raw, &rows) != nil || len(rows) != len(chunks) {
		return nil, fmt.Errorf("invalid invented index")
	}
	return rows, nil
}
