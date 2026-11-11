package checkpoint

import (
	"context"
	"errors"
	"testing"
)

type fakeSnapshot struct {
	active    bool
	reference func(context.Context, string, string) (bool, error)
}

func snapshotWithReference(reference func(context.Context, string, string) (bool, error)) func(context.Context, string) (workshopSnapshot, error) {
	return func(context.Context, string) (workshopSnapshot, error) {
		return &fakeSnapshot{active: true, reference: reference}, nil
	}
}

func (*fakeSnapshot) snapshotID() string { return "unit-snapshot" }
func (s *fakeSnapshot) artifactReferenced(ctx context.Context, root, rel string) (bool, error) {
	if !s.active {
		return false, errors.New("snapshot closed before reference check")
	}
	if s.reference == nil {
		return false, nil
	}
	return s.reference(ctx, root, rel)
}
func (s *fakeSnapshot) close(context.Context) { s.active = false }

type observingDumper struct {
	dump func(context.Context, string, string) error
}

func (d observingDumper) Dump(ctx context.Context, _, path, snapshot string) error {
	return d.dump(ctx, path, snapshot)
}

func TestTake_HoldsSnapshotThroughDumpAndReferenceCheck(t *testing.T) {
	for _, mode := range []string{"required artifact", "orphan", "dump fails", "begin fails", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			taker := setup(t)
			corruptArtifact(t, taker)
			snapshot := &fakeSnapshot{active: true}
			checked, dumped := false, false
			snapshot.reference = func(context.Context, string, string) (bool, error) { checked = true; return mode != "orphan", nil }
			taker.beginSnapshot = func(context.Context, string) (workshopSnapshot, error) {
				if mode == "begin fails" {
					return nil, errors.New("snapshot unavailable")
				}
				return snapshot, nil
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			taker.Dumper = observingDumper{dump: func(ctx context.Context, path, id string) error {
				dumped = true
				if !snapshot.active || id != snapshot.snapshotID() {
					t.Fatalf("dump snapshot=%s active=%v", id, snapshot.active)
				}
				if mode == "dump fails" {
					return errors.New("dump failed")
				}
				if mode == "cancelled" {
					cancel()
					return ctx.Err()
				}
				return fakeDumper{}.Dump(ctx, "", path, id)
			}}
			if _, err := taker.Take(ctx, "run", 1, "fetch"); (err == nil) != (mode == "orphan") {
				t.Fatalf("checkpoint error=%v", err)
			}
			if mode == "begin fails" {
				if dumped {
					t.Fatal("dump ran without a snapshot")
				}
				return
			}
			if snapshot.active {
				t.Fatal("snapshot not released after checkpoint")
			}
			if checked != (mode == "required artifact" || mode == "orphan") {
				t.Fatalf("reference checked=%v", checked)
			}
		})
	}
}
