// Package discover lists documents from the frozen VBPL queries.
package discover

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/chinhphu"
	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"danny.vn/trachuan/apps/corpus/internal/vbpl"
	"danny.vn/trachuan/internal/retrieval"
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
	Detail   string                     `json:"detail,omitempty"`
	Items    map[string]json.RawMessage `json:"items"`
}

// Stage discovers the documents named by VBPL listings.
type Stage struct {
	client   *vbpl.Client
	chinhphu *chinhphu.Client
	ceiling  int
}

// Option configures discovery.
type Option func(*Stage)

// WithChinhphu sets the secondary source client.
func WithChinhphu(client *chinhphu.Client) Option { return func(s *Stage) { s.chinhphu = client } }

// WithCandidateCeiling sets a lower ceiling for controlled runs.
func WithCandidateCeiling(limit int) Option {
	return func(s *Stage) {
		if limit > 0 && limit < 100 {
			s.ceiling = limit
		}
	}
}

// New constructs a discover stage.
func New(client *vbpl.Client, options ...Option) *Stage {
	s := &Stage{client: client}
	for _, option := range options {
		option(s)
	}
	return s
}

// Name returns the stage name.
func (*Stage) Name() string { return "discover" }

// Recipe returns the discovery recipe hash.
func (*Stage) Recipe(snapshot stage.Snapshot) string {
	list, err := source.Parse(snapshot.SourceList)
	if err == nil && list.VBPL.SelectionVersion == 1 {
		if len(list.Chinhphu.Keywords) > 0 || len(list.Chinhphu.NamedNumbers) > 0 {
			return stage.RecipeHash("discover", "secondary-discover/1", "issuer-classifier/1", "issuer-normalization/1", retrieval.NumberNormalizationVersion, snapshot.SourceListHash, string(snapshot.SourceList))
		}
		return stage.RecipeHash("discover", "vbpl-discover/selection-2", snapshot.SourceListHash, string(snapshot.SourceList))
	}
	return stage.RecipeHash("discover", "vbpl-discover/1")
}

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

func queryIdentity(key string, definition query) (string, error) {
	data, err := json.Marshal(definition)
	if err != nil {
		return "", fmt.Errorf("encode query: %w", err)
	}
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%s@%x", key, sum[:8]), nil
}

