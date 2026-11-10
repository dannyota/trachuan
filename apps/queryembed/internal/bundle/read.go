package bundle

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ReadVerified reads one listed regular file, bounded by its manifest size,
// and returns only bytes whose size and SHA-256 match the manifest.
func (m Manifest) ReadVerified(dir, path string) ([]byte, error) {
	entries, err := m.fileEntries()
	if err != nil {
		return nil, err
	}
	file, entry, err := openFile(dir, path, entries)
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(file, entry.Size))
	closeErr := file.Close()
	if err != nil {
		return nil, fmt.Errorf("read bundle file %q: %w", path, err)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close bundle file %q: %w", path, closeErr)
	}
	if int64(len(data)) != entry.Size {
		return nil, fmt.Errorf("bundle file %q size mismatch", path)
	}
	hash := sha256.Sum256(data)
	if hex.EncodeToString(hash[:]) != entry.SHA256 {
		return nil, fmt.Errorf("bundle file %q hash mismatch", path)
	}
	return data, nil
}

func (m Manifest) fileEntries() (map[string]File, error) {
	entries := make(map[string]File, len(m.Files))
	for _, f := range m.Files {
		if !validPath(f.Path) {
			return nil, fmt.Errorf("invalid bundle path %q", f.Path)
		}
		if _, ok := entries[f.Path]; ok {
			return nil, fmt.Errorf("duplicate bundle path %q", f.Path)
		}
		entries[f.Path] = f
	}
	return entries, nil
}

func openFile(dir, path string, entries map[string]File) (*os.File, File, error) {
	f, ok := entries[path]
	if !ok {
		return nil, File{}, fmt.Errorf("requested file %q not listed", path)
	}
	current := dir
	parts := strings.Split(path, "/")
	for i, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return nil, File{}, fmt.Errorf("stat bundle file %q: %w", path, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, File{}, fmt.Errorf("symlink in bundle path %q", path)
		}
		if i == len(parts)-1 && !info.Mode().IsRegular() {
			return nil, File{}, fmt.Errorf("bundle file %q is not regular", path)
		}
	}
	file, err := os.Open(current)
	if err != nil {
		return nil, File{}, fmt.Errorf("open bundle file %q: %w", path, err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, File{}, fmt.Errorf("stat bundle file %q: %w", path, err)
	}
	if !info.Mode().IsRegular() || info.Size() != f.Size {
		_ = file.Close()
		return nil, File{}, fmt.Errorf("bundle file %q size or type mismatch", path)
	}
	return file, f, nil
}
