package artifact

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

func syncPath(path string, syncFn func(string) error) error {
	if syncFn != nil {
		return syncFn(path)
	}
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	syncErr := f.Sync()
	closeErr := f.Close()
	if syncErr != nil {
		return fmt.Errorf("sync %s: %w", path, syncErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close %s: %w", path, closeErr)
	}
	return nil
}

func syncTree(root string, syncFn func(string) error) error {
	var dirs []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			dirs = append(dirs, path)
			return nil
		}
		return syncPath(path, syncFn)
	})
	if err != nil {
		return fmt.Errorf("sync files: %w", err)
	}
	for i := len(dirs) - 1; i >= 0; i-- {
		if syncErr := syncPath(dirs[i], syncFn); syncErr != nil {
			return fmt.Errorf("sync directory: %w", syncErr)
		}
	}
	return nil
}
