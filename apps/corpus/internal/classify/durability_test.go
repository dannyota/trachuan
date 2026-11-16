package classify

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type faultLedger struct {
	*memoryLedger
	failSubmitted bool
}

func (m *faultLedger) Save(ctx context.Context, s Submission) error {
	if m.failSubmitted && s.State == "submitted" {
		return errors.New("synthetic database interruption")
	}
	return m.memoryLedger.Save(ctx, s)
}

func TestScreen_PreservesReceiptWhenDatabaseAttachFails(t *testing.T) {
	r, m, f, o := screenFixture(t)
	fault := &faultLedger{memoryLedger: m, failSubmitted: true}
	r.Repository = fault
	if _, err := r.Screen(context.Background(), o, documents(), families()); err == nil {
		t.Fatal("expected interrupted database write")
	}
	if _, err := os.Stat(filepath.Join(r.IntentDir, "submissions", m.rows[0].ID+".batch")); err != nil {
		t.Fatal("provider identity was lost", err)
	}
	fault.failSubmitted = false
	if _, err := r.Screen(context.Background(), o, documents(), families()); err != nil {
		t.Fatal("did not resume durable receipt", err)
	}
	if f.creates != 1 || m.rows[0].State != "completed" {
		t.Fatal("recreated known batch")
	}
}

func TestScreen_ResumeDryRunReportsCountedInput(t *testing.T) {
	r, _, f, o := screenFixture(t)
	f.ended = false
	if _, err := r.Screen(context.Background(), o, documents(), families()); err == nil {
		t.Fatal("expected interrupted polling")
	}
	o.DryRun = true
	s, err := r.Screen(context.Background(), o, documents(), families())
	if err != nil || s.InputTokens != 100 || f.creates != 1 {
		t.Fatalf("summary %+v error %v", s, err)
	}
}

type roundedLedger struct{ *memoryLedger }

func (m roundedLedger) Save(ctx context.Context, s Submission) error {
	s.CreatedAt = s.CreatedAt.Round(time.Microsecond)
	if s.SubmittedAt != nil {
		v := s.SubmittedAt.Round(time.Microsecond)
		s.SubmittedAt = &v
	}
	if s.DeadlineAt != nil {
		v := s.DeadlineAt.Round(time.Microsecond)
		s.DeadlineAt = &v
	}
	return m.memoryLedger.Save(ctx, s)
}

func TestScreen_ReceiptSurvivesPostgresTimestampPrecision(t *testing.T) {
	r, m, f, o := screenFixture(t)
	r.Repository = roundedLedger{m}
	r.Now = func() time.Time { return time.Date(2099, 1, 1, 0, 0, 0, 123456789, time.UTC) }
	f.ended = false
	if _, err := r.Screen(context.Background(), o, documents(), families()); err == nil {
		t.Fatal("expected interrupted polling")
	}
	f.ended = true
	if _, err := r.Screen(context.Background(), o, documents(), families()); err != nil {
		t.Fatal("timestamp precision broke receipt replay", err)
	}
	if f.creates != 1 || m.rows[0].State != "completed" {
		t.Fatal("did not resume original batch")
	}
}
