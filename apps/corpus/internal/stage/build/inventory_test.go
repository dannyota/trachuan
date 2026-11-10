package build

import "testing"

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
