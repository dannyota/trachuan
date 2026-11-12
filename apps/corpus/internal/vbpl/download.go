package vbpl

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/filecache"
	"danny.vn/trachuan/apps/corpus/internal/sourcegate"
)

// Download saves a bounded attachment and returns its content hashes.
func (c *Client) Download(ctx context.Context, _ string, f File, dir string, limit int64) (Downloaded, error) {
	if limit < 0 {
		return Downloaded{}, fmt.Errorf("negative download limit")
	}
	limit = min(limit, 50_000_000)
	if f.URL == "" {
		return Downloaded{}, ErrNotFound
	}
	if f.Size > limit {
		return Downloaded{}, ErrTooLarge
	}
	if err := c.checkDownloadURL(f.URL); err != nil {
		return Downloaded{}, &Failure{Origin: "gateway", Cause: "unknown", Endpoint: "attachment", Err: err}
	}
	policy := sourcegate.Retry{Backoffs: c.backoffs, WaitLimit: c.requestBudget}
	for {
		tmp, err := os.CreateTemp(dir, ".vbpl-*")
		if err != nil {
			return Downloaded{}, fmt.Errorf("create download: %w", err)
		}
		h := filecache.NewHasher()
		var size int64
		var headers http.Header
		timeout := sourcegate.DownloadTimeout(f.Size)
		if c.fileTimeout != 30*time.Second {
			timeout = c.fileTimeout
		}
		status, reqErr := c.request(ctx, http.MethodGet, f.URL, nil, true, requestOptions{timeout: timeout, headers: &headers}, func(r io.Reader) error {
			var e error
			size, e = io.Copy(io.MultiWriter(tmp, h), io.LimitReader(r, limit+1))
			if e != nil {
				return e
			}
			if size > limit {
				return ErrTooLarge
			}
			return nil
		})
		closeErr := tmp.Close()
		contextErr := sourcegate.OutcomeContextError(ctx, status, reqErr)
		if contextErr != nil {
			reqErr = contextErr
		}
		if status == 200 && reqErr == nil && f.Size >= 0 && size != f.Size {
			reqErr = &Failure{Origin: "gateway", Cause: "size", Endpoint: "attachment", HTTPStatus: status}
		}
		if status == 200 && reqErr == nil && closeErr == nil {
			hashes := h.Hashes()
			dest := filepath.Join(dir, hashes.SHA256)
			if e := os.Rename(tmp.Name(), dest); e != nil {
				_ = os.Remove(tmp.Name())
				return Downloaded{}, fmt.Errorf("save download: %w", e)
			}
			return Downloaded{Path: dest, SHA256: hashes.SHA256, Size: size, Hashes: hashes}, nil
		}
		if e := os.Remove(tmp.Name()); e != nil && !errors.Is(e, os.ErrNotExist) {
			return Downloaded{}, fmt.Errorf("remove partial download: %w", e)
		}
		if closeErr != nil {
			return Downloaded{}, fmt.Errorf("close download: %w", closeErr)
		}
		if contextErr != nil {
			return Downloaded{}, contextErr
		}
		if errors.Is(reqErr, sourcegate.ErrSourcePaused) {
			return Downloaded{}, reqErr
		}
		delay, retry, deferred := policy.Next(status, retryable(status, reqErr), sourcegate.RetryAfter(headers.Get("Retry-After"), c.now()))
		if deferred {
			return Downloaded{}, &Failure{Origin: "gateway", Cause: "unknown", Err: ErrDeferred, Endpoint: "attachment", HTTPStatus: status}
		}
		if retry && ctx.Err() == nil {
			if e := c.sleep(ctx, delay); e != nil {
				if contextErr := sourcegate.OutcomeContextError(ctx, status, e); contextErr != nil {
					return Downloaded{}, contextErr
				}
				if ctx.Err() == nil {
					return Downloaded{}, e
				}
			} else {
				continue
			}
		}
		if reqErr != nil {
			return Downloaded{}, fmt.Errorf("download: %w", classify(reqErr, "attachment", status))
		}
		if status == 404 {
			return Downloaded{}, ErrNotFound
		}
		return Downloaded{}, statusFailure(status, "attachment")
	}
}

// ETag asks for one byte through the same admission and file pool as downloads.
// A source failure means no comparable ETag; pauses and our own errors propagate.
func (c *Client) ETag(ctx context.Context, target string) (string, error) {
	if err := c.checkDownloadURL(target); err != nil {
		return "", nil
	}
	var headers http.Header
	status, err := c.request(ctx, http.MethodGet, target, nil, true, requestOptions{ranged: true, headers: &headers}, func(r io.Reader) error { _, e := io.Copy(io.Discard, io.LimitReader(r, 1)); return e })
	if contextErr := sourcegate.OutcomeContextError(ctx, status, err); contextErr != nil {
		return "", contextErr
	}
	if errors.Is(err, sourcegate.ErrSourcePaused) {
		return "", err
	}
	if err != nil {
		var failure *Failure
		if errors.As(classify(err, "attachment", status), &failure) {
			return "", nil
		}
		return "", err
	}
	if status != 206 {
		return "", nil
	}
	return strings.Trim(headers.Get("ETag"), "\""), nil
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
