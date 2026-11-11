package retrieval

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestReportLookups_ClosedPool(t *testing.T) {
	db, err := pgxpool.New(t.Context(), "postgres://localhost/unused")
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	e := &Engine{db: db}
	_, _, err = e.LookupDocument(t.Context(), "vn:1")
	if !errors.Is(err, ErrReleaseGone) {
		t.Fatal(err)
	}
	_, err = e.AnchorExists(t.Context(), "vn:1", "p11")
	if !errors.Is(err, ErrReleaseGone) {
		t.Fatal(err)
	}
	_, err = e.HasNumber(t.Context(), "196-HDBT")
	if !errors.Is(err, ErrReleaseGone) {
		t.Fatal(err)
	}
}

func TestNumberSegments_CachedAndImmutable(t *testing.T) {
	e := &Engine{numberSegments: map[string]struct{}{"TT": {}, "NHNN": {}, "50": {}, "2024": {}}}
	set, err := e.NumberSegments(t.Context())
	if err != nil || len(set) != 4 {
		t.Fatalf("%v %v", set, err)
	}
	delete(set, "TT")
	set, err = e.NumberSegments(t.Context())
	if _, ok := set["TT"]; !ok || err != nil {
		t.Fatal("cache was mutated")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err = e.NumberSegments(ctx); err == nil {
		t.Fatal("canceled lookup succeeded")
	}
}
