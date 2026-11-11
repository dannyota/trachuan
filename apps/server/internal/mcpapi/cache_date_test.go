package mcpapi

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"danny.vn/trachuan/internal/retrieval"
)

type dateEngine struct {
	Engine
	clock func() time.Time
	date  string
}

func (e *dateEngine) Search(_ context.Context, req retrieval.Request) (retrieval.Response, error) {
	instant := req.Instant
	if instant.IsZero() {
		instant = e.clock()
	}
	e.date = instant.In(time.FixedZone("Asia/Ho_Chi_Minh", 7*60*60)).Format(time.DateOnly)
	return retrieval.Response{}, nil
}
func (e *dateEngine) Present(context.Context, retrieval.Response, retrieval.Detail) (retrieval.SearchResult, error) {
	return retrieval.SearchResult{Release: "20990101", Message: e.date}, nil
}

type dateResolver struct{ engine *dateEngine }

func (r dateResolver) Resolve(context.Context, string, string) (Engine, string, func(), error) {
	return r.engine, "20990101", func() {}, nil
}

func TestSearchCache_OneInstantAcrossVietnamMidnight(t *testing.T) {
	before := time.Date(2099, 1, 1, 16, 59, 59, 0, time.UTC)
	calls := 0
	clock := func() time.Time {
		calls++
		if calls == 1 {
			return before
		}
		return before.Add(time.Second)
	}
	e := &dateEngine{clock: clock}
	c := NewCaches(clock)
	c.ActivateRelease("20990101")
	cfg := Config{Resolver: dateResolver{e}, Caches: c, Clock: clock}
	input := searchInput{Country: "vn", Query: "ngân hàng"}
	miss, _, err := cfg.cachedSearch(context.Background(), input, func(any) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	calls = 0
	hit, _, err := cfg.cachedSearch(context.Background(), input, func(any) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	calls = 0
	cfg.Caches = nil
	cold, err := cfg.search(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	raw, _, err := encodeResult(cold, false)
	if err != nil {
		t.Fatal(err)
	}
	if string(miss.StructuredContent.(json.RawMessage)) != string(raw.StructuredContent.(json.RawMessage)) || string(hit.StructuredContent.(json.RawMessage)) != string(raw.StructuredContent.(json.RawMessage)) || cold.Message != "2099-01-01" {
		t.Fatal("cache key and eligibility crossed midnight independently")
	}
}

func TestSearch_RequestInstantSampledOnce(t *testing.T) {
	before := time.Date(2099, 1, 1, 16, 59, 59, 0, time.UTC)
	var calls atomic.Int64
	h := newHarness(t, func(cfg *Config) {
		cfg.Clock = func() time.Time { return before.Add(time.Duration(calls.Add(1)-1) * time.Second) }
	})
	callTool(t, h, "search", searchArgs())
	if calls.Load() != 1 || !h.engine.request.Instant.Equal(before) {
		t.Fatal("tool sampled the injected request clock more than once", calls.Load())
	}
}
