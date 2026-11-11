package ocrcheck

import (
	"bytes"
	"context"
	"crypto/sha256"
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
	if info.Size() > 30_000_000 {
		return p, "over_check_limit"
	}
	data, err := io.ReadAll(io.LimitReader(f, 30_000_001))
	if err != nil {
		return p, "pdf_missing"
	}
	if len(data) > 30_000_000 {
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
	if pages > 600 {
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
			return p, "schema"
		}
	}
	p.pages = pages
	p.request = Request{Model: o.Model, Effort: o.Effort, PDF: data, Text: text}
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
