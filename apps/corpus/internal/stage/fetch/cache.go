package fetch

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"danny.vn/traquydinh/apps/corpus/internal/vbpl"
)

func (s *Stage) cachedFile(id, dir string, f vbpl.File) (vbpl.Downloaded, bool) {
	if f.Size < 0 || f.Size > s.fileLimit || f.Size > 50_000_000 {
		return vbpl.Downloaded{}, false
	}
	path, digest, ok := s.cache.Lookup(id, f.Name, f.Size, f.UpdatedAt)
	if !ok {
		return vbpl.Downloaded{}, false
	}
	source, err := os.Open(path)
	if err != nil {
		return vbpl.Downloaded{}, false
	}
	defer func() { _ = source.Close() }()
	tmp, err := os.CreateTemp(dir, ".cache-*")
	if err != nil {
		return vbpl.Downloaded{}, false
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	hash := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(tmp, hash), io.LimitReader(source, f.Size+1))
	closeErr := tmp.Close()
	if copyErr != nil || closeErr != nil || size != f.Size || hex.EncodeToString(hash.Sum(nil)) != digest {
		return vbpl.Downloaded{}, false
	}
	dest := filepath.Join(dir, digest)
	if err := os.Rename(tmp.Name(), dest); err != nil {
		return vbpl.Downloaded{}, false
	}
	slog.Info("attachment cache hit", "document_key", "vn:vbpl:"+id, "file_name", f.Name, "sha256_prefix", digest[:12])
	return vbpl.Downloaded{Path: dest, SHA256: digest, Size: size}, true
}
