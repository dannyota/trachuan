package ocrcheck

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/anthropics/anthropic-sdk-go/v2"
)

// JPEGRenderer returns validated page files in numeric page order.
type JPEGRenderer interface {
	Preflight(context.Context) error
	RenderJPEGDPI(context.Context, []byte, int, int) ([]string, func(), error)
}

func (r *Runner) prepareJPEG(ctx context.Context, p prepared, data []byte, dpi int) (prepared, string) {
	tail := finishContent(nil, p.request.Text)
	p.request.content = tail
	if size, err := jpegRequestSize(p.request); err != nil || size > maxRequestBytes {
		return p, "over_check_limit"
	}
	paths, cleanup, err := r.JPEGRenderer.RenderJPEGDPI(ctx, data, p.pages, dpi)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		if errors.Is(err, PageTooLarge) || errors.Is(err, RenderTooLarge) || errors.Is(err, renderLimit) {
			return p, "over_check_limit"
		}
		return p, "render_failed"
	}
	if len(paths) != p.pages {
		return p, "page_count_mismatch"
	}
	read := r.readJPEG
	if read == nil {
		read = readJPEGPage
	}
	encode := r.encodeJPEG
	if encode == nil {
		encode = base64.StdEncoding.EncodeToString
	}
	content := make([]anthropic.ContentBlockParamUnion, 0, 2*len(paths)+2)
	total := len(p.request.Text)
	for i, path := range paths {
		if ctx.Err() != nil {
			return p, "render_failed"
		}
		page, readErr := read(path)
		if readErr != nil {
			return p, "render_failed"
		}
		if len(page) > 7_000_000 {
			return p, "over_check_limit"
		}
		total += ((len(page) + 2) / 3) * 4
		if total > maxRequestBytes {
			return p, "over_check_limit"
		}
		content = append(content, anthropic.NewTextBlock(fmt.Sprintf("Page %d:", i+1)), anthropic.NewImageBlockBase64(p.request.MediaType, encode(page)))
	}
	p.request.content = append(content, tail...)
	size, err := jpegRequestSize(p.request)
	p.requestBytes = size
	if err != nil || size > maxRequestBytes {
		return p, "over_check_limit"
	}
	return p, ""
}

func readJPEGPage(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open JPEG page: %w", err)
	}
	page, readErr := io.ReadAll(io.LimitReader(f, 7_000_001))
	closeErr := f.Close()
	if readErr != nil {
		return nil, fmt.Errorf("read JPEG page: %w", readErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close JPEG page: %w", closeErr)
	}
	return page, nil
}
