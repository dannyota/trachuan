package cli

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"

	lawembed "danny.vn/trachuan/apps/corpus/internal/stage/embed"
	"danny.vn/trachuan/internal/embedding"
	"danny.vn/trachuan/internal/frameworks/build"
)

func (*frameworkEmbedder) Validate(ctx context.Context, dir, key string, chunks []build.Chunk, hash string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("validate embedding: %w", err)
	}
	idx, err := lawembed.ReadArtifact(dir)
	if err != nil {
		return fmt.Errorf("read embedding: %w", &frameworkEmbedError{cause: err})
	}
	if idx.DocumentKey != key || idx.ChunkSHA256 != hash || idx.Recipe != embedding.DocumentRecipe1() {
		return fmt.Errorf("embedding identity mismatch")
	}
	reader, err := lawembed.OpenVectors(dir, idx)
	if err != nil {
		return fmt.Errorf("read vectors: %w", &frameworkEmbedError{cause: err})
	}
	if err := reader.Close(); err != nil {
		return fmt.Errorf("close vectors: %w", &frameworkEmbedError{cause: err})
	}
	expected := make(map[int]string, len(chunks))
	for _, c := range chunks {
		if _, ok := expected[c.Ordinal]; ok {
			return fmt.Errorf("duplicate chunk ordinal")
		}
		input := strings.TrimSpace(c.ContextPrefix) + "\n" + strings.TrimSpace(c.Content)
		expected[c.Ordinal] = fmt.Sprintf("%x", sha256.Sum256([]byte(input)))
	}
	for _, row := range idx.Rows {
		input, ok := expected[row.ChunkOrdinal]
		if !ok || input != row.InputSHA256 {
			return fmt.Errorf("embedding row mismatch")
		}
		delete(expected, row.ChunkOrdinal)
	}
	for _, row := range idx.Rejected {
		if _, ok := expected[row.ChunkOrdinal]; !ok {
			return fmt.Errorf("embedding rejection mismatch")
		}
		delete(expected, row.ChunkOrdinal)
	}
	if len(expected) != 0 {
		return fmt.Errorf("missing embedding rows")
	}
	return nil
}
