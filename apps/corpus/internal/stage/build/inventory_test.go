package build

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/membership"
	"danny.vn/trachuan/apps/corpus/internal/store"
)

func TestInventory_SortedAndHashStable(t *testing.T) {
	a := Inventory{Country: "vn", SourceListHash: "source", ExpectedKeys: []string{"b", "a"}, Items: []InventoryItem{{Stage: "extract", ItemKey: "b"}, {Stage: "fetch", ItemKey: "a"}}}
	b := Inventory{Country: "vn", SourceListHash: "source", ExpectedKeys: []string{"a", "b"}, Items: []InventoryItem{{Stage: "fetch", ItemKey: "a"}, {Stage: "extract", ItemKey: "b"}}}
	if a.SourceManifestHash() != b.SourceManifestHash() || a.IdentityHash() != b.IdentityHash() {
		t.Fatal("inventory order changed hash")
	}
}

func TestInventory_IdentityHashIgnoresChunkAndEmbed(t *testing.T) {
	a := Inventory{Country: "vn", Items: []InventoryItem{{Stage: "fetch", ItemKey: "a"}}}
	b := a
	b.Items = append(append([]InventoryItem{}, a.Items...), InventoryItem{Stage: "chunk", ItemKey: "a"}, InventoryItem{Stage: "embed", ItemKey: "a"})
	if a.IdentityHash() != b.IdentityHash() {
		t.Fatal("index stages changed identity hash")
	}
	if a.SourceManifestHash() == b.SourceManifestHash() {
		t.Fatal("index stages did not change source hash")
	}
}

func TestInventory_AcceptanceNotHashed(t *testing.T) {
	a := Inventory{Country: "vn", Gaps: []InventoryGap{{Stage: "fetch", ItemKey: "a", Kind: "missing"}}}
	b := a
	b.Gaps = append([]InventoryGap{}, a.Gaps...)
	b.Gaps[0].AcceptedAt = "2024-01-01"
	b.Gaps[0].Note = "accepted"
	if a.SourceManifestHash() != b.SourceManifestHash() || a.IdentityHash() != b.IdentityHash() {
		t.Fatal("acceptance changed inventory hashes")
	}
}

func TestInventory_V1GoldenBytesUnchanged(t *testing.T) {
	i := Inventory{Country: "vn", SourceListHash: "source", ExpectedKeys: []string{"a"}, Items: []InventoryItem{{Stage: "fetch", ItemKey: "a", State: "done"}}}
	b, err := json.Marshal(i.sorted())
	if err != nil {
		t.Fatal(err)
	}
	const manifest = `{"country":"vn","source_list_hash":"source","expected_keys":["a"],"items":[{"stage":"fetch","item_key":"a","state":"done","input_hash":null,"recipe_hash":null,"artifact_sha256":null}],"gaps":[]}`
	const identity = `{"country":"vn","source_list_hash":"source","expected_keys":["a"],"items":[{"stage":"fetch","item_key":"a","state":"done","input_hash":null,"recipe_hash":null,"artifact_sha256":null}]}`
	id, err := i.IdentityJSON()
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != manifest || string(id) != identity || i.SourceManifestHash() != "5483b2b0639336bf5e18b702ac0e113277639b0791d454b9aca9896f9f1f7509" || i.IdentityHash() != "2dbd2612003114cf16ad720f81042db42dd61159c2252e2897264475e10b272e" {
		t.Fatal("version 1 fixture changed")
	}
}

