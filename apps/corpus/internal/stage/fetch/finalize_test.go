package fetch

import (
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/stage/expand"
	"danny.vn/trachuan/apps/corpus/internal/stage/membership"
)

func TestFinalize_NewTargetIsScopeChanged(t *testing.T) {
	s := membership.PhaseSnapshot{Active: []membership.Entry{{DocumentKey: "vn:vbpl:1"}}}
	got := findDrift("vn:vbpl:1", expand.Observation{Targets: []expand.Target{{TargetKey: "vn:vbpl:2", RawCode: "A"}}}, s, "expandhash", "fetchhash")
	if len(got) != 1 || got[0].reason != "absent" || got[0].target != "vn:vbpl:2" {
		t.Fatalf("drift = %#v", got)
	}
}
func TestFinalize_RetainedRemovalKeyIsDrift(t *testing.T) {
	s := membership.PhaseSnapshot{RetainedRemovals: []membership.Entry{{DocumentKey: "vn:vbpl:2", Disposition: "withdrawn"}}}
	got := findDrift("vn:vbpl:1", expand.Observation{Targets: []expand.Target{{TargetKey: "vn:vbpl:2", RawCode: "A"}}}, s, "none", "fetchhash")
	if len(got) != 1 || got[0].reason != "retained_removal" {
		t.Fatalf("drift = %#v", got)
	}
}

func TestFinalize_DeduplicatesDriftAcrossMethods(t *testing.T) {
	obs := expand.Observation{Targets: []expand.Target{{TargetKey: "vn:vbpl:2", RawCode: "A", Method: "vbpl_references"}, {TargetKey: "vn:vbpl:2", RawCode: "A", Method: "vbpl_diagram"}}}
	got := findDrift("vn:vbpl:1", obs, membership.PhaseSnapshot{}, "old", "new")
	if len(got) != 1 {
		t.Fatalf("drifts = %#v", got)
	}
}
