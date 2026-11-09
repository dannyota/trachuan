package build

import "testing"

func TestExclusionCounts_DistinguishesReasons(t *testing.T) {
	inv := Inventory{ExpectedKeys: []string{"a", "b", "c", "d"}, Items: []InventoryItem{{Stage: "fetch", ItemKey: "a", State: "done"}, {Stage: "fetch", ItemKey: "b", State: "done"}, {Stage: "fetch", ItemKey: "c", State: "gap"}, {Stage: "fetch", ItemKey: "d", State: "done"}}, Gaps: []InventoryGap{{ItemKey: "a", Kind: "withdrawn"}, {ItemKey: "b", Kind: "scope_removed"}}}
	got := exclusionCounts(inv)
	if got["excluded_withdrawn"] != 1 || got["excluded_scope_removed"] != 1 || got["excluded_fetch_gap"] != 1 {
		t.Fatalf("counts: %v", got)
	}
}
