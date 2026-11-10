package stage

import (
	"fmt"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/sourcegate"
)

func TestProcessFailure_SourcePauseOverridesGap(t *testing.T) {
	err := RetryableGap(fmt.Errorf("challenge: %w", sourcegate.ErrSourcePaused), "source_unavailable")
	retryable := err.(*RetryableError)
	out := processFailure(err, retryable)
	if out.State != "failed" || out.Message != "source_paused" || len(out.Gaps) != 0 {
		t.Fatalf("output = %+v", out)
	}
}
