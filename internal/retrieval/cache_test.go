package retrieval

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

type memoryCache struct {
	values map[[32]byte][]byte
	hits   int
	puts   int
}

func (c *memoryCache) MaxValueBytes() int64 { return 32_000_000 }

func (c *memoryCache) Get(_ string, k [32]byte) ([]byte, bool) {
	v, ok := c.values[k]
	if ok {
		c.hits++
	}
	return v, ok
}
func (c *memoryCache) Put(_ string, k [32]byte, v []byte) {
	if c.values == nil {
		c.values = make(map[[32]byte][]byte)
	}
	c.values[k] = v
	c.puts++
}

func TestDocumentCache_StatusNeverCached(t *testing.T) {
	cache := &memoryCache{}
	e := &Engine{release: Release{Rel: "20990101"}, options: Options{Documents: cache}}
	original := DocumentEntry{Title: "title", RelatedCounts: map[string]int{"in:implements": 1}, Status: Status{Class: "in_force", Current: true}}
	e.cacheDocument(context.Background(), "vn:1", original)
	for _, raw := range cache.values {
		if strings.Contains(string(raw), `"status"`) {
			t.Fatal("status retained")
		}
	}
	docs, err := e.Documents(context.Background(), []string{"vn:1"})
	if err != nil || docs["vn:1"].Status != (Status{}) {
		t.Fatal("status returned from metadata cache", err)
	}
	response := Response{Hits: []Hit{{Identity: Identity{DocumentKey: "vn:1"}, Stamp: Validity{StatusClass: "in_force", Current: true}}}}
	first := present(response, docs, DetailCompact)
	response.Hits[0].Stamp.Current = false
	second := present(response, docs, DetailCompact)
	if !first.Documents["vn:1"].Status.Current || second.Documents["vn:1"].Status.Current {
		t.Fatal("status reused")
	}
	docs["vn:1"].RelatedCounts["in:implements"] = 99
	again, err := e.Documents(context.Background(), []string{"vn:1"})
	if err != nil || again["vn:1"].RelatedCounts["in:implements"] != 1 {
		t.Fatal("caller mutated cached metadata")
	}
}

type cacheRows struct {
	pgx.Rows
	values [][]any
	at     int
}

func (r *cacheRows) Next() bool             { r.at++; return r.at <= len(r.values) }
func (r *cacheRows) Scan(dest ...any) error { assignCacheRow(dest, r.values[r.at-1]); return nil }
func (r *cacheRows) Err() error             { return nil }
func (r *cacheRows) Close()                 {}

type cacheRow []any

func (r cacheRow) Scan(dest ...any) error { assignCacheRow(dest, r); return nil }
func assignCacheRow(dest, values []any) {
	for i, v := range values {
		if v != nil {
			reflect.ValueOf(dest[i]).Elem().Set(reflect.ValueOf(v))
		}
	}
}

type readCacheTx struct {
	pgx.Tx
	queries       int
	relationCount int
}

func (tx *readCacheTx) QueryRow(context.Context, string, ...any) pgx.Row {
	tx.queries++
	end := time.Date(2099, 1, 2, 0, 0, 0, 0, time.UTC)
	return cacheRow{int64(1), "number", "title", "law", "issuer", nil, nil, &end, "", "in_force", "source", true, false}
}
func (tx *readCacheTx) Query(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
	tx.queries++
	r := &cacheRows{}
	if strings.Contains(sql, "SELECT id,parent_id,kind") {
		r.values = [][]any{{int64(1), nil, "dieu", "Điều 1", "heading", 1, 1, strings.Repeat("điều luật ", 12000), "p1", nil}}
	}
	if sql == readRelationsSQL {
		offset := args[5].(int)
		total := tx.relationCount
		if total == 0 {
			total = 2
		}
		for i := offset; i < min(total, offset+RelationPageSize); i++ {
			r.values = append(r.values, []any{"in", "implements", "vn:2", "two", true, "raw", "evidence", total, false})
		}
	}
	return r, nil
}

