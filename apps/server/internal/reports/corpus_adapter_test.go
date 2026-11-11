package reports

import (
	"context"
	"errors"
	"testing"

	"danny.vn/trachuan/apps/server/internal/release"
	"danny.vn/trachuan/internal/retrieval"
)

type activeReportRelease struct{}

func (activeReportRelease) GetActiveRelease(context.Context, string) (string, error) {
	return "corpus_vn_20991111", nil
}

func TestReleaseResolver_PinsLease(t *testing.T) {
	closed := 0
	r := &release.Resolver{Auth: activeReportRelease{}, Clock: testNow, Open: func(context.Context, string) (*retrieval.Engine, func(), error) {
		return &retrieval.Engine{}, func() { closed++ }, nil
	}}
	adapter := ReleaseResolver{Resolver: r}
	if _, err := adapter.Resolve(t.Context(), "vn", "20991110"); !errors.Is(err, release.ErrExpired) {
		t.Fatal(err)
	}
	lease, err := adapter.Resolve(t.Context(), "vn", "")
	if err != nil || lease.Release != "20991111" || lease.Corpus == nil || lease.Done == nil {
		t.Fatalf("%+v %v", lease, err)
	}
	r.Close()
	if closed != 0 {
		t.Fatal("released engine during validation")
	}
	lease.Done()
	lease.Done()
	if closed != 1 {
		t.Fatal("lease not forwarded")
	}
}

func TestEngineCorpus_ForwardsCachedLookup(t *testing.T) {
	// Zero engines have no dependencies; use cancellation to exercise the cached
	// lookup without opening PostgreSQL.
	c := EngineCorpus{Engine: &retrieval.Engine{}}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := c.HasNumberSegment(ctx, "TT"); err == nil {
		t.Fatal("cancellation lost")
	}
}
