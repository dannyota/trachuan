package vbpl

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// Download saves a bounded attachment and returns its content hash.
func (c *Client) Download(ctx context.Context, id string, f File, dir string, limit int64) (Downloaded, error) {
	ctx, cancel := context.WithTimeout(ctx, c.requestBudget)
	defer cancel()
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
		if f.URL == "" {
			return Downloaded{}, ErrNotFound
		}
		if err := c.checkDownloadURL(f.URL); err != nil {
			return Downloaded{}, err
		}
		tmp, err := os.CreateTemp(dir, ".vbpl-*")
		if err != nil {
			return Downloaded{}, fmt.Errorf("create download: %w", err)
		}
		hash := sha256.New()
		var size int64
		status, reqErr := c.request(ctx, http.MethodGet, f.URL, nil, true, func(r io.Reader) error {
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
		if status == http.StatusOK && reqErr == nil && closeErr == nil && f.Size > 0 && size != f.Size {
			reqErr = fmt.Errorf("download size %d does not match expected %d", size, f.Size)
		}
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
		if status == http.StatusForbidden && reqErr == nil && !refreshed {
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

func (c *Client) checkDownloadURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.Host == "" {
		return fmt.Errorf("invalid download URL")
	}
	loopbackHTTP := c.allowTestHTTP && u.Scheme == "http" && u.Hostname() == "127.0.0.1"
	if u.Scheme != "https" && !loopbackHTTP {
		return fmt.Errorf("download URL scheme refused")
	}
	base, err := url.Parse(c.base)
	if err != nil {
		return fmt.Errorf("invalid gateway URL: %w", err)
	}
	if strings.EqualFold(u.Host, base.Host) {
		return nil
	}
	for _, host := range c.fileHosts {
		if strings.EqualFold(u.Host, strings.TrimSpace(host)) {
			return nil
		}
	}
	return fmt.Errorf("download host refused")
}
