package release

import (
	"context"
	"errors"
	"testing"

	"danny.vn/trachuan/internal/retrieval"
)

func TestResolver_RollbackClosesOnlyRetiredGeneration(t *testing.T) {
	r, s, _ := resolverFixture(t)
	type pool struct{ closed bool }
	latest := make(map[string]*pool)
	r.Open = func(_ context.Context, name string) (*retrieval.Engine, func(), error) {
		opened := &pool{}
		latest[name] = opened
		return &retrieval.Engine{}, func() { opened.closed = true }, nil
	}
	first := takeLease(t, r, "")
	oldPool := latest["corpus_vn_20990103"]
	s.active = "corpus_vn_20990104"
	takeLease(t, r, "").Done()
	s.active = "corpus_vn_20990103"
	current := takeLease(t, r, "")
	currentPool := latest["corpus_vn_20990103"]
	first.Done()
	if !oldPool.closed || currentPool.closed {
		t.Fatalf("retired closed=%t current closed=%t", oldPool.closed, currentPool.closed)
	}
	r.Close()
	if currentPool.closed {
		t.Fatal("closed current generation before its lease ended")
	}
	current.Done()
	if !currentPool.closed {
		t.Fatal("current generation did not close after Done")
	}
}

func TestResolver_FailedOpenClosesItsPoolOnce(t *testing.T) {
	r, _, _ := resolverFixture(t)
	failure := errors.New("open failed")
	closed := 0
	r.Open = func(context.Context, string) (*retrieval.Engine, func(), error) {
		return nil, func() { closed++ }, failure
	}
	if _, err := r.Resolve(context.Background(), "vn", ""); !errors.Is(err, failure) {
		t.Fatalf("open error=%v", err)
	}
	r.Close()
	if closed != 1 {
		t.Fatalf("pool closed %d times", closed)
	}
}
