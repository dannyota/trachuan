package ocrvision

import (
	"context"
	"os"
	"sync"
)

const maxTextBytes = 50_000_000

func (r *Runner) pages(ctx context.Context, images []string, b *budget) ([]string, string) {
	limit := r.textLimit
	if limit <= 0 || limit > maxTextBytes {
		limit = maxTextBytes
	}
	// Reserve the separators before accepting any page text.
	size := max(0, 2*(len(images)-1))
	if size > limit {
		return nil, "failed"
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	texts := make([]string, len(images))
	jobs := make(chan int)
	var wg sync.WaitGroup
	var mu sync.Mutex
	outcome := ""
	for range min(8, len(images)) {
		wg.Go(func() {
			for i := range jobs {
				var text string
				info, err := os.Stat(images[i])
				reason := ""
				if err != nil || !info.Mode().IsRegular() {
					reason = "render_failed"
				} else if info.Size() > 7_000_000 {
					reason = "page_too_large"
				} else {
					var data []byte
					read := r.readPage
					if read == nil {
						read = os.ReadFile
					}
					data, err = read(images[i])
					if err != nil {
						reason = "render_failed"
					} else if len(data) > 7_000_000 {
						reason = "page_too_large"
					} else {
						text, err = r.Client.recognize(ctx, data, b)
					}
				}
				mu.Lock()
				if reason != "" {
					outcome = reason
					cancel()
				} else if err != nil && outcome == "" {
					outcome = "failed"
				} else if len(text) > limit-size {
					outcome = "failed"
					cancel()
				} else if ctx.Err() == nil {
					size += len(text)
					texts[i] = text
				}
				mu.Unlock()
			}
		})
	}
send:
	for i := range images {
		select {
		case jobs <- i:
		case <-ctx.Done():
			break send
		}
	}
	close(jobs)
	wg.Wait()
	if ctx.Err() != nil && outcome == "" {
		outcome = "failed"
	}
	return texts, outcome
}
