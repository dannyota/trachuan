package cli

import (
	"context"
	"fmt"

	lawembed "danny.vn/trachuan/apps/corpus/internal/stage/embed"
	"danny.vn/trachuan/internal/frameworks/build"
)

func (s *frameworkEmbedder) Read(ctx context.Context, dir, key string, chunks []build.Chunk, hash string) ([]build.Vector, error) {
	if err := s.Validate(ctx, dir, key, chunks, hash); err != nil {
		return nil, err
	}
	idx, err := lawembed.ReadArtifact(dir)
	if err != nil {
		return nil, fmt.Errorf("read embedding index: %w", &frameworkEmbedError{cause: err})
	}
	reader, err := lawembed.OpenVectors(dir, idx)
	if err != nil {
		return nil, fmt.Errorf("open embedding vectors: %w", &frameworkEmbedError{cause: err})
	}
	defer func() { _ = reader.Close() }()
	rows := make([]build.Vector, 0, len(idx.Rows))
	for _, row := range idx.Rows {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		values, err := reader.Row(row.Row)
		if err != nil {
			return nil, fmt.Errorf("read embedding vector: %w", &frameworkEmbedError{cause: err})
		}
		rows = append(rows, build.Vector{Ordinal: row.ChunkOrdinal, TokenCount: row.TokenCount, InputSHA256: row.InputSHA256, Values: values})
	}
	if err := reader.Close(); err != nil {
		return nil, fmt.Errorf("close embedding vectors: %w", &frameworkEmbedError{cause: err})
	}
	return rows, nil
}
