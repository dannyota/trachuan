package cli

import (
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/evaluation"
)

func TestEvalCLI_ExitCodes(t *testing.T) {
	for _, tc := range []struct {
		status  string
		require bool
		want    int
	}{{"pass", true, 0}, {"fail", true, 2}, {"unavailable", true, 2}, {"fail", false, 0}} {
		r := evaluation.Report{Status: "complete", Floors: make([]evaluation.Floor, 8)}
		for i := range r.Floors {
			r.Floors[i].Status = tc.status
		}
		if got := evalExit(r, tc.require); got != tc.want {
			t.Fatalf("%s %v: %d", tc.status, tc.require, got)
		}
	}
	if got := evalExit(evaluation.Report{Status: "incomplete"}, true); got != 1 {
		t.Fatalf("incomplete: %d", got)
	}
}
