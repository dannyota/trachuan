package ocrvision

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"

	"danny.vn/trachuan/apps/corpus/internal/ocrcheck"
	"github.com/jackc/pgx/v5/pgtype"
)

type textStore interface {
	Put(context.Context, string, string, string, pgtype.Int4, []byte) (bool, error)
}
type renderer interface {
	Preflight(context.Context) error
	RenderDPI(context.Context, []byte, int, int) (ocrcheck.Rendered, error)
}

// Runner processes documents serially and pages with at most four workers.
type Runner struct {
	Client     *Client
	Store      textStore
	Renderer   renderer
	CountPages func(context.Context, []byte) (int, error)
}

// Document names a PDF listed in a verified fetch bundle.
type Document struct {
	Hash, Path string
	Present    bool
}

// Options bounds billable requests, including retries.
type Options struct {
	MaxPages, LimitDocs int
	DryRun              bool
}

// Summary counts document outcomes and actual request attempts.
type Summary struct{ Stored, Present, Failed, Skipped, Requests, Pages, Documents int }

type result struct {
	pages, requests int
	outcome         string
}

// Run prints stable content-free outcomes and fails if any document failed.
func (r *Runner) Run(ctx context.Context, docs []Document, o Options, out io.Writer) (Summary, error) {
	var summary Summary
	if o.MaxPages < 1 || o.LimitDocs < 0 {
		return summary, fmt.Errorf("positive --max-pages and nonnegative --limit-docs required")
	}
	if !o.DryRun {
		if err := r.Client.Preflight(ctx); err != nil {
			return summary, err
		}
		if r.Renderer == nil {
			r.Renderer = &ocrcheck.PopplerRenderer{}
		}
		if err := r.Renderer.Preflight(ctx); err != nil {
			return summary, err
		}
	}
	if r.CountPages == nil {
		r.CountPages = ocrcheck.CountPages
	}
	b := newBudget(o.MaxPages)
	for i, doc := range docs {
		if o.LimitDocs > 0 && i >= o.LimitDocs {
			break
		}
		res := r.document(ctx, doc, o, b)
		summary.Documents++
		summary.Pages += res.pages
		summary.Requests += res.requests
		if o.DryRun && res.outcome == "" {
			_, _ = fmt.Fprintf(out, "pdf=%s pages=%d\n", doc.Hash, res.pages)
			continue
		}
		switch res.outcome {
		case "stored":
			summary.Stored++
		case "present":
			summary.Present++
		case "skipped_limit":
			summary.Skipped++
		default:
			summary.Failed++
		}
		_, _ = fmt.Fprintf(out, "pdf=%s pages=%d outcome=%s requests=%d\n", doc.Hash, res.pages, res.outcome, res.requests)
	}
	if o.DryRun {
		_, _ = fmt.Fprintf(out, "documents=%d pages=%d failed=%d\n", summary.Documents, summary.Pages, summary.Failed)
	} else {
		_, _ = fmt.Fprintf(out, "stored=%d present=%d failed=%d skipped_limit=%d requests=%d\n", summary.Stored, summary.Present, summary.Failed, summary.Skipped, summary.Requests)
	}
	if summary.Failed > 0 {
		return summary, fmt.Errorf("%d OCR documents failed", summary.Failed)
	}
	return summary, nil
}

func (r *Runner) document(ctx context.Context, doc Document, o Options, b *budget) (res result) {
	before := b.used()
	defer func() { res.requests = b.used() - before }()
	res.outcome = "failed"
	data, reason := readPDF(doc)
	if reason != "" {
		res.outcome = reason
		return res
	}
	pages, err := r.CountPages(ctx, data)
	if err != nil || pages < 1 {
		return res
	}
	res.pages = pages
	if o.DryRun {
		res.outcome = ""
		return res
	}
	if doc.Present {
		res.outcome = "present"
		return res
	}
	if pages > 500 {
		res.outcome = "too_many_pages"
		return res
	}
	if pages > b.remaining() {
		res.outcome = "skipped_limit"
		return res
	}
	images, err := r.Renderer.RenderDPI(ctx, data, int(^uint(0)>>1), 200)
	if err != nil || len(images.Pages) != pages {
		res.outcome = "render_failed"
		return res
	}
	for _, page := range images.Pages {
		if len(page) > 7_000_000 {
			res.outcome = "page_too_large"
			return res
		}
	}
	texts, ok := r.pages(ctx, images.Pages, b)
	if !ok {
		return res
	}
	text := strings.Join(texts, "\n\n")
	if strings.TrimSpace(text) == "" {
		res.outcome = "empty_text"
		return res
	}
	present, err := r.Store.Put(ctx, doc.Hash, EngineID, "vision", pgtype.Int4{Int32: int32(pages), Valid: true}, []byte(text))
	if err != nil {
		return res
	}
	res.outcome = "stored"
	if present {
		res.outcome = "present"
	}
	return res
}

func (r *Runner) pages(ctx context.Context, images [][]byte, b *budget) ([]string, bool) {
	texts := make([]string, len(images))
	jobs := make(chan int)
	var wg sync.WaitGroup
	var mu sync.Mutex
	success := true
	for range min(4, len(images)) {
		wg.Go(func() {
			for i := range jobs {
				text, err := r.Client.recognize(ctx, images[i], b)
				if err != nil {
					mu.Lock()
					success = false
					mu.Unlock()
				} else {
					texts[i] = text
				}
			}
		})
	}
	for i := range images {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	return texts, success
}
