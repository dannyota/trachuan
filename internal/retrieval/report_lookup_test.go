package retrieval

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type rollbackContextKey struct{}

type stalledReportRollback struct {
	observed chan error
}

func (s stalledReportRollback) Rollback(ctx context.Context) error {
	if ctx.Err() != nil || ctx.Value(rollbackContextKey{}) != "kept" {
		s.observed <- errors.New("rollback context was canceled or lost its values")
		return nil
	}
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > 20*time.Millisecond {
		s.observed <- errors.New("rollback deadline missing or too long")
		return nil
	}
	<-ctx.Done()
	s.observed <- ctx.Err()
	return ctx.Err()
}

func TestReportRollback_StallIsBoundedAfterCancellation(t *testing.T) {
	ctx := context.WithValue(t.Context(), rollbackContextKey{}, "kept")
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	observed := make(chan error, 1)
	rollbackReport(ctx, stalledReportRollback{observed: observed}, 10*time.Millisecond)
	if err := <-observed; !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("rollback context: %v", err)
	}
}

type failingMetadataTx struct {
	pgx.Tx
	stalledReportRollback
}

func (f failingMetadataTx) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, errors.New("PRIVATE")
}

func (f failingMetadataTx) Rollback(ctx context.Context) error {
	return f.stalledReportRollback.Rollback(ctx)
}

func TestMetadataTx_SetupFailureBoundsRollback(t *testing.T) {
	ctx := context.WithValue(t.Context(), rollbackContextKey{}, "kept")
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	observed := make(chan error, 1)
	err := configureMetadataTx(ctx, failingMetadataTx{stalledReportRollback: stalledReportRollback{observed: observed}}, 2*time.Second, 10*time.Millisecond)
	if err == nil || strings.Contains(err.Error(), "PRIVATE") {
		t.Fatal("setup failure was not sanitized", err)
	}
	if got := <-observed; !errors.Is(got, context.DeadlineExceeded) {
		t.Fatalf("rollback context: %v", got)
	}
}

func TestReportSegment_MembershipWithoutAllocation(t *testing.T) {
	e := &Engine{numberSegments: map[string]struct{}{"TT": {}, "NHNN": {}}}
	ctx := t.Context()
	for segment, want := range map[string]bool{"TT": true, "NHNN": true, "UNKNOWN": false, "": false} {
		found, err := e.HasNumberSegment(ctx, segment)
		if err != nil || found != want {
			t.Fatalf("%q: got %v, err %v", segment, found, err)
		}
	}
	if allocations := testing.AllocsPerRun(100, func() {
		if found, err := e.HasNumberSegment(ctx, "TT"); !found || err != nil {
			t.Fatal("cached membership failed")
		}
	}); allocations != 0 {
		t.Fatalf("membership allocated %v times", allocations)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := e.HasNumberSegment(canceled, "TT"); err == nil {
		t.Fatal("canceled lookup succeeded")
	}
}

func TestReportSegments_BoundsAndOneLimitEvent(t *testing.T) {
	segments := make(map[string]struct{})
	events := 0
	limit := func() { events++ }
	exact := strings.Repeat("Đ", 16)
	tooLong := exact + "A"
	addReportSegment(segments, exact, limit)
	addReportSegment(segments, tooLong, limit)
	addReportSegment(segments, "", limit)
	if _, ok := segments[exact]; !ok || len(segments) != 1 {
		t.Fatal("32-byte limit did not filter segments")
	}
	for i := 0; i < 49_998; i++ {
		addReportSegment(segments, fmt.Sprintf("S%05d", i), limit)
	}
	addReportSegment(segments, "S00000", limit)
	if len(segments) != 49_999 || events != 0 {
		t.Fatalf("before limit: count=%d events=%d", len(segments), events)
	}
	addReportSegment(segments, "LAST", limit)
	addReportSegment(segments, "EXTRA", limit)
	addReportSegment(segments, "ANOTHER", limit)
	if len(segments) != 50_000 || events != 1 {
		t.Fatalf("at limit: count=%d events=%d", len(segments), events)
	}
	if _, ok := segments["EXTRA"]; ok {
		t.Fatal("segment admitted beyond budget")
	}
}

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
	for _, segment := range []string{"TT", "NHNN", "50", "2024"} {
		if found, err := e.HasNumberSegment(t.Context(), segment); !found || err != nil {
			t.Fatalf("missing cached segment %q: %v", segment, err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := e.HasNumberSegment(ctx, "TT"); err == nil {
		t.Fatal("canceled lookup succeeded")
	}
}