func TestInventory_V2GoldenBytesAndHashes(t *testing.T) {
	i := Inventory{Version: 2, Country: "vn", SourceListHash: "source", FinalMembershipHash: "final", ExpectedKeys: []string{"a"}, Items: []InventoryItem{{Stage: "fetch", ItemKey: "a", State: "done"}}}
	b, err := json.Marshal(i.sorted())
	if err != nil {
		t.Fatal(err)
	}
	const manifest = `{"version":2,"country":"vn","source_list_hash":"source","final_membership_hash":"final","expected_keys":["a"],"items":[{"stage":"fetch","item_key":"a","state":"done","input_hash":null,"recipe_hash":null,"artifact_sha256":null}],"gaps":[]}`
	const identity = `{"version":2,"country":"vn","source_list_hash":"source","final_membership_hash":"final","expected_keys":["a"],"items":[{"stage":"fetch","item_key":"a","state":"done","input_hash":null,"recipe_hash":null,"artifact_sha256":null}]}`
	id, err := i.IdentityJSON()
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != manifest || string(id) != identity || i.SourceManifestHash() != "0b4d3814bf415fed83d8df0f4834cab793f25e0424f0e10a03665db459113c26" || i.IdentityHash() != "fd29b24b2873815d2e50ce790602aa4e6e7a8be9831267238c3ece94d23d1178" {
		t.Fatalf("v2 fixture mismatch: %s / %s", b, id)
	}
}

func TestInventory_SnapshotHashMismatchFails(t *testing.T) {
	_, err := verifySnapshot([]byte(`{"version":1,"entries":[]}`), "bad")
	if err == nil {
		t.Fatal("accepted wrong snapshot hash")
	}
}

func TestInventory_RepublishedFinalChangesBuildInput(t *testing.T) {
	a := Inventory{Version: 2, FinalMembershipHash: "first"}
	b := a
	b.FinalMembershipHash = "second"
	if inventoryInputHash("vn", "20990101", a, "map") == inventoryInputHash("vn", "20990101", b, "map") {
		t.Fatal("final hash did not bind input")
	}
}

func TestBuild_MembershipUnreconciled(t *testing.T) {
	i := Inventory{Version: 2, Membership: membership.PhaseSnapshot{Active: []membership.Entry{{DocumentKey: "a"}}}}
	if err := reconcileMembership(i); err == nil || !strings.Contains(err.Error(), "a") {
		t.Fatalf("missing key: %v", err)
	}
}

func TestProvenance_SnapshotStableAndBound(t *testing.T) {
	a := provenanceSnapshot([]provenanceFile{{DocumentKey: "b", Name: "x"}, {DocumentKey: "a", Name: "x"}}, map[string]bool{"a": true})
	b := provenanceSnapshot([]provenanceFile{{DocumentKey: "a", Name: "x"}}, map[string]bool{"a": true})
	if hashJSON(a) != hashJSON(b) || len(a.Files) != 1 {
		t.Fatal("provenance differs")
	}
	i := Inventory{Version: 2, Provenance: a}
	before := inventoryInputHash("vn", "20990101", i, "map")
	i.Provenance.Files[0].Name = "changed"
	if before == inventoryInputHash("vn", "20990101", i, "map") {
		t.Fatal("provenance not bound")
	}
}

func TestBuild_RequiresFinalMembership(t *testing.T) {
	if _, err := verifySnapshot(nil, ""); err == nil {
		t.Fatal("accepted missing final membership")
	}
}

func TestInventory_V2RoundTripStructuredGaps(t *testing.T) {
	value := "source"
	original := Inventory{Version: 2, Country: "vn", FinalMembershipHash: "hash", ExpectedKeys: []string{}, Items: []InventoryItem{}, Gaps: []InventoryGap{{Stage: "fetch", ItemKey: "a", Source: &value}}}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Inventory
	if err = json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.SourceManifestHash() != original.SourceManifestHash() || decoded.Gaps[0].Source == nil || *decoded.Gaps[0].Source != value {
		t.Fatalf("round trip: %s", data)
	}
}

type inventoryQueryStub struct {
	store.Querier
	raw json.RawMessage
}

func (q inventoryQueryStub) GetBuildInventoryV2(context.Context, int64) (json.RawMessage, error) {
	return q.raw, nil
}

