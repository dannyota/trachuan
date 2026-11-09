package artifact

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// VerifyFiles checks selected regular files against the manifest and returns its hash.
func VerifyFiles(dir string, names ...string) (string, error) {
	info, err := os.Lstat(dir)
	if err != nil {
		return "", fmt.Errorf("inspect artifact: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("artifact is not a directory: %s", dir)
	}
	info, err = os.Lstat(filepath.Join(dir, manifestName))
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
	entries := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" {
			continue
		}
		if len(line) < 67 || line[64:66] != "  " {
			return "", fmt.Errorf("invalid manifest line")
		}
		digest, name := line[:64], line[66:]
		if _, err = hex.DecodeString(digest); err != nil {
			return "", fmt.Errorf("invalid manifest hash: %w", err)
		}
		if _, ok := entries[name]; ok {
			return "", fmt.Errorf("duplicate manifest entry %q", name)
		}
		entries[name] = digest
	}
	for _, name := range names {
		if !filepath.IsLocal(name) || name == "." || name == manifestName || strings.ContainsAny(name, "\n\r") {
			return "", fmt.Errorf("invalid artifact path %q", name)
		}
		expected, ok := entries[filepath.ToSlash(name)]
		if !ok {
			return "", fmt.Errorf("manifest missing %q", name)
		}
		path := dir
		parts := strings.Split(filepath.Clean(name), string(filepath.Separator))
		for i, part := range parts {
			path = filepath.Join(path, part)
			info, err = os.Lstat(path)
			if err != nil {
				return "", fmt.Errorf("inspect %s: %w", name, err)
			}
			if i < len(parts)-1 {
				if !info.IsDir() {
					return "", fmt.Errorf("artifact parent is not a directory: %s", name)
				}
			} else if !info.Mode().IsRegular() {
				return "", fmt.Errorf("artifact is not a regular file: %s", name)
			}
		}
		f, err := os.Open(path)
		if err != nil {
			return "", fmt.Errorf("open %s: %w", name, err)
		}
		h := sha256.New()
		_, copyErr := io.Copy(h, f)
		closeErr := f.Close()
		if copyErr != nil {
			return "", fmt.Errorf("hash %s: %w", name, copyErr)
		}
		if closeErr != nil {
			return "", fmt.Errorf("close %s: %w", name, closeErr)
		}
		if hex.EncodeToString(h.Sum(nil)) != expected {
			return "", fmt.Errorf("manifest mismatch for %s", name)
		}
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
