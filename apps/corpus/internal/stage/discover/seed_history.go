package discover

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/run"
	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/membership"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"github.com/jackc/pgx/v5"
)

type observation struct {
	row      store.GetRunItemArtifactRow
	complete bool
	catalog  map[string]bool
}

func readObservation(ctx context.Context, q *store.Queries, env stage.Env, id int64, key string, central bool) (observation, error) {
	row, err := q.GetRunItemArtifact(ctx, store.GetRunItemArtifactParams{RunID: id, Stage: "discover", ItemKey: key})
	if errors.Is(err, pgx.ErrNoRows) {
		return observation{}, nil
	}
	if err != nil {
		return observation{}, fmt.Errorf("read discovery item %s: %w", key, err)
	}
	out := observation{row: row}
	if row.State != "done" {
		return out, nil
	}
	dir := env.Artifacts.Abs(row.Path)
	hash, err := artifact.Verify(dir)
	if err != nil {
		return out, fmt.Errorf("verify discovery artifact %s: %w", key, err)
	}
	if hash != row.Sha256 {
		return out, fmt.Errorf("discovery artifact %s hash mismatch", key)
	}
	var report scopeReport
	if err = readJSON(dir, "report.json", &report); err != nil {
		return out, err
	}
	out.complete = report.Version == 2 && report.Stable
	if central {
		var catalog catalogFile
		if err = readJSON(dir, "catalog.json", &catalog); err != nil {
			return out, err
		}
		out.complete = out.complete && catalog.Complete
		out.catalog = make(map[string]bool, len(catalog.IDs))
		for _, id := range catalog.IDs {
			out.catalog[id] = true
		}
	}
	return out, nil
}
func earlierRuns(ctx context.Context, q *store.Queries, r stage.Run) ([]store.ListEarlierRunsRow, error) {
	rows, err := q.ListEarlierRuns(ctx, store.ListEarlierRunsParams{Country: r.Country, ID: r.ID})
	if err != nil {
		return nil, fmt.Errorf("list earlier runs: %w", err)
	}
	out := rows[:0]
	for _, row := range rows {
		if run.CompareRel(row.Rel, r.Rel) < 0 {
			out = append(out, row)
		}
	}
	sort.Slice(out, func(i, j int) bool { return run.CompareRel(out[i].Rel, out[j].Rel) > 0 })
	return out, nil
}

func legacyMembership(ctx context.Context, q *store.Queries, id int64, country string) ([]membership.Entry, error) {
	keys, err := q.ListLegacyExpectedKeys(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("list legacy expected: %w", err)
	}
	rows, err := q.ListLegacyListingSupports(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("list legacy supports: %w", err)
	}
	supports := map[string][]membership.SupportTuple{}
	for _, row := range rows {
		kind, value := "", ""
		query := row.QueryKey
		switch {
		case strings.HasPrefix(query, "vbpl:sweep@"):
			kind, value = "agency", "62|908"
		case strings.HasPrefix(query, "vbpl:keyword:"):
			kind = "term"
			value = strings.TrimPrefix(strings.SplitN(query, "@", 2)[0], "vbpl:keyword:")
		default:
			continue
		}
		supports[row.ItemKey] = append(supports[row.ItemKey], membership.SupportTuple{RuleKind: kind, RuleValue: value, Evidence: membership.EvidenceRef{QueryIdentity: &query, ArtifactHash: row.ArtifactSha256, Path: "report.json"}})
	}
	out := make([]membership.Entry, 0, len(keys))
	prefix := country + ":vbpl:"
	for _, key := range keys {
		if !strings.HasPrefix(key, prefix) || englishRendition(key) {
			continue
		}
		out = append(out, membership.Entry{DocumentKey: key, Source: "vbpl", SourceID: strings.TrimPrefix(key, prefix), Class: "primary", Disposition: "active", Supports: supports[key]})
	}
	return out, nil
}
func earlierCatalog(ctx context.Context, q *store.Queries, env stage.Env, rows []store.ListEarlierRunsRow) (observation, error) {
	for _, row := range rows {
		var snapshot stage.Snapshot
		if err := json.Unmarshal(row.Snapshot, &snapshot); err != nil {
			return observation{}, fmt.Errorf("decode earlier snapshot: %w", err)
		}
		list, err := source.Parse(snapshot.SourceList)
		if err != nil {
			return observation{}, fmt.Errorf("parse earlier sources: %w", err)
		}
		if list.VBPL.SelectionVersion != 1 {
			continue
		}
		o, err := readObservation(ctx, q, env, row.ID, "vbpl:central", true)
		if err != nil {
			return o, err
		}
		if o.complete {
			return o, nil
		}
	}
	return observation{}, nil
}
