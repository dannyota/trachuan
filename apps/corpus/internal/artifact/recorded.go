package artifact

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// OpenRecorded verifies every bundle file and the previously committed manifest
// hash. Missing or invalid content is a miss; unreadable storage is an error.
func (s Store) OpenRecorded(stage, inputHash, recipeHash, manifestHash string) (Result, bool, error) {
	if err := validKey(stage, inputHash, recipeHash); err != nil {
		return Result{}, false, nil
	}
	root, err := os.Stat(s.Root)
	if errors.Is(err, os.ErrNotExist) {
		return Result{}, false, nil
	}
	if err != nil {
		return Result{}, false, fmt.Errorf("inspect artifact storage: %w", err)
	}
	if !root.IsDir() {
		return Result{}, false, fmt.Errorf("artifact storage is not a directory")
	}
	rel := filepath.Join(stage, inputHash+"-"+recipeHash)
	sum, err := Verify(s.Abs(rel))
	if err != nil {
		var pathErr *os.PathError
		if errors.As(err, &pathErr) && !errors.Is(err, os.ErrNotExist) && !errors.Is(err, syscall.ENOTDIR) {
			return Result{}, false, fmt.Errorf("read recorded artifact: %w", err)
		}
		return Result{}, false, nil
	}
	if sum != manifestHash {
		return Result{}, false, nil
	}
	return Result{Path: rel, InputHash: inputHash, SHA256: sum}, true, nil
}
