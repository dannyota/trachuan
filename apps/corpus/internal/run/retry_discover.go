package run

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/store"
)

func namedRetryIdentity(key string) (string, error) {
	entry, err := source.ParseNamedNumber(strings.TrimPrefix(key, "vbpl:named:"))
	if err != nil {
		return "", fmt.Errorf("parse retry item %q: %w", key, err)
	}
	return source.NamedLookupIdentity("vbpl", "docNum", "exact", "TRUNG_UONG", entry.Number), nil
}

func widenDiscoverRetry(ctx context.Context, q *store.Queries, runID int64, country string, plan *retryPlan) error {
	identities := map[string]bool{}
	selected := map[retryItem]bool{}
	for _, item := range plan.items {
		selected[item] = true
		if item.stage == "discover" && strings.HasPrefix(item.key, "vbpl:named:") {
			identity, err := namedRetryIdentity(item.key)
			if err != nil {
				return err
			}
			identities[identity] = true
		}
	}
	if len(identities) == 0 {
		return nil
	}
	items, err := q.ListDiscoveryItems(ctx, country)
	if err != nil {
		return fmt.Errorf("list discovery items for retry: %w", err)
	}
	for _, row := range items {
		item := retryItem{stage: "discover", key: row.ItemKey}
		if row.RunID != runID || selected[item] || !strings.HasPrefix(item.key, "vbpl:named:") {
			continue
		}
		identity, err := namedRetryIdentity(item.key)
		if err != nil {
			return err
		}
		if identities[identity] {
			plan.items = append(plan.items, item)
			selected[item] = true
		}
	}
	return nil
}

func deleteDiscoverListings(ctx context.Context, q *store.Queries, runID int64, key string) error {
	var identity string
	switch {
	case key == "vbpl:central":
		identity = source.CentralListingIdentity("vbpl", "TRUNG_UONG", 1000)
	case strings.HasPrefix(key, "vbpl:named:"):
		var err error
		identity, err = namedRetryIdentity(key)
		if err != nil {
			return err
		}
	case key == "vbpl:sweep":
		if _, err := q.DeleteRunListingsByItem(ctx, store.DeleteRunListingsByItemParams{RunID: runID, ItemKey: key}); err != nil {
			return fmt.Errorf("delete legacy discovery listings: %w", err)
		}
		return nil
	case strings.HasPrefix(key, "vbpl:keyword:"):
		rows, err := q.ListRunListings(ctx, runID)
		if err != nil {
			return fmt.Errorf("list legacy discovery listings: %w", err)
		}
		for _, row := range rows {
			suffix, ok := strings.CutPrefix(row.QueryKey, key+"@")
			if !ok || len(suffix) != 16 {
				continue
			}
			if _, err := hex.DecodeString(suffix); err != nil {
				continue
			}
			if _, err := q.DeleteRunListingsByQuery(ctx, store.DeleteRunListingsByQueryParams{RunID: runID, QueryKey: row.QueryKey}); err != nil {
				return fmt.Errorf("delete legacy discovery listings: %w", err)
			}
		}
		return nil
	default:
		return nil
	}
	if _, err := q.DeleteRunListingsByQuery(ctx, store.DeleteRunListingsByQueryParams{RunID: runID, QueryKey: identity}); err != nil {
		return fmt.Errorf("delete discovery listings: %w", err)
	}
	return nil
}
