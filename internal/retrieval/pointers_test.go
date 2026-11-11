package retrieval

import "testing"

func TestPointerKindRank_Order(t *testing.T) {
	kinds := []string{"repeals", "replaces", "suspends", "amends_supplements", "consolidates", "implements", "guided_by", "legal_basis", "cites", "references", "unknown"}
	for i, kind := range kinds {
		if got := pointerKindRank(kind); got != i {
			t.Errorf("rank of %s = %d, want %d", kind, got, i)
		}
	}
	if got := pointerKindRank("unrecognized"); got < pointerKindRank("unknown") {
		t.Errorf("unrecognized rank %d sorts before unknown", got)
	}
}
