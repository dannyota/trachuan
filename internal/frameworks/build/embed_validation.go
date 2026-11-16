package build

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

func validateEmbeddings(ctx context.Context, base, dir string, service Embedder, hash string) error {
	reg, err := readRegistry(filepath.Join(base, "snapshot/sources/frameworks.yaml"))
	if err != nil {
		return err
	}
	chunks, err := loadRows[Chunk](filepath.Join(base, "chunk/chunks.jsonl"))
	if err != nil {
		return err
	}
	groups := map[string][]Chunk{}
	for _, version := range releaseVersions(reg) {
		for _, prefix := range []string{"fw:", "fwp:"} {
			groups[prefix+version.Code+":"+version.Version] = nil
		}
	}
	for _, row := range chunks {
		if _, ok := groups[row.DocumentKey]; !ok {
			return fmt.Errorf("unknown document key")
		}
		groups[row.DocumentKey] = append(groups[row.DocumentKey], row)
	}
	root := filepath.Join(dir, "artifacts")
	entries, err := os.ReadDir(root)
	if err != nil {
		if len(groups) == 0 && os.IsNotExist(err) {
			return nil
		}
		return hide("read embedding artifacts", err)
	}
	if len(entries) != len(groups) {
		return fmt.Errorf("embedding artifact count mismatch")
	}
	for key, rows := range groups {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("validate embeddings: %w", err)
		}
		path := filepath.Join(root, digest([]byte(key)))
		if err := service.Validate(ctx, path, key, rows, hash); err != nil {
			return fmt.Errorf("invalid embedding artifact %s: %w", key, err)
		}
	}
	return nil
}
