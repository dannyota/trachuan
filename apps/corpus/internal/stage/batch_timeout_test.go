package stage

import (
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/store"
)

func TestRepeatedSourceTimeout(t *testing.T) {
	for _, tc := range []struct {
		name                                                     string
		generation, previousGeneration, attempt, previousAttempt int
		cause, previous                                          Cause
		want                                                     bool
	}{
		{"fetch", 2, 2, 2, 1, CauseTimeout, CauseHTTP429, false},
		{"fetch", 2, 2, 2, 1, CauseTimeout, CauseTimeout, true},
		{"expand", 2, 2, 2, 1, CauseTimeout, CauseTimeout, true},
		{"fetch", 2, 2, 3, 1, CauseTimeout, CauseTimeout, false},
		{"fetch", 3, 2, 2, 1, CauseTimeout, CauseTimeout, false},
		{"fetch", 2, 2, 2, 1, CauseHTTP429, CauseTimeout, false},
		{"discover", 2, 2, 2, 1, CauseTimeout, CauseTimeout, false},
	} {
		message := `retry:{"cause":"` + string(tc.previous) + `","endpoint":"doc/diagram"}`
		row := store.GetRunItemCommittedRow{State: "pending", Generation: int32(tc.previousGeneration), Attempt: int32(tc.previousAttempt), Message: &message}
		err := &RetryableError{GapKind: "source_unavailable", Cause: GapCause{Cause: tc.cause}}
		if got := repeatedSourceTimeout(tc.name, tc.generation, tc.attempt, row, err); got != tc.want {
			t.Errorf("stage %s attempt %d previous %d cause %s previous cause %s: got %v, want %v", tc.name, tc.attempt, tc.previousAttempt, tc.cause, tc.previous, got, tc.want)
		}
	}
}
