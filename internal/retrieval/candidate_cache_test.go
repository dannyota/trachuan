package retrieval

import (
	"context"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/internal/embedding"
	"danny.vn/trachuan/internal/retrieval/lexical"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func candidateEngine() *Engine {
	return &Engine{
		queryRecipe: embedding.QueryRecipe1(),
		release:     Release{Rel: "20990101", RetrievalRecipe: Recipe2(), LexicalRecipe: lexical.Recipe1()},
		options:     Options{Profile: "serving", Now: func() time.Time { return time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC) }},
	}
}

func TestCandidateKey_Fields(t *testing.T) {
	date := "2099-01-01"
	req := Request{Query: "  ngân hàng  ", Mode: "ann"}
	depths := Depths{50, 50}
	base := candidateEngine().candidateKey(req, depths, date)
	for _, tc := range []struct {
		name   string
		change func(*Engine, *Request, *Depths, *string)
	}{
		{"query", func(_ *Engine, r *Request, _ *Depths, _ *string) { r.Query = "ngân hàng" }},
		{"collection date", func(_ *Engine, _ *Request, _ *Depths, d *string) { *d = "2099-01-02" }},
		{"as of same collection day", func(_ *Engine, r *Request, _ *Depths, _ *string) { r.AsOf = &Date{2099, time.January, 1} }},
		{"mode", func(_ *Engine, r *Request, _ *Depths, _ *string) { r.Mode = "exact" }},
		{"dense depth", func(_ *Engine, _ *Request, d *Depths, _ *string) { d.Dense++ }},
		{"lexical depth", func(_ *Engine, _ *Request, d *Depths, _ *string) { d.Lexical++ }},
		{"query recipe", func(e *Engine, _ *Request, _ *Depths, _ *string) { e.queryRecipe.Model = "other" }},
		{"retrieval recipe", func(e *Engine, _ *Request, _ *Depths, _ *string) { e.release.RetrievalRecipe.Version++ }},
		{"lexical recipe", func(e *Engine, _ *Request, _ *Depths, _ *string) { e.release.LexicalRecipe.Version++ }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, other, depth, day := candidateEngine(), req, depths, date
			tc.change(e, &other, &depth, &day)
			cache := &memoryCache{}
			cache.Put(e.release.Rel, base, []byte("entry"))
			if _, hit := cache.Get(e.release.Rel, e.candidateKey(other, depth, day)); hit {
				t.Fatal("changed collection input hit")
			}
		})
	}
	e := candidateEngine()
	req.AsOf = &Date{2098, time.May, 1}
	first := e.candidateKey(req, depths, date)
	req.AsOf = &Date{2098, time.June, 1}
	if first == e.candidateKey(req, depths, date) {
		t.Fatal("different as_of dates on the same collection day collided")
	}
}

func TestCandidateKey_PresentationAndCursor(t *testing.T) {
	e := candidateEngine()
	req, cursor, err := e.searchRequest(Request{Query: "ngân hàng", Mode: "ann"})
	if err != nil {
		t.Fatal(err)
	}
	depths := Depths{50, 50}
	key := e.candidateKey(req, depths, e.searchDate(req))
	force := false
	other := req
	other.Nodes, other.Documents = []string{"1"}, []string{"vn:1"}
	other.DocTypes, other.Issuers = []string{"law"}, []string{"issuer"}
	other.Limit, other.InForceOnly, other.Arms = 1, &force, "lexical"
	if key != e.candidateKey(other, depths, e.searchDate(other)) {
		t.Fatal("presentation changed key")
	}
	other = req
	other.Mode = ""
	if key != e.candidateKey(other, depths, e.searchDate(other)) {
		t.Fatal("default ANN changed key")
	}
	cursor.TakenPrimary = 3
	other = req
	other.Cursor = encodeSearchCursor(cursor)
	other.Instant = other.Instant.AddDate(0, 0, 1)
	other, _, err = e.searchRequest(other)
	if err != nil || key != e.candidateKey(other, depths, e.searchDate(other)) {
		t.Fatal("cursor did not pin cache date", err)
	}
	other.Cursor = "invalid"
	if _, _, err = e.searchRequest(other); err == nil {
		t.Fatal("invalid cursor accepted")
	}
}

type candidateCacheSpy struct {
	memoryCache
	gets int
}

func (c *candidateCacheSpy) Get(rel string, key [32]byte) ([]byte, bool) {
	c.gets++
	return c.memoryCache.Get(rel, key)
}

type candidateTx struct {
	pgx.Tx
	queries int
	err     error
}

