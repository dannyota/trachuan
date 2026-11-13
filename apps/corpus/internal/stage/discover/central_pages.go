package discover

import (
	"context"
	"errors"
	"sync"

	"danny.vn/trachuan/apps/corpus/internal/sourcegate"
	"danny.vn/trachuan/apps/corpus/internal/vbpl"
)

type centralPageResult struct {
	page int
	data vbpl.ListPage
	err  error
}

type centralPages struct {
	ctx         context.Context
	cancel      context.CancelFunc
	client      *vbpl.Client
	results     chan centralPageResult
	pending     map[int]context.CancelFunc
	ready       map[int]centralPageResult
	next        int
	failure     int
	paused      error
	mu          sync.Mutex
	cancels     map[int]context.CancelFunc
	cancelAfter int
}

func newCentralPages(ctx context.Context, client *vbpl.Client) *centralPages {
	ctx, cancel := context.WithCancel(ctx)
	return &centralPages{ctx: ctx, cancel: cancel, client: client, results: make(chan centralPageResult, 8), pending: make(map[int]context.CancelFunc), ready: make(map[int]centralPageResult), cancels: make(map[int]context.CancelFunc), next: 1}
}

func (p *centralPages) start(page int) {
	ctx, cancel := context.WithCancel(p.ctx)
	p.pending[page] = cancel
	p.mu.Lock()
	p.cancels[page] = cancel
	if p.cancelAfter != 0 && page > p.cancelAfter {
		cancel()
	}
	p.mu.Unlock()
	go func() {
		data, err := p.client.List(ctx, vbpl.ListQuery{Page: page, PageSize: 1000, Level: "TRUNG_UONG"})
		if errors.Is(err, sourcegate.ErrSourcePaused) {
			p.cancel()
		}
		p.mu.Lock()
		delete(p.cancels, page)
		if err != nil && (p.cancelAfter == 0 || page < p.cancelAfter) {
			p.cancelAfter = page
			for index, cancelPage := range p.cancels {
				if index > page {
					cancelPage()
				}
			}
		}
		p.mu.Unlock()
		p.results <- centralPageResult{page: page, data: data, err: err}
	}()
}

func (p *centralPages) receive() {
	res := <-p.results
	p.pending[res.page]()
	delete(p.pending, res.page)
	p.ready[res.page] = res
	if errors.Is(res.err, sourcegate.ErrSourcePaused) {
		p.paused = res.err
		for _, cancel := range p.pending {
			cancel()
		}
	} else if res.err != nil && (p.failure == 0 || res.page < p.failure) {
		p.failure = res.page
		for page, cancel := range p.pending {
			if page > res.page {
				cancel()
			}
		}
	}
}

func (p *centralPages) get(page int) (vbpl.ListPage, error) {
	for len(p.results) > 0 {
		p.receive()
	}
	if p.paused != nil {
		return vbpl.ListPage{}, p.paused
	}
	end := page + 7
	if page == 1 {
		end = 1
	}
	if end > 100001 {
		end = 100001
	}
	for p.next <= end && (p.failure == 0 || p.next < p.failure) {
		p.start(p.next)
		p.next++
	}
	for {
		if p.paused != nil {
			return vbpl.ListPage{}, p.paused
		}
		if res, ok := p.ready[page]; ok {
			delete(p.ready, page)
			return res.data, res.err
		}
		p.receive()
	}
}

func (p *centralPages) close() error {
	p.cancel()
	for _, cancel := range p.pending {
		cancel()
	}
	for len(p.pending) != 0 {
		p.receive()
	}
	return p.paused
}
