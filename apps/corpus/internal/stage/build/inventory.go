// Package build assembles a corpus from a frozen workshop inventory.
package build

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"

	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/membership"
	"danny.vn/trachuan/apps/corpus/internal/store"
)

// InventoryItem identifies one frozen stage result.
type InventoryItem struct {
	Stage          string  `json:"stage"`
	ItemKey        string  `json:"item_key"`
	State          string  `json:"state"`
	InputHash      *string `json:"input_hash"`
	RecipeHash     *string `json:"recipe_hash"`
	ArtifactSHA256 *string `json:"artifact_sha256"`
	Path           *string `json:"-"`
}

// InventoryGap records an upstream coverage gap.
type InventoryGap struct {
	NodeID      *int64  `json:"-"`
	Stage       string  `json:"stage"`
	ItemKey     string  `json:"item_key"`
	Kind        string  `json:"kind"`
	Detail      string  `json:"detail"`
	AcceptedAt  string  `json:"-"`
	Note        string  `json:"-"`
	Source      *string `json:"-"`
	Origin      *string `json:"-"`
	Cause       *string `json:"-"`
	EvidenceRef *string `json:"-"`
}

// Inventory holds frozen source identities and upstream results.
type Inventory struct {
	Version             int                      `json:"-"`
	FinalMembershipHash string                   `json:"-"`
	Membership          membership.PhaseSnapshot `json:"-"`
	Provenance          provenance               `json:"-"`
	Failures            int                      `json:"-"`
	PausedSources       []string                 `json:"-"`
	Country             string                   `json:"country"`
	SourceListHash      string                   `json:"source_list_hash"`
	ExpectedKeys        []string                 `json:"expected_keys"`
	Items               []InventoryItem          `json:"items"`
	Gaps                []InventoryGap           `json:"gaps"`
}

func stageOrder(s string) int {
	switch s {
	case "discover":
		return 0
	case "expand":
		return 1
	case "fetch":
		return 2
	case "extract":
		return 3
	case "relate":
		return 4
	case "chunk":
		return 5
	case "embed":
		return 6
	}
	return 7
}

func (i Inventory) order(s string) int {
	if i.Version != 2 && s == "expand" {
		return 7
	}
	return stageOrder(s)
}

func (i Inventory) sorted() Inventory {
	i.ExpectedKeys = append([]string{}, i.ExpectedKeys...)
	i.Items = append([]InventoryItem{}, i.Items...)
	i.Gaps = append([]InventoryGap{}, i.Gaps...)
	sort.Strings(i.ExpectedKeys)
	sort.Slice(i.Items, func(a, b int) bool {
		x, y := i.Items[a], i.Items[b]
		if x.Stage != y.Stage {
			return i.order(x.Stage) < i.order(y.Stage)
		}
		return x.ItemKey < y.ItemKey
	})
	sort.Slice(i.Gaps, func(a, b int) bool {
		x, y := i.Gaps[a], i.Gaps[b]
		if x.Stage != y.Stage {
			return i.order(x.Stage) < i.order(y.Stage)
		}
		if x.ItemKey != y.ItemKey {
			return x.ItemKey < y.ItemKey
		}
		if x.Kind != y.Kind {
			return x.Kind < y.Kind
		}
		return x.Detail < y.Detail
	})
	return i
}
func hashJSON(v any) string {
	b, _ := json.Marshal(v)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// SourceManifestHash returns the hash of the sorted upstream inventory.
func (i Inventory) SourceManifestHash() string { return hashJSON(i.sorted()) }

// IdentityJSON returns the compact sorted identity inventory.
func (i Inventory) IdentityJSON() ([]byte, error) {
	i = i.sorted()
	items := make([]InventoryItem, 0, len(i.Items))
	for _, item := range i.Items {
		if item.Stage == "fetch" || item.Stage == "extract" || item.Stage == "relate" || (i.Version == 2 && (item.Stage == "discover" || item.Stage == "expand")) {
			items = append(items, item)
		}
	}
	if i.Version == 2 {
		return json.Marshal(identityV2{2, i.Country, i.SourceListHash, i.FinalMembershipHash, i.ExpectedKeys, items})
	}
	return json.Marshal(struct {
		Country        string          `json:"country"`
		SourceListHash string          `json:"source_list_hash"`
		ExpectedKeys   []string        `json:"expected_keys"`
		Items          []InventoryItem `json:"items"`
	}{i.Country, i.SourceListHash, i.ExpectedKeys, items})
}

// IdentityHash returns the hash of the source identity inventory.
func (i Inventory) IdentityHash() string {
	b, _ := i.IdentityJSON()
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// LoadInventory reads the requested stages in one workshop snapshot.
func LoadInventory(ctx context.Context, q store.Querier, run stage.Run, through string) (Inventory, error) {
	limit := stageOrder(through)
	if limit == 7 {
		return Inventory{}, fmt.Errorf("unknown stage %q", through)
	}
	list, err := source.Parse(run.Snapshot.SourceList)
	if err != nil {
		return Inventory{}, fmt.Errorf("parse source list: %w", err)
	}
	if list.VBPL.SelectionVersion == 1 {
		return loadInventoryV2(ctx, q, run, limit)
	}
	if list.VBPL.SelectionVersion != 0 {
		return Inventory{}, fmt.Errorf("unsupported selection version %d", list.VBPL.SelectionVersion)
	}
	if through == "expand" {
		return Inventory{}, fmt.Errorf("unknown stage %q", through)
	}
	raw, err := q.GetBuildInventory(ctx, run.ID)
	if err != nil {
		return Inventory{}, fmt.Errorf("read inventory: %w", err)
	}
	var payload struct {
		Items []json.RawMessage `json:"items"`
		Gaps  []InventoryGap    `json:"gaps"`
	}
	if err = json.Unmarshal(raw, &payload); err != nil {
		return Inventory{}, fmt.Errorf("decode inventory: %w", err)
	}
	result := Inventory{Country: run.Country, SourceListHash: run.Snapshot.SourceListHash, ExpectedKeys: []string{}, Items: []InventoryItem{}, Gaps: []InventoryGap{}}
	for _, data := range payload.Items {
		var item InventoryItem
		if err = json.Unmarshal(data, &item); err != nil {
			return Inventory{}, fmt.Errorf("decode inventory item: %w", err)
		}
		var path struct {
			Path *string `json:"path"`
		}
		if err = json.Unmarshal(data, &path); err != nil {
			return Inventory{}, fmt.Errorf("decode artifact path: %w", err)
		}
		item.Path = path.Path
		if result.order(item.Stage) <= limit {
			result.Items = append(result.Items, item)
			if item.Stage == "fetch" {
				result.ExpectedKeys = append(result.ExpectedKeys, item.ItemKey)
			}
		}
	}
	for _, gap := range payload.Gaps {
		if result.order(gap.Stage) <= limit {
			result.Gaps = append(result.Gaps, gap)
		}
	}
	return result.sorted(), nil
}
