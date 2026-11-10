package discover

import (
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/stage/membership"
)

func TestSeed_SameRuleSupersedesOldEvidence(t *testing.T) {
	old := membership.SupportTuple{RuleKind: "agency", RuleValue: "62", Evidence: membership.EvidenceRef{ArtifactHash: "old"}}
	records := []membership.EvidenceRecord{{State: "current", Support: membership.SupportTuple{RuleKind: "agency", RuleValue: "62", Evidence: membership.EvidenceRef{ArtifactHash: "new"}}}}
	if !superseded(old, records) {
		t.Fatal("fresh evidence did not supersede prior rule")
	}
	old.RuleValue = "908"
	if superseded(old, records) {
		t.Fatal("different rule was superseded")
	}
}
