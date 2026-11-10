package build

import "testing"

func TestExcluded_WithdrawalAndScopeRemoval(t *testing.T) {
	l := loader{gaps: []InventoryGap{{Stage: "fetch", ItemKey: "a", Kind: "withdrawn"}, {Stage: "fetch", ItemKey: "b", Kind: "scope_removed"}, {Stage: "extract", ItemKey: "c", Kind: "no_tree"}}}
	if !l.excluded("a") || !l.excluded("b") || l.excluded("c") {
		t.Fatal("invalid exclusion")
	}
}
