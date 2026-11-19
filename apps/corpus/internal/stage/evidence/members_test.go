package evidence

import (
	"reflect"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/stage/build"
	"danny.vn/trachuan/apps/corpus/internal/stage/membership"
)

func memberInventory() build.Inventory {
	replacement := "vn:vbpl:2099004"
	entries := []membership.Entry{
		{DocumentKey: "vn:vbpl:2099001", Disposition: "active"},
		{DocumentKey: "vn:vbpl:2099002", Disposition: "withdrawn"},
		{DocumentKey: "vn:vbpl:2099005", Disposition: "scope_removed"},
		{DocumentKey: "vn:chinhphu:2099003", Disposition: "source_replaced", ReplacementKey: &replacement},
		{DocumentKey: "vn:chinhphu:2099008", Disposition: "source_replaced"},
	}
	return build.Inventory{Version: 2, Country: "vn", FinalMembershipHash: strings.Repeat("f", 64), Membership: membership.PhaseSnapshot{Entries: entries}}
}

func TestMembersFrom_ReadsDispositionsAndHandOvers(t *testing.T) {
	inv := memberInventory()
	m, err := membersFrom(inv, inv.IdentityHash())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(m.active, map[string]bool{"vn:vbpl:2099001": true}) {
		t.Fatalf("active %v", m.active)
	}
	if m.disposition["vn:vbpl:2099002"] != "withdrawn" || m.disposition["vn:chinhphu:2099008"] != "source_replaced" || len(m.disposition) != 5 {
		t.Fatalf("dispositions %v", m.disposition)
	}
	if !reflect.DeepEqual(m.replacement, map[string]string{"vn:chinhphu:2099003": "vn:vbpl:2099004"}) {
		t.Fatalf("replacements %v", m.replacement)
	}
}

func TestMembersFrom_RefusesUnboundOrLegacyInventory(t *testing.T) {
	inv := memberInventory()
	if _, err := membersFrom(inv, strings.Repeat("0", 64)); err == nil || !strings.Contains(err.Error(), "differs from the build") {
		t.Fatalf("other build: %v", err)
	}
	legacy := inv
	legacy.Version = 0
	if _, err := membersFrom(legacy, legacy.IdentityHash()); err == nil || !strings.Contains(err.Error(), "membership") {
		t.Fatalf("legacy: %v", err)
	}
	changed := memberInventory()
	changed.FinalMembershipHash = strings.Repeat("e", 64)
	if _, err := membersFrom(changed, inv.IdentityHash()); err == nil {
		t.Fatal("membership changed after build accepted")
	}
}