func TestInventory_V2QuerySnapshot(t *testing.T) {
	entries := []membership.Entry{{DocumentKey: "a", Disposition: "active"}, {DocumentKey: "b", Disposition: "scope_removed"}, {DocumentKey: "c", Disposition: "source_replaced"}}
	snapshot, hash, err := membership.CanonicalSnapshot(entries)
	if err != nil {
		t.Fatal(err)
	}
	payload := struct {
		Items         []map[string]any `json:"items"`
		Gaps          []map[string]any `json:"gaps"`
		FinalSnapshot []byte           `json:"final_snapshot"`
		FinalHash     string           `json:"final_hash"`
		Files         []provenanceFile `json:"files"`
	}{
		Items: []map[string]any{{"stage": "fetch", "item_key": "a", "state": "done"}, {"stage": "fetch", "item_key": "c", "state": "done"}},
		Gaps:  []map[string]any{{"stage": "fetch", "item_key": "a", "kind": "x", "detail": "y", "source": "vbpl"}}, FinalSnapshot: snapshot, FinalHash: hash,
		Files: []provenanceFile{{DocumentKey: "a", Name: "kept"}, {DocumentKey: "c", Name: "excluded"}},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	run := stage.Run{Country: "vn", Snapshot: stage.Snapshot{SourceList: []byte(`{"vbpl":{"selection_version":1}}`)}}
	inv, err := LoadInventory(context.Background(), inventoryQueryStub{raw: raw}, run, "embed")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(inv.ExpectedKeys, []string{"a", "b"}) || len(inv.Membership.Entries) != 3 || len(inv.Provenance.Files) != 1 || inv.Provenance.Files[0].Name != "kept" || inv.Gaps[0].Source == nil || *inv.Gaps[0].Source != "vbpl" {
		t.Fatalf("unexpected inventory: %+v", inv)
	}
}

func TestBuild_ReconciliationStates(t *testing.T) {
	active := membership.PhaseSnapshot{Active: []membership.Entry{{DocumentKey: "a"}}}
	tests := []struct {
		name     string
		items    []InventoryItem
		failures int
		paused   []string
		want     bool
	}{
		{name: "fetch gap", items: []InventoryItem{{Stage: "fetch", ItemKey: "a", State: "gap"}}},
		{name: "present terminal", items: []InventoryItem{{Stage: "fetch", ItemKey: "a", State: "done"}, {Stage: "extract", ItemKey: "a", State: "gap"}, {Stage: "relate", ItemKey: "a", State: "done"}}},
		{name: "missing relate", items: []InventoryItem{{Stage: "fetch", ItemKey: "a", State: "done"}, {Stage: "extract", ItemKey: "a", State: "done"}}, want: true},
		{name: "durable failure", failures: 1, want: true},
		{name: "paused source", paused: []string{"vbpl"}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := reconcileMembership(Inventory{Version: 2, Membership: active, Items: tt.items, Failures: tt.failures, PausedSources: tt.paused})
			if (err != nil) != tt.want {
				t.Fatalf("reconcile: %v", err)
			}
		})
	}
}

func (q inventoryQueryStub) GetBuildInventory(context.Context, int64) (json.RawMessage, error) {
	return q.raw, nil
}

func TestInventory_V1IgnoresExpand(t *testing.T) {
	q := inventoryQueryStub{raw: json.RawMessage(`{"items":[{"stage":"expand","item_key":"a","state":"done"}],"gaps":[{"stage":"expand","item_key":"a","kind":"x","detail":"y"}]}`)}
	inv, err := LoadInventory(context.Background(), q, stage.Run{Snapshot: stage.Snapshot{SourceList: []byte(`{"vbpl":{"selection_version":0}}`)}}, "embed")
	if err != nil {
		t.Fatal(err)
	}
	if len(inv.Items) != 0 || len(inv.Gaps) != 0 {
		t.Fatalf("legacy included expand: %+v", inv)
	}
}
