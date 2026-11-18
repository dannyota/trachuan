package mcpapi

import (
	"context"
	"testing"
	"time"
)

func TestTaxonomyNodeCache_VietnamDate(t *testing.T) {
	e := &nodeCacheEngine{cachedEngine: &cachedEngine{release: "20990101", generation: 1}}
	now := time.Date(2099, time.January, 1, 16, 59, 58, 0, time.UTC)
	clock := func() time.Time { return now }
	c := NewCaches(clock)
	c.ActivateRelease(e.release, e.generation)
	o := NewOperations(Config{Resolver: nodeCacheResolver{e}, Caches: c, Clock: clock})
	for _, step := range []struct {
		advance time.Duration
		calls   int64
	}{{0, 1}, {time.Second, 1}, {time.Second, 2}, {time.Hour, 2}} {
		now = now.Add(step.advance)
		if _, err := o.TaxonomyNode(context.Background(), "1"); err != nil {
			t.Fatal(err)
		}
		if got := e.calls.Load(); got != step.calls {
			t.Fatalf("at %s: calls %d, want %d", now, got, step.calls)
		}
	}
}
