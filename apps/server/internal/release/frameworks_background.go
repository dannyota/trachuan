package release

import (
	"context"
	"sync"
	"time"

	"danny.vn/trachuan/internal/frameworks/serve"
	"golang.org/x/sync/singleflight"
)

func (r *Resolver) WatchFrameworks(ctx context.Context) {
	if r.Frameworks == nil {
		return
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	r.watchFrameworks(ctx, ticker.C)
}

func (r *Resolver) watchFrameworks(ctx context.Context, ticks <-chan time.Time) {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	if r.workCtx == nil {
		r.workCtx, r.cancelWork = context.WithCancel(ctx)
	}
	work := r.workCtx
	r.mu.Unlock()
	var pending sync.WaitGroup
	defer pending.Wait()
	for {
		if opened := r.refreshFrameworks(work); opened != nil {
			pending.Go(func() { <-opened })
		}
		select {
		case <-work.Done():
			return
		case <-ticks:
		}
	}
}

func (r *Resolver) refreshFrameworks(ctx context.Context) <-chan singleflight.Result {
	name, _, _, err := r.resolveName(ctx, "vn", "")
	if err != nil {
		return nil
	}
	r.mu.Lock()
	entry := r.entries[name]
	r.mu.Unlock()
	if entry != nil {
		return nil
	}
	return r.startOpen("vn", name)
}

func (r *Resolver) readyFrameworks(name, rel string, generation uint64) (*Lease, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	entry := r.entries[name]
	if r.closed || r.active["vn"] != name || entry == nil || entry.opening || entry.retired || entry.openErr != nil || entry.frameworks == nil {
		return nil, serve.ErrUnavailable
	}
	return r.leaseLocked(entry, rel, generation), nil
}
