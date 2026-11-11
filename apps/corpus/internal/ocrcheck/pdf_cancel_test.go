package ocrcheck

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCheck_PageCounterCancelsExpiredDeadline(t *testing.T) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	_, err := recoverCount(ctx, countPages, []byte("untrusted PDF input"))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expired parse was not canceled: %v", err)
	}
}
func TestCheck_PDFParseReceivesTenSecondDeadline(t *testing.T) {
	r, _, _, docs := fixture(t)
	r.CountPages = func(ctx context.Context, _ []byte) (int, error) {
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > 10*time.Second || time.Until(deadline) < 9*time.Second {
			t.Fatal("page parser did not receive the ten-second limit")
		}
		return 0, context.DeadlineExceeded
	}
	_, reason := r.prepare(context.Background(), docs[0], options())
	if reason != "page_count_unknown" {
		t.Fatalf("timed-out parse reason=%s", reason)
	}
}
