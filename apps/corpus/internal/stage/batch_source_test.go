package stage

import (
	"errors"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/sourcegate"
)

func TestBatchRetry_SourceRule(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  *RetryableError
		want bool
	}{
		{"own error", &RetryableError{Err: errors.New("disk")}, true},
		{"429", &RetryableError{GapKind: "source_unavailable", Cause: GapCause{Cause: CauseHTTP429}}, true},
		{"pause", &RetryableError{Err: sourcegate.ErrSourcePaused}, true},
		{"timeout", &RetryableError{GapKind: "source_unavailable", Cause: GapCause{Cause: CauseTimeout}}, false},
		{"transport", &RetryableError{GapKind: "source_unavailable", Cause: GapCause{Cause: CauseConnectionError}}, false},
		{"5xx", &RetryableError{GapKind: "source_unavailable", Cause: GapCause{Cause: CauseHTTP5xx}}, false},
		{"non JSON", &RetryableError{GapKind: "source_unavailable", Cause: GapCause{Cause: CauseNonJSON200}}, false},
		{"unexpected", &RetryableError{GapKind: "source_unavailable", Cause: GapCause{Cause: CauseUnknown}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := retryBatch(tc.err); got != tc.want {
				t.Fatalf("retry=%v want=%v", got, tc.want)
			}
		})
	}
}
