package discover

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/run"
	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/membership"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Expected identifies a document and an optional reason to skip fetching it.
type Expected struct {
	Key, VBPLID string
	Skip        *stage.Gap
}

type discoveryRun struct {
	rel        string
	identities map[string]string
}

// ExpectedSet returns current and previously discovered documents in key order.
func ExpectedSet(ctx context.Context, db *pgxpool.Pool, r stage.Run) ([]Expected, error) {
	q := store.New(db)
	list, err := source.Parse(r.Snapshot.SourceList)
	if err != nil {
		return nil, fmt.Errorf("parse discovery sources: %w", err)
	}
	if list.VBPL.SelectionVersion == 1 {
		return expectedMembership(ctx, q, r)
	}
	runs, err := q.ListDiscoveryRuns(ctx, r.Country)
	if err != nil {
		return nil, fmt.Errorf("list discovery runs: %w", err)
	}
	listings, err := q.ListDiscoveryListings(ctx, r.Country)
	if err != nil {
		return nil, fmt.Errorf("list discovery listings: %w", err)
	}
	items, err := q.ListDiscoveryItems(ctx, r.Country)
	if err != nil {
		return nil, fmt.Errorf("list discovery items: %w", err)
	}
	current, err := discoveryIdentities(r)
	if err != nil {
		return nil, err
	}
	history := make(map[int64]discoveryRun)
	for _, row := range runs {
		if run.CompareRel(row.Rel, r.Rel) >= 0 {
			continue
		}
		var snapshot stage.Snapshot
		if err := json.Unmarshal(row.Snapshot, &snapshot); err != nil {
			return nil, fmt.Errorf("decode snapshot for %s: %w", row.Rel, err)
		}
		identities, err := discoveryIdentities(stage.Run{Snapshot: snapshot})
		if err != nil {
			return nil, fmt.Errorf("discovery queries for %s: %w", row.Rel, err)
		}
		history[row.ID] = discoveryRun{rel: row.Rel, identities: identities}
	}
	complete := make(map[int64]map[string]bool)
	for _, item := range items {
		if complete[item.RunID] == nil {
			complete[item.RunID] = make(map[string]bool)
		}
		complete[item.RunID][item.ItemKey] = item.Complete
	}
	byRun := make(map[int64]map[string]map[string]bool)
	for _, listing := range listings {
		if englishRendition(listing.ItemKey) {
			continue
		}
		if byRun[listing.RunID] == nil {
			byRun[listing.RunID] = make(map[string]map[string]bool)
		}
		if byRun[listing.RunID][listing.QueryKey] == nil {
			byRun[listing.RunID][listing.QueryKey] = make(map[string]bool)
		}
		byRun[listing.RunID][listing.QueryKey][listing.ItemKey] = true
	}
	// Retain every current listing, including unions from incomplete queries.
	documents := make(map[string]map[string]bool)
	for _, keys := range byRun[r.ID] {
		for key := range keys {
			documents[key] = make(map[string]bool)
		}
	}
	latest := make(map[string]int64)
	for id, h := range history {
		for identity, itemKey := range h.identities {
			if !complete[id][itemKey] {
				continue
			}
			if previous, ok := latest[identity]; !ok || run.CompareRel(h.rel, history[previous].rel) > 0 {
				latest[identity] = id
			}
			for key := range byRun[id][identity] {
				if documents[key] == nil {
					documents[key] = make(map[string]bool)
				}
				documents[key][identity] = true
			}
		}
	}
	result := make([]Expected, 0, len(documents))
	for key, identities := range documents {
		prefix := r.Country + ":vbpl:"
		if !strings.HasPrefix(key, prefix) || len(key) == len(prefix) {
			return nil, fmt.Errorf("invalid listing document key %q", key)
		}
		entry := Expected{Key: key, VBPLID: strings.TrimPrefix(key, prefix)}
		present := false
		for _, keys := range byRun[r.ID] {
			present = present || keys[key]
		}
		if !present {
			remaining, withdrawn := 0, true
			for identity := range identities {
				itemKey, ok := current[identity]
				if !ok {
					continue
				}
				remaining++
				previous, ok := latest[identity]
				if !complete[r.ID][itemKey] || !ok || byRun[previous][identity][key] {
					withdrawn = false
				}
			}
			if remaining == 0 {
				entry.Skip = &stage.Gap{Kind: "scope_removed", Detail: key}
			} else if withdrawn {
				entry.Skip = &stage.Gap{Kind: "withdrawn", Detail: key}
			}
		}
		result = append(result, entry)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Key < result[j].Key })
	return result, nil
}

func discoveryIdentities(r stage.Run) (map[string]string, error) {
	definitions, err := queries(r)
	if err != nil {
		return nil, err
	}
	identities := make(map[string]string, len(definitions))
	for key, definition := range definitions {
		identity, err := queryIdentity(key, definition)
		if err != nil {
			return nil, err
		}
		identities[identity] = key
	}
	return identities, nil
}

// englishRendition reports VBPL English translations (ids starting with
// vbpqta_). They are not legal text in Vietnamese, so they are never expected.
func englishRendition(key string) bool {
	return strings.Contains(key, ":vbpl:vbpqta_")
}

func expectedMembership(ctx context.Context, q *store.Queries, r stage.Run) ([]Expected, error) {
	phase := membership.Seeds
	for _, name := range r.Snapshot.Stages {
		if name == "expand" {
			phase = membership.Expanded
			break
		}
	}
	snapshot, err := membership.ReadPhase(ctx, q, r.ID, phase)
	if err != nil {
		return nil, fmt.Errorf("read expected membership: %w", err)
	}
	result := make([]Expected, 0, len(snapshot.Entries))
	for _, entry := range snapshot.Entries {
		if entry.Disposition == "source_replaced" {
			continue
		}
		if entry.Disposition != "active" && entry.Disposition != "withdrawn" && entry.Disposition != "scope_removed" {
			return nil, fmt.Errorf("unknown membership disposition %q", entry.Disposition)
		}
		e := Expected{Key: entry.DocumentKey, VBPLID: entry.SourceID}
		if entry.Disposition != "active" {
			e.Skip = &stage.Gap{Kind: entry.Disposition, Detail: entry.DocumentKey}
		}
		result = append(result, e)
	}
	return result, nil
}
