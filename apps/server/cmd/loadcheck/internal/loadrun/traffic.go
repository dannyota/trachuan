package loadrun

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

func (r *runner) search(ctx context.Context, phase string, i int) {
	query, err := r.queries.next()
	if err != nil {
		r.fail(err)
		return
	}
	a := r.accounts[i%len(r.accounts)]
	payload, _ := json.Marshal(map[string]string{"country": "vn", "query": query})
	selected := false
	requestCtx, cancel := abortContext(ctx, selected)
	defer cancel()
	var abort context.CancelFunc
	if selected {
		abort = cancel
	}
	start := time.Now()
	res := send(requestCtx, a.client, http.MethodPost, r.config.API+"/api/v1/search", string(payload), a.addr, r.config.PublicURL, a.csrf, false, abort)
	r.output.add(Record{Phase: phase, Stream: "search", Start: start, DurationMS: float64(time.Since(start)) / float64(time.Millisecond), Status: res.status, Aborted: selected && requestCtx.Err() != nil})
}
func (r *runner) reader(ctx context.Context, phase string, i int) {
	a := r.accounts[i%len(r.accounts)]
	key := r.keys[i%len(r.keys)]
	selected := false
	requestCtx, cancel := abortContext(ctx, selected)
	defer cancel()
	var abort context.CancelFunc
	if selected {
		abort = cancel
	}
	start := time.Now()
	res := send(requestCtx, a.client, http.MethodGet, r.config.Pages+"/vn/documents/"+keyPath(key), "", a.addr, "", "", true, abort)
	if res.status >= 200 && res.status < 300 {
		res = send(requestCtx, a.client, http.MethodGet, r.config.API+"/api/v1/documents/"+keyPath(key), "", a.addr, "", "", false, abort)
	}
	r.output.add(Record{Phase: phase, Stream: "reader", Key: key, Start: start, DurationMS: float64(time.Since(start)) / float64(time.Millisecond), Status: res.status, Aborted: selected && requestCtx.Err() != nil})
}
func (r *runner) summary(ctx context.Context, phase, stream string, i int, key string, pass int) {
	target := r.config.Pages + "/vn/documents/" + keyPath(key) + "/summary"
	n := 16
	if phase == "D" {
		n = 40
	}
	if stream == "public_api" {
		target = r.config.API + "/api/v1/public/documents/" + keyPath(key)
		n = 50
	}
	if stream == "list" {
		target = r.config.API + "/api/v1/public/documents"
		n = 50
	}
	selected := phase == "D" && abortRequest(i, r.abortSeed)
	requestCtx, cancel := abortContext(ctx, selected)
	defer cancel()
	var abort context.CancelFunc
	if selected {
		abort = cancel
	}
	start := time.Now()
	var res response
	attempt := func() (bool, time.Duration) {
		res = send(requestCtx, r.client, http.MethodGet, target, "", address(i, n), "", "", stream == "summary", abort)
		return res.busy, res.retryAfter
	}
	retries := 0
	if phase == "B" {
		retries = retryBusy(attempt, func(delay time.Duration) {
			timer := time.NewTimer(delay)
			defer timer.Stop()
			select {
			case <-requestCtx.Done():
			case <-timer.C:
			}
		})
	} else {
		_, _ = attempt()
	}
	r.output.add(Record{Phase: phase, Stream: stream, Key: key, Pass: pass, Start: start, DurationMS: float64(time.Since(start)) / float64(time.Millisecond), Status: res.status, Retries: retries, Aborted: selected && requestCtx.Err() != nil})
}
func (r *runner) warm(ctx context.Context, duration time.Duration) {
	window := min(duration, 60*time.Second)
	rate := int((int64(1000)*int64(time.Second) + int64(window) - 1) / int64(window))
	ready := make([]chan struct{}, 500)
	for i := range ready {
		ready[i] = make(chan struct{})
	}
	schedule(ctx, window, rate, func(i int) {
		if i < 500 {
			defer close(ready[i])
			r.summary(ctx, "C", "summary", i, r.keys[i], 1)
		} else if i < 1000 {
			select {
			case <-ctx.Done():
				return
			case <-ready[i-500]:
			}
			r.summary(ctx, "C", "summary", i, r.keys[i-500], 2)
		}
	})
}
