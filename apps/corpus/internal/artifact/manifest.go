package artifact

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const manifestName = "MANIFEST.sha256"

func manifest(dir string) ([]byte, error) {
	info, err := os.Lstat(dir)
	if err != nil {
		return nil, fmt.Errorf("inspect artifact: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("artifact is not a directory: %s", dir)
	}
	var lines []string
	err = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == dir {
			return nil
		}
		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		if strings.ContainsAny(rel, "\n\r") {
			return fmt.Errorf("invalid artifact path %q", rel)
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink in artifact: %s", rel)
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("non-regular artifact file: %s", rel)
		}
		if rel == manifestName {
			return nil
		}
		f, openErr := os.Open(path)
		if openErr != nil {
			return openErr
		}
		h := sha256.New()
		_, copyErr := io.Copy(h, f)
		closeErr := f.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		lines = append(lines, fmt.Sprintf("%x  %s\n", h.Sum(nil), rel))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan artifact: %w", err)
	}
	sort.Strings(lines)
	return []byte(strings.Join(lines, "")), nil
}

// Verify checks the manifest and every file in dir and returns the manifest hash.
func Verify(dir string) (string, error) {
	info, err := os.Lstat(filepath.Join(dir, manifestName))
	if err != nil {
		return "", fmt.Errorf("inspect manifest: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("manifest is not a regular file")
	}
	data, err := os.ReadFile(filepath.Join(dir, manifestName))
	if err != nil {
		return "", fmt.Errorf("read manifest: %w", err)
	}
	actual, err := manifest(dir)
	if err != nil {
		return "", err
	}
	if !bytes.Equal(data, actual) {
		return "", fmt.Errorf("manifest does not match artifact files")
	}
	return fmt.Sprintf("%x", sha256.Sum256(data)), nil
}
