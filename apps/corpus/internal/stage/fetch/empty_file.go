package fetch

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"danny.vn/trachuan/apps/corpus/internal/filecache"
	"danny.vn/trachuan/apps/corpus/internal/vbpl"
)

func writeEmptyFile(ctx context.Context, dir string, q hashStore) (vbpl.Downloaded, error) {
	if err := ctx.Err(); err != nil {
		return vbpl.Downloaded{}, err
	}
	hashes := filecache.NewHasher().Hashes()
	dest := filepath.Join(dir, hashes.SHA256)
	if err := os.WriteFile(dest, nil, 0600); err != nil {
		return vbpl.Downloaded{}, fmt.Errorf("write empty attachment: %w", err)
	}
	if err := writeHashes(ctx, q, hashes); err != nil {
		return vbpl.Downloaded{}, err
	}
	return vbpl.Downloaded{Path: dest, SHA256: hashes.SHA256, Size: 0, Hashes: hashes}, nil
}
