//go:build integration

package serve

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

func syntheticVector(text string) []float32 {
	values := make([]float32, 1024)
	switch {
	case strings.Contains(strings.ToLower(text), "nebula"):
		values[1] = 1
	case strings.Contains(strings.ToLower(text), "quartz"):
		values[0] = 1
	default:
		values[2] = 1
	}
	return values
}

type syntheticQuery struct{}

func (syntheticQuery) Recipe(context.Context) (embedding.Recipe, error) {
	return embedding.QueryRecipe1(), nil
}

func (syntheticQuery) Embed(_ context.Context, text string) (embedding.Vector, error) {
	return embedding.Vector{Values: syntheticVector(text), TokenCount: 4, RecipeHash: embedding.QueryRecipe1().Hash()}, nil
}

type syntheticEmbedder struct{}

func (syntheticEmbedder) Recipe() string              { return "invented-framework-embedder/1" }
func (syntheticEmbedder) Check(context.Context) error { return nil }

type syntheticIndex struct {
	Key, Hash string
	Rows      []build.Vector
}

func (syntheticEmbedder) Build(_ context.Context, key string, chunks []build.Chunk, hash string) (build.EmbedResult, error) {
	index := syntheticIndex{Key: key, Hash: hash}
	var vectors []byte
	for _, c := range chunks {
		text := c.Content + " " + c.ContextPrefix
		input := sha256.Sum256([]byte(text))
		values := syntheticVector(text)
		index.Rows = append(index.Rows, build.Vector{Ordinal: c.Ordinal, TokenCount: 4, InputSHA256: hex.EncodeToString(input[:])})
		for _, v := range values {
			vectors = binary.LittleEndian.AppendUint32(vectors, math.Float32bits(v))
		}
	}
	raw, err := json.Marshal(index)
	return build.EmbedResult{Index: raw, Vectors: vectors, Rows: len(chunks)}, err
}

func (s syntheticEmbedder) Validate(ctx context.Context, dir, key string, chunks []build.Chunk, hash string) error {
	_, err := s.Read(ctx, dir, key, chunks, hash)
	return err
}

func (syntheticEmbedder) Read(_ context.Context, dir, key string, chunks []build.Chunk, hash string) ([]build.Vector, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "index.json"))
	if err != nil {
		return nil, fmt.Errorf("read invented index: %w", err)
	}
	var index syntheticIndex
	if json.Unmarshal(raw, &index) != nil || index.Key != key || index.Hash != hash || len(index.Rows) != len(chunks) {
		return nil, fmt.Errorf("invalid invented index")
	}
	vectors, err := os.ReadFile(filepath.Join(dir, "vectors.f32"))
	if err != nil {
		return nil, fmt.Errorf("read invented vectors: %w", err)
	}
	if len(vectors) != len(chunks)*1024*4 {
		return nil, fmt.Errorf("invalid invented vectors")
	}
	for i := range index.Rows {
		values := make([]float32, 1024)
		for j := range values {
			offset := (i*1024 + j) * 4
			values[j] = math.Float32frombits(binary.LittleEndian.Uint32(vectors[offset:]))
		}
		index.Rows[i].Values = values
	}
	return index.Rows, nil
}
