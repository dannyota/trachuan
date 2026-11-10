// Package bundle loads and verifies query model manifests.
package bundle

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

const model = "AITeamVN/Vietnamese_Embedding_v2"
const revision = "18b44161e041"

// File describes a file recorded in a bundle.
type File struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// Manifest describes a model bundle.
type Manifest struct {
	Model            string `json:"model"`
	Revision         string `json:"revision"`
	ResolvedRevision string `json:"resolved_revision"`
	Files            []File `json:"files"`
}

func (m Manifest) validate() error {
	if m.Model != model || m.Revision != revision || !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(m.ResolvedRevision) || !strings.HasPrefix(m.ResolvedRevision, m.Revision) {
		return fmt.Errorf("invalid model or revision")
	}
	return nil
}

// Load reads a strictly decoded manifest from dir.
func Load(dir string) (Manifest, error) {
	var m Manifest
	data, err := os.ReadFile(filepath.Join(dir, "bundle.json"))
	if err != nil {
		return m, fmt.Errorf("open manifest: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return Manifest{}, fmt.Errorf("decode manifest: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return Manifest{}, fmt.Errorf("manifest has trailing data")
	}
	if err := m.validate(); err != nil {
		return Manifest{}, fmt.Errorf("validate manifest: %w", err)
	}
	return m, nil
}

func validPath(p string) bool {
	return p != "" && p != "." && !strings.Contains(p, `\`) && !path.IsAbs(p) && path.Clean(p) == p && p != ".." && !strings.HasPrefix(p, "../")
}

// Verify checks the requested files against the manifest without loading others.
func (m Manifest) Verify(dir string, paths []string) error {
	entries := make(map[string]File, len(m.Files))
	for _, f := range m.Files {
		if !validPath(f.Path) {
			return fmt.Errorf("invalid bundle path %q", f.Path)
		}
		if _, ok := entries[f.Path]; ok {
			return fmt.Errorf("duplicate bundle path %q", f.Path)
		}
		entries[f.Path] = f
	}
	for _, p := range paths {
		f, ok := entries[p]
		if !ok {
			return fmt.Errorf("requested file %q not listed", p)
		}
		current := dir
		for _, part := range strings.Split(p, "/") {
			current = filepath.Join(current, part)
			info, err := os.Lstat(current)
			if err != nil {
				return fmt.Errorf("stat bundle file %q: %w", p, err)
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("symlink in bundle path %q", p)
			}
		}
		file, err := os.Open(current)
		if err != nil {
			return fmt.Errorf("open bundle file %q: %w", p, err)
		}
		info, err := file.Stat()
		if err != nil {
			_ = file.Close()
			return fmt.Errorf("stat bundle file %q: %w", p, err)
		}
		if !info.Mode().IsRegular() || info.Size() != f.Size {
			_ = file.Close()
			return fmt.Errorf("bundle file %q size or type mismatch", p)
		}
		h := sha256.New()
		_, err = io.Copy(h, file)
		closeErr := file.Close()
		if err != nil {
			return fmt.Errorf("hash bundle file %q: %w", p, err)
		}
		if closeErr != nil {
			return fmt.Errorf("close bundle file %q: %w", p, closeErr)
		}
		if hex.EncodeToString(h.Sum(nil)) != f.SHA256 {
			return fmt.Errorf("bundle file %q hash mismatch", p)
		}
	}
	return nil
}
