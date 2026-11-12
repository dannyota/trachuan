package ocrvision

import (
	"context"
	"sync"
)

const maxTextBytes = 50_000_000

func (r *Runner) pages(ctx context.Context, images [][]byte, b *budget) ([]string, bool) {
	limit := r.textLimit
	if limit <= 0 || limit > maxTextBytes {
		limit = maxTextBytes
	}
	// Reserve the separators before accepting any page text.
	size := max(0, 2*(len(images)-1))
	if size > limit {
		return nil, false
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	texts := make([]string, len(images))
	jobs := make(chan int)
	var wg sync.WaitGroup
	var mu sync.Mutex
	success := true
	for range min(4, len(images)) {
		wg.Go(func() {
			for i := range jobs {
				text, err := r.Client.recognize(ctx, images[i], b)
				mu.Lock()
				if err != nil {
					success = false
				} else if len(text) > limit-size {
					success = false
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
	return texts, success && ctx.Err() == nil
}
