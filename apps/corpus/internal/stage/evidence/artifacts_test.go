package evidence

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/store"
)

func TestProcess_WiresArtifactDirToTheRunInventory(t *testing.T) {
	f := newFixture(t, noneDecision())
	var seen Inputs
	f.stage.cfg.Detectors = []Detector{fakeDetector{id: "cross", seen: &seen, observation: Observation{ObservedCompletely: always}}}
	if _, _, err := f.process(); err != nil {
		t.Fatal(err)
	}
	if seen.ArtifactDir == nil {
		t.Fatal("detectors get no artifact lookup")
	}
	dir, ok, err := seen.ArtifactDir("build", "vn/"+testRel)
	if err != nil || !ok || dir != f.env.Artifacts.Abs(f.build.Path) {
		t.Fatalf("build dir %q %v %v", dir, ok, err)
	}
	if _, ok, err = seen.ArtifactDir("extract", "vn:missing"); err != nil || ok {
		t.Fatalf("missing item found: %v %v", ok, err)
	}
}

type countingQueries struct {
	store.Querier
	inner *fakeQueries
	calls atomic.Int32
}

func (c *countingQueries) GetBuildInventory(ctx context.Context, id int64) (json.RawMessage, error) {
	c.calls.Add(1)
	return c.inner.GetBuildInventory(ctx, id)
}

func TestLazyArtifactDirs_ReadsInventoryOnceAndOnDemand(t *testing.T) {
	f := newFixture(t, noneDecision())
	q := &countingQueries{inner: f.queries}
	dirs := lazyArtifactDirs(context.Background(), func() store.Querier { return q }, f.env, f.run)
	if q.calls.Load() != 0 {
		t.Fatal("inventory read before the first lookup")
	}
	for range 3 {
		if _, ok, err := dirs("build", "vn/"+testRel); err != nil || !ok {
			t.Fatalf("lookup %v %v", ok, err)
		}
	}
	if q.calls.Load() != 1 {
		t.Fatalf("inventory read %d times", q.calls.Load())
	}
}

func TestLazyArtifactDirs_ReportsInventoryFailure(t *testing.T) {
	f := newFixture(t, noneDecision())
	f.queries.failure = errors.New("workshop unavailable")
	dirs := lazyArtifactDirs(context.Background(), func() store.Querier { return f.queries }, f.env, f.run)
	if _, ok, err := dirs("build", "vn/"+testRel); err == nil || ok {
		t.Fatalf("failure hidden: %v %v", ok, err)
	}
}

func TestCountInputs_ResolvesDirsFromTheRunInventory(t *testing.T) {
	f := newFixture(t, noneDecision())
	in := CountInputs(context.Background(), f.queries, f.env, f.run, nil)
	dir, ok, err := in.ArtifactDir("build", "vn/"+testRel)
	if err != nil || !ok || dir != f.env.Artifacts.Abs(f.build.Path) || in.Run.ID != f.run.ID {
		t.Fatalf("%q %v %v %+v", dir, ok, err, in.Run)
	}
}
