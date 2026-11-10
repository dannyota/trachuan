package build

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/membership"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"go.temporal.io/sdk/temporal"
)

type identityV2 struct {
	Version             int             `json:"version"`
	Country             string          `json:"country"`
	SourceListHash      string          `json:"source_list_hash"`
	FinalMembershipHash string          `json:"final_membership_hash"`
	ExpectedKeys        []string        `json:"expected_keys"`
	Items               []InventoryItem `json:"items"`
}
type gapV2 struct {
	Stage       string  `json:"stage"`
	ItemKey     string  `json:"item_key"`
	Kind        string  `json:"kind"`
	Detail      string  `json:"detail"`
	Source      *string `json:"source"`
	Origin      *string `json:"origin"`
	Cause       *string `json:"cause"`
	EvidenceRef *string `json:"evidence_ref"`
}
type manifestV2 struct {
	identityV2
	Gaps []gapV2 `json:"gaps"`
}

func (i Inventory) MarshalJSON() ([]byte, error) {
	if i.Version != 2 {
		type legacy Inventory
		return json.Marshal(legacy(i))
	}
	gaps := make([]gapV2, 0, len(i.Gaps))
	for _, g := range i.Gaps {
		gaps = append(gaps, gapV2{g.Stage, g.ItemKey, g.Kind, g.Detail, g.Source, g.Origin, g.Cause, g.EvidenceRef})
	}
	return json.Marshal(manifestV2{identityV2{2, i.Country, i.SourceListHash, i.FinalMembershipHash, i.ExpectedKeys, i.Items}, gaps})
}
func (i *Inventory) UnmarshalJSON(data []byte) error {
	type legacy Inventory
	var v struct {
		*legacy
		Version             int     `json:"version"`
		FinalMembershipHash string  `json:"final_membership_hash"`
		Gaps                []gapV2 `json:"gaps"`
	}
	v.legacy = (*legacy)(i)
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	i.Version = v.Version
	i.FinalMembershipHash = v.FinalMembershipHash
	i.Gaps = make([]InventoryGap, 0, len(v.Gaps))
	for _, g := range v.Gaps {
		i.Gaps = append(i.Gaps, InventoryGap{Stage: g.Stage, ItemKey: g.ItemKey, Kind: g.Kind, Detail: g.Detail, Source: g.Source, Origin: g.Origin, Cause: g.Cause, EvidenceRef: g.EvidenceRef})
	}
	return nil
}
func hashBytes(b []byte) string { return fmt.Sprintf("%x", sha256.Sum256(b)) }
func verifySnapshot(data []byte, hash string) (membership.PhaseSnapshot, error) {
	if data == nil || hash == "" {
		return membership.PhaseSnapshot{}, fmt.Errorf("final membership phase is not frozen")
	}
	var s struct {
		Version int                `json:"version"`
		Entries []membership.Entry `json:"entries"`
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return membership.PhaseSnapshot{}, fmt.Errorf("decode membership snapshot: %w", err)
	}
	canonical, actual, err := membership.CanonicalSnapshot(s.Entries)
	if err != nil {
		return membership.PhaseSnapshot{}, err
	}
	if s.Version != 1 || actual != hash || !bytes.Equal(data, canonical) {
		return membership.PhaseSnapshot{}, fmt.Errorf("final membership snapshot failed verification")
	}
	result := membership.PhaseSnapshot{Entries: s.Entries, Hash: hash, Active: []membership.Entry{}, RetainedRemovals: []membership.Entry{}}
	for _, e := range s.Entries {
		switch e.Disposition {
		case "active":
			result.Active = append(result.Active, e)
		case "withdrawn", "scope_removed":
			result.RetainedRemovals = append(result.RetainedRemovals, e)
		}
	}
	return result, nil
}
func loadInventoryV2(ctx context.Context, q store.Querier, run stage.Run, limit int) (Inventory, error) {
	raw, err := q.GetBuildInventoryV2(ctx, run.ID)
	if err != nil {
		return Inventory{}, fmt.Errorf("read inventory: %w", err)
	}
	var p struct {
		Items         []json.RawMessage `json:"items"`
		Gaps          []gapV2           `json:"gaps"`
		FinalSnapshot []byte            `json:"final_snapshot"`
		FinalHash     *string           `json:"final_hash"`
		Failures      int               `json:"failures"`
		PausedSources []string          `json:"paused_sources"`
		Files         []provenanceFile  `json:"files"`
	}
	if err = json.Unmarshal(raw, &p); err != nil {
		return Inventory{}, fmt.Errorf("decode inventory: %w", err)
	}
	hash := ""
	if p.FinalHash != nil {
		hash = *p.FinalHash
	}
	snap, err := verifySnapshot(p.FinalSnapshot, hash)
	if err != nil {
		return Inventory{}, err
	}
	i := Inventory{Version: 2, Country: run.Country, SourceListHash: run.Snapshot.SourceListHash, FinalMembershipHash: hash, Membership: snap, ExpectedKeys: []string{}, Items: []InventoryItem{}, Gaps: []InventoryGap{}, Failures: p.Failures, PausedSources: p.PausedSources}
	for _, e := range append(append([]membership.Entry{}, snap.Active...), snap.RetainedRemovals...) {
		i.ExpectedKeys = append(i.ExpectedKeys, e.DocumentKey)
	}
	for _, data := range p.Items {
		var item InventoryItem
		var path struct {
			Path *string `json:"path"`
		}
		if err = json.Unmarshal(data, &item); err != nil {
			return Inventory{}, fmt.Errorf("decode item: %w", err)
		}
		if err = json.Unmarshal(data, &path); err != nil {
			return Inventory{}, fmt.Errorf("decode path: %w", err)
		}
		item.Path = path.Path
		if stageOrder(item.Stage) <= limit {
			i.Items = append(i.Items, item)
		}
	}
	for _, g := range p.Gaps {
		if stageOrder(g.Stage) <= limit {
			i.Gaps = append(i.Gaps, InventoryGap{Stage: g.Stage, ItemKey: g.ItemKey, Kind: g.Kind, Detail: g.Detail, Source: g.Source, Origin: g.Origin, Cause: g.Cause, EvidenceRef: g.EvidenceRef})
		}
	}
	present := i.Presence()
	i.Provenance = provenanceSnapshot(p.Files, present)
	return i.sorted(), nil
}
func reconcileMembership(i Inventory) error {
	if i.Version != 2 {
		return nil
	}
	if i.Failures > 0 {
		return fmt.Errorf("durable run failures: %d", i.Failures)
	}
	if len(i.PausedSources) > 0 {
		return fmt.Errorf("paused sources: %s", strings.Join(i.PausedSources, ", "))
	}
	items := map[string]map[string]string{}
	for _, it := range i.Items {
		if items[it.ItemKey] == nil {
			items[it.ItemKey] = map[string]string{}
		}
		items[it.ItemKey][it.Stage] = it.State
	}
	bad := []string{}
	terminal := func(s string) bool { return s == "done" || s == "gap" }
	for _, e := range i.Membership.Active {
		states := items[e.DocumentKey]
		if !terminal(states["fetch"]) || (states["fetch"] == "done" && (!terminal(states["extract"]) || !terminal(states["relate"]))) {
			bad = append(bad, e.DocumentKey)
		}
	}
	if len(bad) > 0 {
		sort.Strings(bad)
		return temporal.NewNonRetryableApplicationError("membership_unreconciled: "+strings.Join(bad, ", "), "membership_unreconciled", nil)
	}
	return nil
}
