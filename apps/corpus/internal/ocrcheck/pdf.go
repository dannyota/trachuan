package ocrcheck

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"golang.org/x/sys/unix"
)

func countPages(ctx context.Context, data []byte) (int, error) {
	conf := model.NewStatelessConfiguration()
	// Keep parsing independent of user configuration, installed fonts and network.
	conf.Offline = true
	return api.PageCount(ctx, bytes.NewReader(data), conf)
}

// CountPages counts verified PDF bytes with offline parsing and a ten-second limit.
// Parser panics become errors, as they do in the cross-check.
func CountPages(ctx context.Context, data []byte) (int, error) {
	deadline, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	pages, err := recoverCount(deadline, countPages, data)
	if err != nil {
		return 0, fmt.Errorf("count PDF pages: %w", err)
	}
	if err = deadline.Err(); err != nil {
		return 0, fmt.Errorf("count PDF pages: %w", err)
	}
	return pages, nil
}

// preRequestFailure identifies reasons produced while preparing a request.
func preRequestFailure(reason string) bool {
	switch reason {
	case "pdf_missing", "pdf_hash_mismatch", "over_check_limit", "page_count_unknown", "page_count_mismatch", "ocr_text_unreadable", "ocr_marker_collision", "render_failed":
		return true
	default:
		return false
	}
}

func (r *Runner) prepare(ctx context.Context, doc Document, o Options) (p prepared, reason string) {
	p = prepared{doc: doc}
	fd, err := unix.Open(doc.PDFPath, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return p, "pdf_missing"
	}
	f := os.NewFile(uintptr(fd), "OCR PDF")
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return p, "pdf_missing"
	}
	if info.Size() > 50_000_000 {
		return p, "over_check_limit"
	}
	data, err := io.ReadAll(io.LimitReader(f, 50_000_001))
	if err != nil {
		return p, "pdf_missing"
	}
	if len(data) > 50_000_000 {
		return p, "over_check_limit"
	}
	if fmt.Sprintf("%x", sha256.Sum256(data)) != doc.Entry.PdfSha256 {
		return p, "pdf_hash_mismatch"
	}
	deadline, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	pages, countErr := recoverCount(deadline, r.CountPages, data)
	if countErr != nil || deadline.Err() != nil || pages < 1 {
		return p, "page_count_unknown"
	}
	p.pages = pages
	if pages > maxPages {
		return p, "over_check_limit"
	}
	if doc.Entry.Pages.Valid && pages != int(doc.Entry.Pages.Int32) {
		return p, "page_count_mismatch"
	}
	text := doc.Text
	if doc.ReadText != nil {
		var textErr error
		text, textErr = doc.ReadText()
		if textErr != nil {
			return p, "ocr_text_unreadable"
		}
	}
	if bytes.Contains(text, []byte(ocrBegin)) || bytes.Contains(text, []byte(ocrEnd)) {
		return p, "ocr_marker_collision"
	}
	p.request = Request{Model: o.Model, Effort: o.Effort, Text: text}
	if v, _ := o.version(); v.directOnly {
		p.request.MediaType = v.mediaType
		return r.prepareJPEG(ctx, p, data, v.dpi)
	}
	base, encodeErr := json.Marshal(messageParams(p.request))
	if encodeErr != nil {
		return p, "over_check_limit"
	}
	allowance := maxRequestBytes - len(base) - len(`,"stream":true`)
	if allowance <= 0 {
		return p, "over_check_limit"
	}
	renderer := r.Renderer
	if renderer == nil {
		renderer = &PopplerRenderer{}
	}
	rendered, renderErr := renderer.Render(ctx, data, allowance)
	p.rendering = &rendered.Stats
	if renderErr != nil {
		if errors.Is(renderErr, renderLimit) {
			return p, "over_check_limit"
		}
		return p, "render_failed"
	}
	if len(rendered.Pages) > maxPages {
		return p, "over_check_limit"
	}
	if len(rendered.Pages) != pages {
		return p, "page_count_mismatch"
	}
	total := len(text)
	for _, page := range rendered.Pages {
		if reason := validateImage(page); reason != "" {
			return p, reason
		}
		total += ((len(page) + 2) / 3) * 4
		if total >= maxRequestBytes {
			return p, "over_check_limit"
		}
	}
	p.request.Images = rendered.Pages
	encoded, encodeErr := json.Marshal(messageParams(p.request))
	p.requestBytes = len(encoded) + len(`,"stream":true`)
	// Direct streams add the stream field; reserve it for both modes.
	if encodeErr != nil || len(encoded)+len(`,"stream":true`) >= maxRequestBytes {
		return p, "over_check_limit"
	}
	return p, ""
}
func recoverCount(ctx context.Context, count func(context.Context, []byte) (int, error), data []byte) (pages int, err error) {
	defer func() {
		if recover() != nil {
			pages = 0
			err = fmt.Errorf("PDF page count failed")
		}
	}()
	return count(ctx, data)
}