func (tx *candidateTx) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, tx.err
}

func (tx *candidateTx) Query(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
	tx.queries++
	if tx.err != nil {
		return nil, tx.err
	}
	if strings.HasPrefix(sql, "WITH pool AS MATERIALIZED") {
		yes, no := true, false
		return &cacheRows{values: [][]any{{int64(1), .75, &yes}, {int64(2), .5, &no}}}, nil
	}
	id := int64(3)
	if !args[3].(bool) {
		id = 4
	}
	return &cacheRows{values: [][]any{{id, .625}}}, nil
}

func TestCandidates_HitEqualsMissAndOwnsLists(t *testing.T) {
	for _, tc := range []struct {
		name  string
		mode  Mode
		depth int
	}{{"ann fallback", "ann", 2}, {"ann without fallback", "ann", 1}, {"exact", "exact", 2}} {
		t.Run(tc.name, func(t *testing.T) {
			e, cache, tx := candidateEngine(), &candidateCacheSpy{}, &candidateTx{}
			p := Prepared{Request: Request{Query: "ngân hàng", Mode: tc.mode}, Validation: Validation{Accepted: true}}
			depths := Depths{tc.depth, 2}
			date := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
			miss, save, err := e.candidates(t.Context(), tx, p, "[1]", nil, depths, date, false, nil, cache)
			if err != nil || save == nil {
				t.Fatal("miss did not collect", err)
			}
			if cache.puts != 0 {
				t.Fatal("stored before collection succeeded")
			}
			for i, group := range miss {
				wantDense := []Candidate{{ChunkID: int64(i + 3), Cosine: .625, Rank: 1}}
				var wantRaw []Candidate
				if tc.mode == "ann" {
					wantRaw = []Candidate{{ChunkID: int64(i + 1), Cosine: .75 - float64(i)*.25, Rank: 1}}
				}
				if tc.mode == "ann" && tc.depth == 1 {
					wantDense = wantRaw
				}
				wantLexical := []Candidate{{ChunkID: int64(i + 3), BM25: .625, Rank: 1}}
				if !reflect.DeepEqual(group.Dense, wantDense) || !reflect.DeepEqual(group.RawANN, wantRaw) || !reflect.DeepEqual(group.Lexical, wantLexical) || group.Fallback != (tc.mode == "ann" && tc.depth == 2) {
					t.Fatal("candidate collection changed", group)
				}
			}
			save()
			queries := tx.queries
			hit, save, err := e.candidates(t.Context(), tx, p, "[1]", nil, depths, date, false, nil, cache)
			if err != nil || save != nil || !reflect.DeepEqual(hit, miss) || tx.queries != queries {
				t.Fatal("hit changed lists or queried", err)
			}
			for i := range hit {
				for _, list := range [][]Candidate{hit[i].RawANN, hit[i].Dense, hit[i].Lexical} {
					if len(list) > 0 {
						list[0].ChunkID = 999
						list[0].Cosine = -1
						list[0].BM25 = -2
					}
				}
				hit[i].Fallback = !hit[i].Fallback
			}
			again, _, err := e.candidates(t.Context(), tx, p, "[1]", nil, depths, date, false, nil, cache)
			if err != nil || !reflect.DeepEqual(again, miss) {
				t.Fatal("caller mutated cache", err)
			}
			uncached, _, err := e.candidates(t.Context(), tx, p, "[1]", nil, depths, date, false, nil, nil)
			if err != nil || !reflect.DeepEqual(uncached, miss) {
				t.Fatal("nil cache changed lists", err)
			}
		})
	}
}

