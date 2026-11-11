package ocrstore

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func ensureDirectory(path string) error {
	info, err := os.Stat(path)
	if err == nil {
		if !info.IsDir() {
			return fmt.Errorf("OCR path is not a directory")
		}
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat directory: %w", err)
	}
	parent := filepath.Dir(path)
	if parent == path {
		return fmt.Errorf("cannot create directory")
	}
	if err = ensureDirectory(parent); err != nil {
		return err
	}
	if err = os.Mkdir(path, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("create directory: %w", err)
	}
	if err = syncDirectory(parent); err != nil {
		return fmt.Errorf("sync parent: %w", err)
	}
	return nil
}

func syncDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open directory: %w", err)
	}
	syncErr := dir.Sync()
	closeErr := dir.Close()
	if syncErr != nil {
		return fmt.Errorf("sync directory: %w", syncErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close directory: %w", closeErr)
	}
	return nil
}
