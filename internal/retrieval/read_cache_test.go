package retrieval

import (
	"context"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

type boundedSequenceCache struct {
	memoryCache
	limit int64
}

func (c *boundedSequenceCache) MaxValueBytes() int64 { return c.limit }

func TestReadCache_NoSerializationBeforeCommit(t *testing.T) {
	cache := &boundedSequenceCache{limit: 4_000_000}
	e := &Engine{release: Release{Rel: "20990101"}, options: Options{Sequences: cache}}
	seq := readSequence{units: []Unit{{Kind: "provision", Text: strings.Repeat("x", 2_000_000)}}, complete: true}
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	save := e.saveSequence(context.Background(), cursor{Item: ReadItem{Handle: "vn:1#p1"}}, seq)
	runtime.ReadMemStats(&after)
	if after.TotalAlloc-before.TotalAlloc > 64_000 {
		t.Fatal("sequence serialization allocated before commit")
	}
	if cache.puts != 0 || save == nil {
		t.Fatal("cache was stored before commit or save was lost")
	}
	save() // The caller invokes saves only after a successful commit.
	if cache.puts != 1 {
		t.Fatal("committed read was not stored")
	}
}

func TestReadCache_OversizedSequenceAllocatesNoBuffer(t *testing.T) {
	cache := &boundedSequenceCache{limit: 1024}
	e := &Engine{release: Release{Rel: "20990101"}, options: Options{Sequences: cache}}
	seq := readSequence{units: []Unit{{Kind: "provision", Text: strings.Repeat("x", 2_000_000)}}, complete: true}
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	save := e.saveSequence(context.Background(), cursor{Item: ReadItem{Handle: "vn:1#p1"}}, seq)
	if save != nil {
		save()
	}
	runtime.ReadMemStats(&after)
	if after.TotalAlloc-before.TotalAlloc > 64_000 {
		t.Fatal("oversized sequence allocated a serialization buffer")
	}
	if cache.puts != 0 {
		t.Fatal("oversized sequence was serialized and offered to cache")
	}
}

func TestReadCache_OversizedRangeMatchesUncachedUnderDeadline(t *testing.T) {
	e := &Engine{release: Release{Rel: "20990101", RetrievalRecipe: Recipe1()}, options: Options{Now: time.Now}}
	c := cursor{V: 1, Release: e.release.Rel, Item: ReadItem{Handle: "vn:1", From: "p1", To: "p1"}}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	cold, _, _, err := e.readItem(ctx, &readCacheTx{}, c, false, ReadCallCap, true)
	if err != nil {
		t.Fatal(err)
	}
	cache := &boundedSequenceCache{limit: 1024}
	e.options.Sequences = cache
	warm, _, save, err := e.readItem(ctx, &readCacheTx{}, c, false, ReadCallCap, true)
	if err != nil || !reflect.DeepEqual(cold, warm) {
		t.Fatal("cache changed bounded range", err)
	}
	if save != nil {
		save()
	}
	if cache.puts != 0 {
		t.Fatal("oversized range was offered to cache")
	}
	// A deadline reached after commit must also skip preparation of eligible data.
	ctx, cancel2 := context.WithCancel(context.Background())
	cancel2()
	save = e.saveSequence(ctx, c, readSequence{complete: true})
	if save != nil {
		save()
	}
	if cache.puts != 0 {
		t.Fatal("canceled request prepared a cache value")
	}
}
