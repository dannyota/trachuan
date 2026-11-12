package chinhphu

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/filecache"
)

// FileInfo describes the bytes actually fetched from an attachment URL.
type FileInfo struct {
	Hashes            filecache.Hashes
	Name              string
	Size              int64
	SHA256, MediaType string
	FetchedAt         time.Time
}

// Download streams a validated PDF into dst within the given byte limit.
func (c *Client) Download(ctx context.Context, target string, dst io.Writer, max int64) (FileInfo, error) {
	if max <= 0 {
		return FileInfo{}, errors.New("invalid download limit")
	}
	if max > c.limits.FileBytes {
		max = c.limits.FileBytes
	}
	u, parseErr := url.Parse(target)
	if parseErr != nil {
		return FileInfo{}, errors.New("invalid attachment URL")
	}
	info := FileInfo{Name: path.Base(u.Path)}
	tmp, createErr := os.CreateTemp("", "chinhphu-download-*")
	if createErr != nil {
		return FileInfo{}, fmt.Errorf("create PDF: %w", createErr)
	}
	defer func() { _ = tmp.Close(); _ = os.Remove(tmp.Name()) }()
	err := c.request(ctx, http.MethodGet, target, nil, true, func(r io.Reader, h http.Header) error {
		if _, err := tmp.Seek(0, io.SeekStart); err != nil {
			return &localDownloadError{err}
		}
		if err := tmp.Truncate(0); err != nil {
			return &localDownloadError{err}
		}
		media, _, mediaErr := mime.ParseMediaType(h.Get("Content-Type"))
		if mediaErr != nil || media != "application/pdf" {
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
		hash := filecache.NewHasher()
		size, e := io.Copy(io.MultiWriter(tmp, hash), io.LimitReader(reader, max))
		if e != nil {
			return e
		}
		if size == max {
			_, err := reader.Peek(1)
			if err == nil {
				return ErrTooLarge
			}
			if !errors.Is(err, io.EOF) {
				return err
			}
		}
		info.Size = size
		info.Hashes = hash.Hashes()
		info.SHA256 = info.Hashes.SHA256
		info.MediaType = media
		info.FetchedAt = c.now()
		return nil
	})
	if err != nil {
		return FileInfo{}, fmt.Errorf("download: %w", err)
	}
	if _, rewindErr := tmp.Seek(0, io.SeekStart); rewindErr != nil {
		return FileInfo{}, fmt.Errorf("rewind PDF: %w", rewindErr)
	}
	if _, writeErr := io.Copy(dst, tmp); writeErr != nil {
		return FileInfo{}, fmt.Errorf("write PDF: %w", writeErr)
	}
	return info, nil
}

type localDownloadError struct{ err error }

func (e *localDownloadError) Error() string { return "local download storage" }
func (e *localDownloadError) Unwrap() error { return e.err }
