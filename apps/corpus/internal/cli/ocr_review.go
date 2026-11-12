package cli

import (
	"context"
	"fmt"
	"io"

	"danny.vn/trachuan/apps/corpus/internal/ocrcheck"
	"danny.vn/trachuan/apps/corpus/internal/ocrstore"
	"danny.vn/trachuan/apps/corpus/internal/store"
)

func runOCRReview(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) < 2 {
		_, _ = fmt.Fprintln(stderr, "usage: corpus ocr review <cc> <rel> [--model MODEL] [--effort EFFORT] [--check-version 2|3|4]")
		return 2
	}
	o, err := parseCheckOptions(args[2:], true)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 2
	}
	env, err := openCheckRun(ctx, args[0], args[1])
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	defer env.db.Close()
	docs, err := env.review(ctx, o.CheckerID(), o.CheckVersion >= 3)
	if err == nil {
		err = ocrcheck.WriteReview("ocr-review.json", docs)
	}
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "ocr-review.json: %d documents (%s)\n", len(docs), o.CheckerID())
	return 0
}
func (e *checkRun) review(ctx context.Context, checker string, allEntries bool) ([]ocrcheck.ReviewDocument, error) {
	q := store.New(e.db)
	source := ocrstore.Open(e.root, e.db)
	docs, err := e.checkDocuments(ctx, checker, allEntries)
	if err != nil {
		return nil, err
	}
	hashes := make([]string, 0, len(docs))
	for _, d := range docs {
		hashes = append(hashes, d.Entry.PdfSha256)
	}
	checks, err := q.ListOCRChecks(ctx, store.ListOCRChecksParams{CheckerID: checker, PdfSha256s: hashes})
	if err != nil {
		return nil, fmt.Errorf("list OCR review checks: %w", err)
	}
	output := []ocrcheck.ReviewDocument{}
	for _, d := range docs {
		for _, check := range checks {
			if check.OcrTextID != d.Entry.ID {
				continue
			}
			result, decodeErr := ocrstore.DecodeCheck(check.Result)
			if decodeErr != nil {
				return nil, decodeErr
			}
			text, readErr := source.ReadText(d.Entry)
			if readErr != nil {
				return nil, readErr
			}
			doc, reviewErr := ocrcheck.Review(d.Key, d.PDFName, d.Entry, checker, result, text)
			if reviewErr != nil {
				return nil, reviewErr
			}
			output = append(output, doc)
		}
	}
	return output, nil
}
