package build

import (
	"sync"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/stage/chunk"
	"danny.vn/trachuan/apps/corpus/internal/stage/fetch"
)

const identityConcurrency = 8

type identityResult struct {
	detail fetch.Document
	nodes  []chunk.Node
	err    error
}

type identityPool struct {
	jobs    chan int
	results []chan identityResult
	done    chan struct{}
	next    int
	wg      sync.WaitGroup
}

func newIdentityPool(count, limit int, work func(int) identityResult) *identityPool {
	p := &identityPool{jobs: make(chan int, limit), results: make([]chan identityResult, count), done: make(chan struct{})}
	for i := range p.results {
		p.results[i] = make(chan identityResult, 1)
	}
	for i := 0; i < limit; i++ {
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			for {
				select {
				case <-p.done:
					return
				case index, ok := <-p.jobs:
					if !ok {
						return
					}
					p.results[index] <- work(index)
				}
			}
		}()
	}
	for p.next < count && p.next < limit {
		p.jobs <- p.next
		p.next++
	}
	return p
}

func (p *identityPool) take(index int) identityResult {
	result := <-p.results[index]
	if p.next < len(p.results) {
		p.jobs <- p.next
		p.next++
	}
	return result
}

func (p *identityPool) stop() {
	close(p.done)
	close(p.jobs)
	p.wg.Wait()
}

func readIdentityFetch(artifacts artifact.Store, item InventoryItem) identityResult {
	dir, err := artifactDir(artifacts, item)
	if err != nil {
		return identityResult{err: identityFetchError(err, false)}
	}
	detail, err := fetch.ReadDocument(dir, item.ItemKey)
	if err != nil {
		return identityResult{err: identityFetchError(err, true)}
	}
	return identityResult{detail: detail}
}
