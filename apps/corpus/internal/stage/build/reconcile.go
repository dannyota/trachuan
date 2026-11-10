package build

import (
	"fmt"
	"path/filepath"

	"danny.vn/trachuan/apps/corpus/internal/stage/chunk"
	"danny.vn/trachuan/apps/corpus/internal/stage/embed"
)

func (l *loader) reconcileInventory() error {
	for key, item := range l.items["chunk"] {
		if item.State != "done" {
			continue
		}
		other, ok := l.items["embed"][key]
		if !ok || other.State != "done" {
			continue
		}
		dir, err := artifactDir(l.artifacts, item)
		if err != nil {
			return fmt.Errorf("chunk artifact: %w", err)
		}
		file, _, err := chunk.ReadArtifact(dir)
		if err != nil {
			return fmt.Errorf("read chunks: %w", err)
		}
		if file.DocumentKey != key {
			return fmt.Errorf("chunk identity mismatch")
		}
		edir, err := artifactDir(l.artifacts, other)
		if err != nil {
			return fmt.Errorf("embed artifact: %w", err)
		}
		idx, err := embed.ReadArtifact(edir)
		if err != nil {
			return fmt.Errorf("read embeddings: %w", err)
		}
		if idx.DocumentKey != key || item.ArtifactSHA256 == nil || idx.ChunkSHA256 != *item.ArtifactSHA256 {
			return fmt.Errorf("embedding identity mismatch")
		}
		if err = checkVectorSize(filepath.Join(edir, "vectors.f32"), len(idx.Rows), idx.Dimensions); err != nil {
			return err
		}
		if err = reconcile(file, idx); err != nil {
			return err
		}
	}
	return nil
}
