package build

import (
	"reflect"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/stage/membership"
)

func TestGaps_DispositionNotDuplicatedByUpstreamGap(t *testing.T) {
	upstream := []InventoryGap{
		{Stage: "fetch", ItemKey: "replaced", Kind: "source_replaced", Detail: "evidence"},
		{Stage: "fetch", ItemKey: "withdrawn", Kind: "other", Detail: "other gap"},
		{Stage: "fetch", ItemKey: "another", Kind: "scope_removed"},
	}
	l := loader{inv: Inventory{Version: 2, Membership: membership.PhaseSnapshot{Entries: []membership.Entry{
		{DocumentKey: "replaced", Disposition: "source_replaced"},
		{DocumentKey: "withdrawn", Disposition: "withdrawn"},
		{DocumentKey: "removed", Disposition: "scope_removed"},
		{DocumentKey: "present", Disposition: "active"},
	}}}, members: map[string]membership.Entry{}, gaps: append([]InventoryGap{}, upstream...)}
	l.addMembershipGaps()
	want := append(upstream, InventoryGap{Stage: "build", ItemKey: "withdrawn", Kind: "withdrawn"}, InventoryGap{Stage: "build", ItemKey: "removed", Kind: "scope_removed"})
	if !reflect.DeepEqual(l.gaps, want) {
		t.Fatalf("gaps=%+v, want %+v", l.gaps, want)
	}
}
