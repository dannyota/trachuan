package registry

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
)

func validPath(p string) bool {
	if p == "" || strings.Contains(p, "\\") || strings.HasPrefix(p, "/") || path.Clean(p) != p || p == "." {
		return false
	}
	for _, part := range strings.Split(p, "/") {
		if part == ".." || part == "." || part == "" {
			return false
		}
	}
	return true
}

func (r *Registry) Read(name string) ([]byte, error) {
	if !validPath(name) {
		return nil, fmt.Errorf("invalid source path")
	}
	var registered *File
	for i := range r.Frameworks {
		for j := range r.Frameworks[i].Files {
			file := &r.Frameworks[i].Files[j]
			if file.Path == name {
				registered = file
			}
		}
	}
	if registered == nil {
		return nil, fmt.Errorf("unregistered source path")
	}
	dir := os.Getenv("FRAMEWORKS_SOURCE_DIR")
	if dir == "" {
		return nil, fmt.Errorf("FRAMEWORKS_SOURCE_DIR is empty")
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("open source directory: %w", err)
	}
	defer func() { _ = root.Close() }()
	f, err := root.Open(name)
	if err != nil {
		return nil, fmt.Errorf("open source file: %w", err)
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat source file: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() != registered.Size {
		return nil, fmt.Errorf("invalid source file size or type")
	}
	data, err := io.ReadAll(io.LimitReader(f, registered.Size+1))
	if err != nil {
		return nil, fmt.Errorf("read source file: %w", err)
	}
	sum := sha256.Sum256(data)
	if int64(len(data)) != registered.Size || fmt.Sprintf("%x", sum) != registered.SHA256 {
		return nil, fmt.Errorf("source file integrity mismatch")
	}
	return data, nil
}