// Plan returns every frozen query without an input hash.
func (*Stage) Plan(_ context.Context, _ stage.Env, r stage.Run) ([]stage.Item, error) {
	list, err := source.Parse(r.Snapshot.SourceList)
	if err != nil {
		return nil, fmt.Errorf("parse discovery sources: %w", err)
	}
	if list.VBPL.SelectionVersion == 1 {
		if err = list.ValidateSecondary(); err != nil {
			return nil, fmt.Errorf("validate secondary sources: %w", err)
		}
		items := []stage.Item{{Key: "vbpl:central"}}
		for _, entry := range list.VBPL.NamedNumbers {
			items = append(items, stage.Item{Key: "vbpl:named:" + entry})
		}
		seen := map[string]bool{}
		for _, group := range []struct {
			prefix string
			values []string
		}{{"chinhphu:keyword:", list.Chinhphu.Keywords}, {"chinhphu:named:", list.Chinhphu.NamedNumbers}} {
			for _, value := range group.values {
				key := group.prefix + value
				if !seen[key] {
					items = append(items, stage.Item{Key: key})
					seen[key] = true
				}
			}
		}
		sort.Slice(items, func(i, j int) bool { return items[i].Key < items[j].Key })
		return items, nil
	}
	if list.VBPL.SelectionVersion != 0 {
		return nil, fmt.Errorf("unsupported selection version %d", list.VBPL.SelectionVersion)
	}
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

// Process collects listing passes and publishes their results.
func (s *Stage) Process(ctx context.Context, env stage.Env, r stage.Run, it stage.Item) (output stage.Output, err error) {
	list, parseErr := source.Parse(r.Snapshot.SourceList)
	if parseErr != nil {
		return stage.Output{}, fmt.Errorf("parse discovery sources: %w", parseErr)
	}
	if list.VBPL.SelectionVersion == 1 {
		if strings.HasPrefix(it.Key, "chinhphu:") {
			return s.processSecondary(ctx, env, r, it, list.Chinhphu)
		}
		return s.processScope(ctx, env, r, it, list.VBPL)
	}
	if list.VBPL.SelectionVersion != 0 {
		return stage.Output{}, fmt.Errorf("unsupported selection version %d", list.VBPL.SelectionVersion)
	}
	contacted := false
	defer func() {
		if contacted {
			output.Sources = []string{"vbpl"}
		}
	}()
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
	identity, err := queryIdentity(it.Key, definition)
	if err != nil {
		return stage.Output{}, err
	}
	result := report{QueryKey: identity, Items: make(map[string]json.RawMessage)}
	var passes [][]page
	var previous []string
	previousComplete := false
	for pass := 0; pass < 3; pass++ {
		var pages []page
		items := make(map[string]json.RawMessage)
		total, pageCap := 0, 1
		for number := 1; number <= pageCap; number++ {
			contacted = true
			p, listErr := s.client.List(ctx, vbpl.ListQuery{AgencyIDs: definition.Agencies, Keyword: definition.Keyword, Page: number})
			if listErr != nil {
				wrapped := fmt.Errorf("list query %q pass %d page %d: %w", it.Key, pass+1, number, listErr)
				if !errors.Is(listErr, vbpl.ErrTooLarge) && !errors.Is(listErr, vbpl.ErrNotFound) {
					return stage.Output{}, stage.Retryable(wrapped)
				}
				return stage.Output{}, wrapped
			}
			if number == 1 {
				total = p.Total
				if total > 0 && total <= 100_000 {
					pageCap = (total+49)/50 + 1
				}
			}
			if p.Total > 100_000 {
				result.Detail = fmt.Sprintf("total %d", p.Total)
			}
			raw := page{Total: p.Total, Items: make([]json.RawMessage, 0, len(p.Items))}
			for _, item := range p.Items {
				if item.ID == "" {
					return stage.Output{}, fmt.Errorf("list query %q returned an empty id", it.Key)
				}
				raw.Items = append(raw.Items, item.Raw)
				items[item.ID] = item.Raw
				result.Items[item.ID] = item.Raw
			}
			pages = append(pages, raw)
			if result.Detail != "" || len(p.Items) < 50 || len(items) >= total {
				break
			}
		}
		passes = append(passes, pages)
		result.Totals = append(result.Totals, total)
		result.Passes++
		if result.Detail != "" {
			break
		}
		current := make([]string, 0, len(items))
		for id := range items {
			current = append(current, id)
		}
		sort.Strings(current)
		complete := len(items) == total
		if complete && previousComplete && reflect.DeepEqual(previous, current) {
			result.Stable = true
			result.Items = items
			break
		}
		previous = current
		previousComplete = complete
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
func (*Stage) Rows(context.Context, stage.Env, artifact.Result) (func(context.Context, pgx.Tx, int64) error, error) {
	return nil, nil
}

// Resolve prepares listing rows and an instability gap for this run.
func (*Stage) Resolve(_ context.Context, env stage.Env, r stage.Run, it stage.Item, art artifact.Result) (stage.Resolution, error) {
	list, err := source.Parse(r.Snapshot.SourceList)
	if err != nil {
		return stage.Resolution{}, fmt.Errorf("parse discovery sources: %w", err)
	}
	if list.VBPL.SelectionVersion == 1 {
		if strings.HasPrefix(it.Key, "chinhphu:") {
			return resolveSecondary(env, it, art)
		}
		return resolveScope(env, r, it, art, list.VBPL)
	}
	if list.VBPL.SelectionVersion != 0 {
		return stage.Resolution{}, fmt.Errorf("unsupported selection version %d", list.VBPL.SelectionVersion)
	}
	data, err := os.ReadFile(filepath.Join(env.Artifacts.Abs(art.Path), "report.json"))
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
		detail := result.Detail
		if detail == "" {
			detail = strconv.Itoa(result.Passes)
		}
		resolution.Gaps = []stage.Gap{{Kind: "listing_unstable", Detail: detail}}
	}
	return resolution, nil
}
