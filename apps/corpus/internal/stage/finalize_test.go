package stage

import (
	"slices"
	"testing"
)

func TestBlockingFailureCodes_SecondaryCandidateCeiling(t *testing.T) {
	if !slices.Contains(BlockingFailureCodes(), "secondary_candidate_ceiling") {
		t.Fatal("candidate ceiling failure is acceptable")
	}
}

func TestBlockingFailureCodes_ExpansionContextCeiling(t *testing.T) {
	if !slices.Contains(BlockingFailureCodes(), "expansion_context_ceiling") {
		t.Fatal("context ceiling must block release")
	}
}
