package build

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
)

func checkArtifactFiles(dir string) error {
	return filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("inspect artifact: %w", err)
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("artifact contains symlink")
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("artifact contains non-regular file")
		}
		if strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
			info, statErr := entry.Info()
			if statErr != nil {
				return fmt.Errorf("stat artifact: %w", statErr)
			}
			if info.Size() > 256<<20 {
				return fmt.Errorf("artifact JSON exceeds 256 MiB")
			}
		}
		return nil
	})
}
func artifactDir(s artifact.Store, item InventoryItem) (string, error) {
	if item.Path == nil || item.ArtifactSHA256 == nil {
		return "", fmt.Errorf("missing artifact for %s", item.ItemKey)
	}
	root, err := filepath.Abs(s.Root)
	if err != nil {
		return "", fmt.Errorf("artifact root: %w", err)
	}
	path, err := filepath.Abs(s.Abs(*item.Path))
	if err != nil {
		return "", fmt.Errorf("artifact path: %w", err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resolve artifact root: %w", err)
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("resolve artifact path: %w", err)
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("artifact path outside root")
	}
	if err = checkArtifactFiles(path); err != nil {
		return "", err
	}
	hash, err := artifact.Verify(path)
	if err != nil {
		return "", fmt.Errorf("verify artifact: %w", err)
	}
	if hash != *item.ArtifactSHA256 {
		return "", fmt.Errorf("artifact hash mismatch")
	}
	return path, nil
}

func checkInputHash(item InventoryItem, expected string) error {
	if item.InputHash == nil || *item.InputHash != expected {
		return fmt.Errorf("artifact input hash mismatch")
	}
	return nil
}