func TestReadCache_NextDecodedFirstAndPagingIdentical(t *testing.T) {
	for _, item := range []ReadItem{{Handle: "vn:1"}, {Handle: "vn:1#p1"}, {Handle: "vn:1", From: "p1", To: "p1"}, {Handle: "vn:1", Relations: true}} {
		t.Run(item.Handle+item.From+map[bool]string{true: "relations"}[item.Relations], func(t *testing.T) {
			now := time.Date(2099, 1, 1, 16, 59, 59, 0, time.UTC)
			cache := &memoryCache{}
			e := &Engine{release: Release{Rel: "20990101", RetrievalRecipe: Recipe1()}, options: Options{Now: func() time.Time { return now }, Sequences: cache}}
			tx := &readCacheTx{}
			chain := func(cached bool) []string {
				t.Helper()
				e.options.Sequences = nil
				if cached {
					e.options.Sequences = cache
				}
				original := item
				var pages []string
				for i := 0; i < 30; i++ {
					c, code := readCursor(original, e.release.Rel)
					if code != "" {
						t.Fatal(code)
					}
					page, _, save, err := e.readItem(context.Background(), tx, c, original.Next != "", ReadCallCap, true)
					if err != nil || page.Error != "" {
						t.Fatal(page.Error, err)
					}
					if save != nil {
						save()
					}
					raw, _ := json.Marshal(page)
					pages = append(pages, string(raw))
					if page.Next == "" {
						return pages
					}
					original = ReadItem{Next: page.Next}
				}
				t.Fatal("chain did not finish")
				return nil
			}
			cold := chain(false)
			warm := chain(true)
			if item.Handle == "vn:1#p1" {
				for _, raw := range cache.values {
					var stored map[string]json.RawMessage
					_ = json.Unmarshal(raw, &stored)
					if len(stored["Parts"]) == 0 {
						t.Fatal("fixed parts were not cached")
					}
				}
			}

			before := tx.queries
			hit := chain(true)
			if !reflect.DeepEqual(cold, warm) || !reflect.DeepEqual(cold, hit) || tx.queries != before {
				t.Fatalf("cached paging or tokens changed: cold=%d warm=%d hit=%d cold==warm=%t cold==hit=%t queries=%d before=%d hits=%d puts=%d", len(cold), len(warm), len(hit), reflect.DeepEqual(cold, warm), reflect.DeepEqual(cold, hit), tx.queries, before, cache.hits, cache.puts)
			}
			e.options.Sequences = cache
			// Invalid next must stop before consulting the populated sequence cache or database.
			hits := cache.hits
			bad, err := e.Read(context.Background(), ReadRequest{Release: e.release.Rel, Items: []ReadItem{{Next: "invalid"}}})
			if err != nil || bad.Items[0].Error != "invalid_next" || cache.hits != hits {
				t.Fatal("next was not decoded first")
			}
			if !item.Relations && item.Handle == "vn:1" && item.From == "" {
				now = now.Add(time.Second)
				changed := chain(true)
				if changed[0] == cold[0] || tx.queries != before {
					t.Fatal("status not recomputed on hit")
				}
			}
		})
	}
}

func TestReadCache_RelationWindowsStayBounded(t *testing.T) {
	e := &Engine{release: Release{Rel: "20990101", RetrievalRecipe: Recipe1()}, options: Options{Now: time.Now}}
	tx := &readCacheTx{relationCount: 45}
	c := cursor{V: 1, Release: e.release.Rel, Item: ReadItem{Handle: "vn:1", Relations: true}}
	cache := &memoryCache{}
	e.options.Sequences = cache
	first, _, save, err := e.readItem(context.Background(), tx, c, false, 1, true)
	if err != nil || first.Next == "" || save == nil {
		t.Fatal("first relation page did not defer", err)
	}
	save()
	c, err = decodeNext(first.Next)
	if err != nil {
		t.Fatal(err)
	}
	if c.Unit != 1 {
		t.Fatal("bad test boundary")
	}
	page := func(cached bool) string {
		t.Helper()
		e.options.Sequences = nil
		if cached {
			e.options.Sequences = cache
		}
		result, _, store, err := e.readItem(context.Background(), tx, c, true, ReadCallCap, true)
		if err != nil || result.Error != "" {
			t.Fatal(result.Error, err)
		}
		if store != nil {
			store()
		}
		raw, _ := json.Marshal(result)
		return string(raw)
	}
	cold := page(false)
	before := tx.queries
	if page(true) != cold || tx.queries-before != 5 {
		t.Fatal("different offset did not run the original bounded query")
	}
	before = tx.queries
	if page(true) != cold || tx.queries != before {
		t.Fatal("matching relation window missed or changed")
	}
}
