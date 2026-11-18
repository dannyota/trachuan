package build

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/detector"
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

func TestGaps_UnregisteredKindRefusesGapLoad(t *testing.T) {
	for _, gap := range []InventoryGap{
		{Stage: "extract", Kind: "future_structure_gap"},
		{Stage: "embed", Kind: ""},
	} {
		l := loader{gaps: []InventoryGap{gap}}
		err := l.loadGaps(context.Background())
		if !errors.Is(err, detector.ErrUnregisteredGap) {
			t.Errorf("gap %+v: err = %v", gap, err)
		}
	}
}
