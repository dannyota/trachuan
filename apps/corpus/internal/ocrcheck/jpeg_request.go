package ocrcheck

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
)

// JPEGRenderer returns validated page files in numeric page order.
type JPEGRenderer interface {
	Preflight(context.Context) error
	RenderJPEGDPI(context.Context, []byte, int, int) ([]string, func(), error)
}

func (r *Runner) prepareJPEG(ctx context.Context, p prepared, data []byte, dpi int) (prepared, string) {
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
	total := len(p.request.Text)
	for _, path := range paths {
		if ctx.Err() != nil {
			return p, "render_failed"
		}
		f, openErr := os.Open(path)
		if openErr != nil {
			return p, "render_failed"
		}
		page, readErr := io.ReadAll(io.LimitReader(f, 7_000_001))
		closeErr := f.Close()
		if readErr != nil || closeErr != nil {
			return p, "render_failed"
		}
		if len(page) > 7_000_000 {
			return p, "over_check_limit"
		}
		total += ((len(page) + 2) / 3) * 4
		if total > maxRequestBytes {
			return p, "over_check_limit"
		}
		p.request.Images = append(p.request.Images, page)
	}
	encoded, err := json.Marshal(messageParams(p.request))
	if err != nil || len(encoded)+len(`,"stream":true`) > maxRequestBytes {
		return p, "over_check_limit"
	}
	return p, ""
}
