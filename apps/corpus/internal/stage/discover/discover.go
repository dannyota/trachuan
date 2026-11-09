// Package discover lists documents from the frozen VBPL queries.
package discover

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/dannyota/traquydinh/apps/corpus/internal/artifact"
	"github.com/dannyota/traquydinh/apps/corpus/internal/source"
	"github.com/dannyota/traquydinh/apps/corpus/internal/stage"
	"github.com/dannyota/traquydinh/apps/corpus/internal/store"
	"github.com/dannyota/traquydinh/apps/corpus/internal/vbpl"
	"github.com/jackc/pgx/v5"
)

type query struct {
	Agencies []string `json:"agencies"`
	Keyword  string   `json:"keyword"`
}
type page struct {
	Total int               `json:"total"`
	Items []json.RawMessage `json:"items"`
}
type report struct {
	QueryKey string                     `json:"query_key"`
	Passes   int                        `json:"passes"`
	Totals   []int                      `json:"totals"`
	Stable   bool                       `json:"stable"`
	Items    map[string]json.RawMessage `json:"items"`
}

// Stage discovers the documents named by VBPL listings.
type Stage struct{ client *vbpl.Client }

// New constructs a discover stage.
func New(client *vbpl.Client) *Stage { return &Stage{client: client} }

// Name returns the stage name.
func (*Stage) Name() string { return "discover" }

// Recipe returns the discovery recipe hash.
func (*Stage) Recipe(stage.Snapshot) string { return stage.RecipeHash("discover", "vbpl-discover/1") }

func queries(r stage.Run) (map[string]query, error) {
	list, err := source.Parse(r.Snapshot.SourceList)
	if err != nil {
		return nil, fmt.Errorf("parse discovery sources: %w", err)
	}
	result := map[string]query{"vbpl:sweep": {Agencies: list.VBPL.SweepAgencies}}
	for _, keyword := range list.VBPL.Keywords {
		result["vbpl:keyword:"+keyword] = query{Agencies: list.VBPL.KeywordAgencies, Keyword: keyword}
	}
	return result, nil
}

// Plan returns every frozen query without an input hash.
func (*Stage) Plan(_ context.Context, _ stage.Env, r stage.Run) ([]stage.Item, error) {
	definitions, err := queries(r)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(definitions))
	for key := range definitions {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	items := make([]stage.Item, 0, len(keys))
	for _, key := range keys {
		items = append(items, stage.Item{Key: key})
	}
	return items, nil
}

// Process collects complete passes and publishes their union.
func (s *Stage) Process(ctx context.Context, env stage.Env, r stage.Run, it stage.Item) (stage.Output, error) {
	definitions, err := queries(r)
	if err != nil {
		return stage.Output{}, err
	}
	definition, ok := definitions[it.Key]
	if !ok {
		return stage.Output{}, fmt.Errorf("unknown discovery query %q", it.Key)
	}
	if s.client == nil {
		return stage.Output{}, fmt.Errorf("VBPL client is required")
	}
	data, err := json.Marshal(definition)
	if err != nil {
		return stage.Output{}, fmt.Errorf("encode query: %w", err)
	}
	sum := sha256.Sum256(data)
	result := report{QueryKey: fmt.Sprintf("%s@%x", it.Key, sum[:8]), Items: make(map[string]json.RawMessage)}
	var passes [][]page
	var previous []string
	for pass := 0; pass < 3; pass++ {
		var pages []page
		ids := make(map[string]struct{})
		total, received := 0, 0
		for number := 1; ; number++ {
			p, listErr := s.client.List(ctx, vbpl.ListQuery{AgencyIDs: definition.Agencies, Keyword: definition.Keyword, Page: number})
			if listErr != nil {
				return stage.Output{}, fmt.Errorf("list query %q pass %d page %d: %w", it.Key, pass+1, number, listErr)
			}
			total = p.Total
			received += len(p.Items)
			raw := page{Total: p.Total, Items: make([]json.RawMessage, 0, len(p.Items))}
			for _, item := range p.Items {
				if item.ID == "" {
					return stage.Output{}, fmt.Errorf("list query %q returned an empty id", it.Key)
				}
				raw.Items = append(raw.Items, item.Raw)
				ids[item.ID] = struct{}{}
				result.Items[item.ID] = item.Raw
			}
			pages = append(pages, raw)
			if len(p.Items) < 50 || received >= total {
				break
			}
		}
		passes = append(passes, pages)
		result.Totals = append(result.Totals, total)
		result.Passes++
		current := make([]string, 0, len(ids))
		for id := range ids {
			current = append(current, id)
		}
		sort.Strings(current)
		if pass > 0 && reflect.DeepEqual(previous, current) {
			result.Stable = true
			break
		}
		previous = current
	}
	art, err := env.Artifacts.PublishContent(ctx, s.Name(), s.Recipe(r.Snapshot), func(dir string) error {
		for _, file := range []struct {
			name  string
			value any
		}{{"passes.json", passes}, {"report.json", result}} {
			b, marshalErr := json.Marshal(file.value)
			if marshalErr != nil {
				return fmt.Errorf("encode %s: %w", file.name, marshalErr)
			}
			if writeErr := os.WriteFile(filepath.Join(dir, file.name), b, 0600); writeErr != nil {
				return fmt.Errorf("write %s: %w", file.name, writeErr)
			}
		}
		return nil
	})
	if err != nil {
		return stage.Output{}, fmt.Errorf("publish discovery artifact: %w", err)
	}
	return stage.Output{Artifact: &art, State: "done"}, nil
}

// Rows returns no artifact rows for listings.
func (*Stage) Rows(context.Context, artifact.Result) (func(context.Context, pgx.Tx, int64) error, error) {
	return nil, nil
}

// Resolve prepares listing rows and an instability gap for this run.
func (*Stage) Resolve(_ context.Context, _ stage.Env, r stage.Run, it stage.Item, art artifact.Result) (stage.Resolution, error) {
	data, err := os.ReadFile(filepath.Join(art.Path, "report.json"))
	if err != nil {
		return stage.Resolution{}, fmt.Errorf("read discovery report: %w", err)
	}
	var result report
	if err := json.Unmarshal(data, &result); err != nil {
		return stage.Resolution{}, fmt.Errorf("decode discovery report: %w", err)
	}
	if !strings.HasPrefix(result.QueryKey, it.Key+"@") {
		return stage.Resolution{}, fmt.Errorf("discovery report query does not match %q", it.Key)
	}
	resolution := stage.Resolution{RunRows: func(ctx context.Context, tx pgx.Tx) error {
		q := store.New(tx)
		keys := make([]string, 0, len(result.Items))
		for id := range result.Items {
			keys = append(keys, id)
		}
		sort.Strings(keys)
		for _, id := range keys {
			if err := q.UpsertListing(ctx, store.UpsertListingParams{RunID: r.ID, QueryKey: result.QueryKey, ItemKey: r.Country + ":vbpl:" + id, Payload: result.Items[id]}); err != nil {
				return fmt.Errorf("upsert listing %q: %w", id, err)
			}
		}
		return nil
	}}
	if !result.Stable {
		resolution.Gaps = []stage.Gap{{Kind: "listing_unstable", Detail: strconv.Itoa(result.Passes)}}
	}
	return resolution, nil
}
