package build

import (
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/stage/membership"
)

func TestPresence_FinalMembershipAndFetch(t *testing.T) {
	inv := Inventory{Version: 2, Membership: membership.PhaseSnapshot{Entries: []membership.Entry{{DocumentKey: "active", Disposition: "active"}, {DocumentKey: "replaced", Disposition: "source_replaced"}, {DocumentKey: "gap", Disposition: "active"}}}, Items: []InventoryItem{{Stage: "fetch", ItemKey: "active", State: "done"}, {Stage: "fetch", ItemKey: "replaced", State: "done"}, {Stage: "fetch", ItemKey: "gap", State: "gap"}}}
	for _, tc := range []struct {
		key  string
		want bool
	}{{"active", true}, {"replaced", false}, {"gap", false}, {"missing", false}} {
		if got := inv.Presence()[tc.key]; got != tc.want {
			t.Errorf("Present(%q) = %v, want %v", tc.key, got, tc.want)
		}
	}
}

func TestReconcileInventory_ChecksAbsentArtifacts(t *testing.T) {
	l := loader{present: map[string]bool{}, items: map[string]map[string]InventoryItem{
		"chunk": {"absent": {State: "done"}}, "embed": {"absent": {State: "done"}},
	}}
	if err := l.reconcileInventory(); err == nil {
		t.Fatal("missing absent artifacts were not checked")
	}
}
