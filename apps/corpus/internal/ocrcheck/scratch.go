package ocrcheck

import (
	"io/fs"
	"path/filepath"
)

func checkScratch(dir string) error {
	var total int64
	return filepath.WalkDir(dir, func(_ string, entry fs.DirEntry, err error) error {
		if err != nil {
			return renderFailed
		}
		if entry.IsDir() {
			return nil
		}
		info, statErr := entry.Info()
		if statErr != nil || !info.Mode().IsRegular() {
			return renderFailed
		}
		if info.Size() >= maxScratchBytes-total {
			return renderLimit
		}
		total += info.Size()
		return nil
	})
}
