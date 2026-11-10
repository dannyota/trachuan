package build

import (
	"fmt"
	"os"

	"danny.vn/trachuan/apps/corpus/internal/stage/chunk"
	"danny.vn/trachuan/apps/corpus/internal/stage/embed"
)

func reconcile(file chunk.File, idx embed.Index) error {
	if len(file.Chunks) != len(idx.Rows)+len(idx.Rejected) {
		return fmt.Errorf("chunk and embedding counts differ")
	}
	ordinals := make(map[int]bool, len(file.Chunks))
	for _, c := range file.Chunks {
		if ordinals[c.Ordinal] {
			return fmt.Errorf("duplicate chunk ordinal")
		}
		ordinals[c.Ordinal] = true
	}
	for _, r := range idx.Rows {
		if !ordinals[r.ChunkOrdinal] {
			return fmt.Errorf("unknown embedding ordinal")
		}
		delete(ordinals, r.ChunkOrdinal)
	}
	for _, r := range idx.Rejected {
		if !ordinals[r.ChunkOrdinal] {
			return fmt.Errorf("unknown rejected ordinal")
		}
		delete(ordinals, r.ChunkOrdinal)
	}
	if len(ordinals) != 0 {
		return fmt.Errorf("unmatched chunk ordinal")
	}
	return nil
}

func checkVectorSize(path string, rows, dimensions int) error {
	if rows < 0 || rows > 500000 || dimensions != 1024 {
		return fmt.Errorf("invalid vector dimensions or row count")
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat vectors: %w", err)
	}
	if info.Size() != int64(rows)*int64(dimensions)*4 {
		return fmt.Errorf("vector file size mismatch")
	}
	return nil
}
