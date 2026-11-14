//go:build integration

package retrieval

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestCandidateCache_SearchResponsesIdentical(t *testing.T) {
	for _, postings := range []bool{false, true} {
		for _, mode := range []Mode{"ann", "exact"} {
			t.Run(string(mode)+map[bool]string{false: "/sql", true: "/postings"}[postings], func(t *testing.T) {
				_, e, _ := fixture(t)
				e.options.Profile = "serving"
				if postings {
					outcome := make(chan PostingsOutcome, 1)
					e.StartSparsePostings(func(result PostingsOutcome) { outcome <- result })
					if result := <-outcome; result.Status != "ready" {
						t.Fatalf("postings: %+v", result)
					}
					t.Cleanup(e.Close)
				}
				cache := &candidateCacheSpy{}
				ctx := WithCaches(t.Context(), nil, nil, cache)
				cold := WithCaches(t.Context(), nil, nil, nil)
				req := Request{Query: "ngân hàng", Mode: mode, Instant: time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC), Limit: 1}
				check := func(req Request, detail Detail) Response {
					t.Helper()
					uncached, err := e.Search(cold, req)
					if err != nil {
						t.Fatal(err)
					}
					cached, err := e.Search(ctx, req)
					if err != nil {
						t.Fatal(err)
					}
					serialize := func(response Response) []byte {
						t.Helper()
						result, presentErr := e.Present(t.Context(), response, detail)
						if presentErr != nil {
							t.Fatal(presentErr)
						}
						raw, marshalErr := json.Marshal(result)
						if marshalErr != nil {
							t.Fatal(marshalErr)
						}
						return raw
					}
					if !bytes.Equal(serialize(uncached), serialize(cached)) {
						t.Fatal("cache changed serialized response")
					}
					return cached
				}
				first := check(req, DetailCompact)
				check(req, DetailCompact)
				filtered := req
				filtered.Documents = []string{"vn:1"}
				check(filtered, DetailCompact)
				check(req, DetailFull)
				if cache.hits != 3 || cache.puts != 1 {
					t.Fatalf("presentation did not reuse candidates: hits=%d puts=%d", cache.hits, cache.puts)
				}
				if first.NextCursor != "" {
					req.Cursor = first.NextCursor
					req.Instant = req.Instant.AddDate(0, 0, 1)
					check(req, DetailFull)
					if cache.hits != 4 || cache.puts != 1 {
						t.Fatal("cursor missed pinned date")
					}
				}
			})
		}
	}
}

func TestCandidateCache_OnlyServingSearchStores(t *testing.T) {
	_, e, _ := fixture(t)
	e.options.Profile = "serving"
	cache := &candidateCacheSpy{}
	ctx := WithCaches(t.Context(), nil, nil, cache)
	req := Request{Query: "ngân hàng", Mode: "exact", Instant: time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)}
	p, err := e.Prepare(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	depths := Depths{2, 2}
	arms, err := e.Collect(ctx, p, depths)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.CollectWithLexical(ctx, p, depths, LexicalArms{Primary: arms.Primary.Lexical, NonCurrent: arms.NonCurrent.Lexical, Depth: depths.Lexical}); err != nil {
		t.Fatal(err)
	}
	e.options.Profile = "evaluation"
	if _, err = e.Search(ctx, req); err != nil {
		t.Fatal(err)
	}
	if _, err = e.CollectWithNotYetEligible(ctx, p, depths); err != nil {
		t.Fatal(err)
	}
	if cache.gets != 0 || cache.puts != 0 {
		t.Fatal("evaluation or direct collection used cache")
	}
	e.options.Profile = "serving"
	for _, query := range []string{"ngan hang", ""} {
		_, _ = e.Search(ctx, Request{Query: query})
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	_, _ = e.Search(canceled, req)
	expired, stop := context.WithDeadline(ctx, time.Time{})
	defer stop()
	_, _ = e.Search(expired, req)
	if cache.gets != 0 || cache.puts != 0 {
		t.Fatal("rejection or cancellation used cache")
	}
	_, cursor, err := e.searchRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	cursor.TakenPrimary = 150
	req.Cursor = encodeSearchCursor(cursor)
	if _, err = e.Search(ctx, req); !errors.Is(err, ErrInvalidRequest) {
		t.Fatal("expected page position error", err)
	}
	if cache.puts != 0 {
		t.Fatal("search error published candidates")
	}
}
