package vbpl

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// Download saves a bounded attachment and returns its content hash.
func (c *Client) Download(ctx context.Context, id string, f File, dir string, limit int64) (Downloaded, error) {
	if limit < 0 {
		return Downloaded{}, fmt.Errorf("negative download limit")
	}
	if limit > 50_000_000 {
		limit = 50_000_000
	}
	refreshed := false
	for attempt := 0; ; attempt++ {
		if attempt > 0 {
			if err := wait(ctx, c.backoffs[attempt-1]); err != nil {
				return Downloaded{}, fmt.Errorf("retry download: %w", err)
			}
		}
		tmp, err := os.CreateTemp(dir, ".vbpl-*")
		if err != nil {
			return Downloaded{}, fmt.Errorf("create download: %w", err)
		}
		hash := sha256.New()
		var size int64
		status, reqErr := c.request(ctx, http.MethodGet, f.URL, nil, func(r io.Reader) error {
			var e error
			size, e = io.Copy(io.MultiWriter(tmp, hash), io.LimitReader(r, limit+1))
			if e != nil {
				return e
			}
			if size > limit {
				return ErrTooLarge
			}
			return nil
		})
		closeErr := tmp.Close()
		if status == http.StatusOK && reqErr == nil && closeErr == nil {
			digest := hex.EncodeToString(hash.Sum(nil))
			path := filepath.Join(dir, digest)
			if err := os.Rename(tmp.Name(), path); err != nil {
				_ = os.Remove(tmp.Name())
				return Downloaded{}, fmt.Errorf("save download: %w", err)
			}
			return Downloaded{Path: path, SHA256: digest, Size: size}, nil
		}
		if err := os.Remove(tmp.Name()); err != nil && !errors.Is(err, os.ErrNotExist) {
			return Downloaded{}, fmt.Errorf("remove partial download: %w", err)
		}
		if closeErr != nil {
			return Downloaded{}, fmt.Errorf("close download: %w", closeErr)
		}
		if status == http.StatusForbidden && !refreshed {
			refreshed = true
			files, err := c.Files(ctx, id)
			if err != nil {
				return Downloaded{}, fmt.Errorf("refresh files: %w", err)
			}
			found := false
			for _, entry := range files {
				if entry.FullPath == f.FullPath {
					f = entry
					found = true
					break
				}
			}
			if !found {
				return Downloaded{}, ErrNotFound
			}
			attempt--
			continue
		}
		if retryable(status, reqErr) && attempt < len(c.backoffs) {
			continue
		}
		if reqErr != nil {
			return Downloaded{}, fmt.Errorf("download: %w", reqErr)
		}
		if status == http.StatusNotFound {
			return Downloaded{}, ErrNotFound
		}
		return Downloaded{}, fmt.Errorf("download status %d", status)
	}
}
