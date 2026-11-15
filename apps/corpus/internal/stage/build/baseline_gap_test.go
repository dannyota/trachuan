package build

import (
	"testing"
	"time"
)

func TestBaselineGap_UnknownDate(t *testing.T) {
	l := loader{items: map[string]map[string]InventoryItem{"fetch": {"vn:vbpl:1": {Stage: "fetch", ItemKey: "vn:vbpl:1", State: "done"}}}, gaps: []InventoryGap{{Stage: "fetch", ItemKey: "vn:vbpl:1", Kind: "source_unavailable"}}}
	l.addMembershipGaps()
	if len(l.gaps) != 2 || l.gaps[1].Kind != "baseline_served" || l.gaps[1].Detail != "unknown" {
		t.Fatalf("gaps=%+v", l.gaps)
	}
}

func TestBaselineGap_UsesUTCDate(t *testing.T) {
	at := time.Date(2099, 1, 2, 1, 0, 0, 0, time.FixedZone("offset", 7*60*60))
	l := loader{inv: Inventory{Provenance: provenance{DetailTimes: []detailTime{{DocumentKey: "vn:vbpl:1", FetchedAt: &at}}}}, items: map[string]map[string]InventoryItem{"fetch": {"vn:vbpl:1": {State: "done"}}}, gaps: []InventoryGap{{Stage: "fetch", ItemKey: "vn:vbpl:1", Kind: "source_unavailable"}}}
	l.addMembershipGaps()
	if len(l.gaps) != 2 || l.gaps[1].Detail != "2099-01-01" {
		t.Fatalf("gaps=%+v", l.gaps)
	}
}

func TestBaselineGap_DateChangesLegacyBuildInput(t *testing.T) {
	at := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	inv := Inventory{Provenance: provenance{DetailTimes: []detailTime{{DocumentKey: "vn:vbpl:1", FetchedAt: &at}}}}
	before := inventoryInputHash("vn", "20990101_1", inv, "identity")
	later := at.AddDate(0, 0, 1)
	inv.Provenance.DetailTimes[0].FetchedAt = &later
	if after := inventoryInputHash("vn", "20990101_1", inv, "identity"); after == before {
		t.Fatal("date did not change build input")
	}
}
