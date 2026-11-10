package chinhphu

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"time"
)

// FileInfo describes the bytes actually fetched from an attachment URL.
type FileInfo struct {
	Name              string
	Size              int64
	SHA256, MediaType string
	FetchedAt         time.Time
}

type partialDownload struct{ err error }

func (e *partialDownload) Error() string { return "partial download" }
func (e *partialDownload) Unwrap() error { return e.err }

// Download streams a validated PDF into dst within the given byte limit.
func (c *Client) Download(ctx context.Context, target string, dst io.Writer, max int64) (FileInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, c.limits.ItemTimeout)
	defer cancel()
	if max <= 0 {
		return FileInfo{}, errors.New("invalid download limit")
	}
	if max > c.limits.FileBytes {
		max = c.limits.FileBytes
	}
	u, e := url.Parse(target)
	if e != nil {
		return FileInfo{}, errors.New("invalid attachment URL")
	}
	info := FileInfo{Name: path.Base(u.Path)}
	err := c.request(ctx, http.MethodGet, target, nil, true, func(r io.Reader, h http.Header) error {
		media, _, e := mime.ParseMediaType(h.Get("Content-Type"))
		if e != nil || media != "application/pdf" {
			if media == "text/html" || media == "application/xhtml+xml" {
				b, err := c.readPage(r, h)
				if err != nil {
					return err
				}
				n, err := parseHTML(b)
				if err != nil {
					return err
				}
				if isChallenge(n, b) {
					return c.pause(ctx, "challenge")
				}

			} else {
				n, err := io.Copy(io.Discard, io.LimitReader(r, max+1))
				if err != nil {
					return err
				}
				if n > max {
					return ErrTooLarge
				}
			}
			return fmt.Errorf("invalid PDF media type: %w", errInvalidPayload)
		}
		reader := bufio.NewReader(io.LimitReader(r, max+1))
		head, e := reader.Peek(5)
		if e != nil || string(head) != "%PDF-" {
			if _, err := io.Copy(io.Discard, reader); err != nil {
				return err
			}
			return fmt.Errorf("invalid PDF signature: %w", errInvalidPayload)
		}
		hash := sha256.New()
		size, e := io.Copy(io.MultiWriter(dst, hash), io.LimitReader(reader, max))
		if e != nil {
			if size > 0 {
				return &partialDownload{err: e}
			}
			return e
		}
		if size == max {
			_, err := reader.Peek(1)
			if err == nil {
				return ErrTooLarge
			}
			if !errors.Is(err, io.EOF) {
				return &partialDownload{err: err}
			}
		}
		info.Size = size
		info.SHA256 = hex.EncodeToString(hash.Sum(nil))
		info.MediaType = media
		info.FetchedAt = c.now()
		return nil
	})
	if err != nil {
		return FileInfo{}, fmt.Errorf("download: %w", err)
	}
	return info, nil
}
