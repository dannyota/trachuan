package fetch

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/filecache"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/vbpl"
)

type candidate struct {
	path, hash, source string
	size               int64
	origin             *stage.FetchObservation
}

func (s *Stage) candidates(id string, f vbpl.File, b *baseline) []candidate {
	var out []candidate
	if b != nil {
		matches := b.index.names[f.Name]
		if len(matches) == 1 && matches[0].Size == f.Size {
			v := matches[0]
			c := candidate{path: filepath.Join(b.dir, "files", v.SHA256), hash: v.SHA256, size: v.Size, source: "baseline"}
			if origins := b.originsFor(v); len(origins) > 0 {
				o := adopted(origins[0])
				c.origin = &o
			}
			out = append(out, c)
		}
	}
	if path, hash, ok := s.cache.Lookup(id, f.Name, f.Size); ok {
		out = append(out, candidate{path: path, hash: hash, size: f.Size, source: "cache"})
	}
	return out
}

func etagForm(etag string) string {
	parts := strings.Split(etag, "-")
	if len(parts) > 2 || len(parts[0]) != 32 || strings.ToLower(parts[0]) != parts[0] {
		return ""
	}
	if _, err := hex.DecodeString(parts[0]); err != nil {
		return ""
	}
	if len(parts) == 1 {
		return "etag_md5"
	}
	if n, err := strconv.Atoi(parts[1]); err == nil && n > 0 {
		return "etag_multipart"
	}
	return ""
}

func copyCandidate(ctx context.Context, c candidate, dir, etag string, q hashStore) (vbpl.Downloaded, string, error) {
	stored, hasRow, err := storedHashes(ctx, q, c.hash)
	if err != nil {
		return vbpl.Downloaded{}, "", err
	}
	if hasRow && stored.Match(etag) == "" {
		return vbpl.Downloaded{}, "etag_differs", nil
	}
	in, err := os.Open(c.path)
	if errors.Is(err, os.ErrNotExist) {
		return vbpl.Downloaded{}, "candidate_unverified", nil
	}
	if err != nil {
		return vbpl.Downloaded{}, "", fmt.Errorf("open candidate: %w", err)
	}
	defer func() { _ = in.Close() }()
	tmp, err := os.CreateTemp(dir, ".candidate-*")
	if err != nil {
		return vbpl.Downloaded{}, "", fmt.Errorf("create candidate copy: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	h := filecache.NewHasher()
	n, copyErr := io.Copy(io.MultiWriter(tmp, h), io.LimitReader(in, c.size+1))
	closeErr := tmp.Close()
	if copyErr != nil {
		return vbpl.Downloaded{}, "", fmt.Errorf("copy candidate: %w", copyErr)
	}
	if closeErr != nil {
		return vbpl.Downloaded{}, "", fmt.Errorf("close candidate: %w", closeErr)
	}
	if ctx.Err() != nil {
		return vbpl.Downloaded{}, "", ctx.Err()
	}
	actual := h.Hashes()
	if n != c.size || actual.SHA256 != c.hash {
		return vbpl.Downloaded{}, "candidate_unverified", nil
	}
	if err = writeHashes(ctx, q, actual); err != nil {
		return vbpl.Downloaded{}, "", err
	}
	if hasRow && stored != actual {
		// Force a download after correcting a false row, even if another cache holds it.
		return vbpl.Downloaded{Hashes: actual}, "candidate_unverified", nil
	}
	form := actual.Match(etag)
	if form == "" {
		return vbpl.Downloaded{}, "etag_differs", nil
	}
	dest := filepath.Join(dir, c.hash)
	if err = os.Rename(tmp.Name(), dest); err != nil {
		return vbpl.Downloaded{}, "", fmt.Errorf("publish candidate: %w", err)
	}
	return vbpl.Downloaded{Path: dest, SHA256: c.hash, Size: n, Hashes: actual}, form + "_" + c.source, nil
}

func (s *Stage) attachment(ctx context.Context, id, dir string, f vbpl.File, b *baseline, unique bool, q hashStore) (vbpl.Downloaded, *stage.FetchObservation, string, error) {
	if f.URL == "" {
		d, e := s.client.Download(ctx, id, f, dir, s.fileLimit)
		return d, nil, "no_url", e
	}
	if f.Size > min(s.fileLimit, int64(50_000_000)) {
		d, e := s.client.Download(ctx, id, f, dir, s.fileLimit)
		return d, nil, "no_candidate", e
	}
	if f.Size == 0 {
		d, err := writeEmptyFile(ctx, dir, q)
		return d, nil, "empty_file", err
	}
	etag, err := s.client.ETag(ctx, f.URL)
	if err != nil {
		return vbpl.Downloaded{}, nil, "", err
	}
	reason := "no_candidate"
	candidates := s.candidates(id, f, b)
	if !unique {
		candidates = nil
	}
	switch {
	case etag == "":
		reason = "etag_unavailable"
	case etagForm(etag) == "":
		reason = "etag_other_form"
	case len(candidates) > 0:
		for _, c := range candidates {
			d, why, e := copyCandidate(ctx, c, dir, etag, q)
			if e != nil {
				return d, nil, "", e
			}
			reason = why
			if d.Path == "" && d.Hashes.SHA256 != "" {
				break
			}
			if d.Path != "" {
				origin := c.origin
				if origin == nil && c.source == "baseline" {
					origin = &stage.FetchObservation{Acquisition: "adopted"}
				}
				return d, origin, why, nil
			}
		}
	}
	d, err := s.client.Download(ctx, id, f, dir, s.fileLimit)
	if err == nil {
		err = writeHashes(ctx, q, d.Hashes)
	}
	return d, nil, reason, err
}