func TestCandidates_BypassesAndFailures(t *testing.T) {
	for _, name := range []string{"evaluation", "not yet", "precomputed", "nil", "canceled", "timeout", "error", "cancel after miss"} {
		t.Run(name, func(t *testing.T) {
			e, spy, tx := candidateEngine(), &candidateCacheSpy{}, &candidateTx{}
			var cache CandidateCache = spy
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			var source *LexicalArms
			if name == "evaluation" {
				e.options.Profile = "evaluation"
			}
			if name == "precomputed" {
				source = &LexicalArms{Depth: 2}
			}
			if name == "nil" {
				cache = nil
			}
			if name == "canceled" {
				cancel()
			}
			if name == "timeout" {
				var stop context.CancelFunc
				ctx, stop = context.WithDeadline(ctx, time.Time{})
				defer stop()
			}
			if name == "error" {
				tx.err = errors.New("synthetic database failure")
			}
			_, save, err := e.candidates(ctx, tx, Prepared{Request: Request{Query: "ngân hàng"}, Validation: Validation{Accepted: true}}, "[1]", nil, Depths{2, 2}, time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC), name == "not yet", source, cache)
			if name == "error" || name == "canceled" || name == "timeout" {
				if err == nil || save != nil {
					t.Fatal("failure can be cached", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if name == "cancel after miss" {
				cancel()
			}
			if save != nil {
				save()
			}
			if spy.puts != 0 || (name != "error" && name != "cancel after miss" && spy.gets != 0) {
				t.Fatal("bypassed request touched cache", spy.gets, spy.puts)
			}
		})
	}
}

func TestCandidateCache_ContextAndRejectedQueries(t *testing.T) {
	e, cache := candidateEngine(), &candidateCacheSpy{}
	threshold := .1
	e.release.RetrievalRecipe.Abstention.Threshold = &threshold
	e.options.Candidates = cache
	if e.candidateCache(t.Context()) != cache {
		t.Fatal("missing default cache")
	}
	ctx := WithCaches(t.Context(), nil, nil, nil)
	if e.candidateCache(ctx) != nil {
		t.Fatal("nil request handle acquired engine default")
	}
	ctx = WithCaches(t.Context(), nil, nil, cache)
	for _, query := range []string{"ngan hang", ""} {
		_, _ = e.Search(ctx, Request{Query: query})
	}
	p := Prepared{Validation: Validation{Accepted: true}}
	_, _ = e.Collect(ctx, p, Depths{2, 2})
	e.options.Profile = "evaluation"
	_, _ = e.CollectWithNotYetEligible(ctx, p, Depths{2, 2})
	_, _ = e.CollectWithLexical(ctx, p, Depths{2, 2}, LexicalArms{Depth: 2})
	if cache.gets != 0 || cache.puts != 0 {
		t.Fatal("non-serving collection touched cache")
	}
}

func TestCandidateEncoding_OnlyListsAndExactScores(t *testing.T) {
	groups := [2]ArmGroup{{Dense: []Candidate{{ChunkID: 1<<60 + 1, Rank: 200, Cosine: math.Nextafter(.5, 1), BM25: math.Copysign(0, -1), Content: "discard"}}, Members: []Candidate{{Content: "discard"}}}, {Lexical: []Candidate{}, Fallback: true}}
	raw := encodeCandidates(groups)
	got, ok := decodeCandidates(raw)
	if !ok || got[0].Dense[0].Content != "" || got[0].Members != nil || got[1].Lexical == nil || !got[1].Fallback {
		t.Fatal("encoded hydrated data or lost empty list")
	}
	a, b := groups[0].Dense[0], got[0].Dense[0]
	if a.ChunkID != b.ChunkID || a.Rank != b.Rank || math.Float64bits(a.Cosine) != math.Float64bits(b.Cosine) || math.Float64bits(a.BM25) != math.Float64bits(b.BM25) {
		t.Fatal("score bits changed")
	}
	for _, bad := range [][]byte{nil, raw[:len(raw)-1], append(append([]byte{}, raw...), 0)} {
		if _, ok := decodeCandidates(bad); ok {
			t.Fatal("accepted malformed entry")
		}
	}
}

func TestCandidatePublication_OnlySuccessfulSearchStores(t *testing.T) {
	cache := &candidateCacheSpy{}
	entry := candidatePublication{CandidateCache: cache}
	entry.Put("20990101", [32]byte{1}, []byte("candidates"))
	if cache.puts != 0 {
		t.Fatal("stored before search succeeded")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	entry.publish(ctx)
	if cache.puts != 0 {
		t.Fatal("canceled search stored")
	}
	entry.publish(t.Context())
	if cache.puts != 1 {
		t.Fatal("successful search did not store")
	}
}

func BenchmarkCandidateCacheEntry(b *testing.B) {
	var groups [2]ArmGroup
	for i := range groups {
		for _, list := range []*[]Candidate{&groups[i].RawANN, &groups[i].Dense, &groups[i].Lexical} {
			for rank := 1; rank <= 50; rank++ {
				*list = append(*list, Candidate{ChunkID: int64(rank), Rank: rank, Cosine: .75, BM25: 1.25})
			}
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	var raw []byte
	for b.Loop() {
		raw = encodeCandidates(groups)
	}
	b.ReportMetric(float64(len(raw)), "value-B/entry")
	b.ReportMetric(float64(len(raw)+32+128), "charged-B/entry")
}
