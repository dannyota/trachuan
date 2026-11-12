package stage

import "testing"

func TestRepeatedSourceTimeout(t *testing.T) {
	for _, tc := range []struct {
		name    string
		attempt int
		cause   Cause
		want    bool
	}{
		{"fetch", 1, CauseTimeout, false},
		{"fetch", 2, CauseTimeout, true},
		{"expand", 2, CauseTimeout, true},
		{"fetch", 2, CauseHTTP429, false},
		{"discover", 2, CauseTimeout, false},
	} {
		err := &RetryableError{GapKind: "source_unavailable", Cause: GapCause{Cause: tc.cause}}
		if got := repeatedSourceTimeout(tc.name, tc.attempt, err); got != tc.want {
			t.Errorf("stage %s attempt %d cause %s: got %v, want %v", tc.name, tc.attempt, tc.cause, got, tc.want)
		}
	}
}
