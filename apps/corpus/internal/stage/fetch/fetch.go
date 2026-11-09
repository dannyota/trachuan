// Package fetch downloads VBPL documents into content-addressed bundles.
package fetch

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"danny.vn/traquydinh/apps/corpus/internal/filecache"
	"danny.vn/traquydinh/apps/corpus/internal/stage"
	"danny.vn/traquydinh/apps/corpus/internal/stage/discover"
	"danny.vn/traquydinh/apps/corpus/internal/vbpl"
)

// Stage fetches documents and their attachments.
type Stage struct {
	client    *vbpl.Client
	cache     *filecache.Cache
	fileLimit int64
}

// New constructs a fetch stage.
func New(client *vbpl.Client) *Stage { return &Stage{client: client, fileLimit: 50_000_000} }

// WithCache sets the optional attachment cache.
func (s *Stage) WithCache(cache *filecache.Cache) *Stage { s.cache = cache; return s }

// WithFileLimit sets the attachment size limit in bytes.
func (s *Stage) WithFileLimit(limit int64) *Stage { s.fileLimit = limit; return s }

// Name returns the stage name.
func (*Stage) Name() string { return "fetch" }

// Recipe returns the fetch recipe hash.
func (*Stage) Recipe(stage.Snapshot) string { return stage.RecipeHash("fetch", "vbpl-fetch/1") }

// Plan returns the expected documents and their skip reasons.
func (*Stage) Plan(ctx context.Context, env stage.Env, r stage.Run) ([]stage.Item, error) {
	expected, err := discover.ExpectedSet(ctx, env.DB, r)
	if err != nil {
		return nil, fmt.Errorf("load expected documents: %w", err)
	}
	items := make([]stage.Item, 0, len(expected))
	for _, e := range expected {
		items = append(items, stage.Item{Key: e.Key, Skip: e.Skip})
	}
	return items, nil
}

// Process fetches the source responses and publishes a bundle.
func (s *Stage) Process(ctx context.Context, env stage.Env, r stage.Run, item stage.Item) (stage.Output, error) {
	const prefix = "vn:vbpl:"
	if !strings.HasPrefix(item.Key, prefix) || len(item.Key) == len(prefix) {
		return stage.Output{}, fmt.Errorf("invalid document key %q", item.Key)
	}
	if s.client == nil {
		return stage.Output{}, fmt.Errorf("VBPL client is required")
	}
	id := strings.TrimPrefix(item.Key, prefix)
	detail, _, err := s.client.Detail(ctx, id)
	if err != nil {
		return apiError(err, id, true)
	}
	tree, _, err := s.client.Tree(ctx, id)
	if err != nil {
		return apiError(err, id, false)
	}
	diagram, err := s.client.Diagram(ctx, id)
	if err != nil {
		return apiError(err, id, false)
	}
	files, err := s.client.Files(ctx, id)
	if err != nil {
		return apiError(err, id, false)
	}
	art, err := env.Artifacts.PublishContent(ctx, s.Name(), s.Recipe(r.Snapshot), func(dir string) error { return s.writeBundle(ctx, dir, id, detail, tree, diagram, files) })
	if err != nil {
		return stage.Output{}, fmt.Errorf("publish fetch artifact: %w", err)
	}
	return stage.Output{Artifact: &art, State: "done"}, nil
}
func apiError(err error, id string, detail bool) (stage.Output, error) {
	if errors.Is(err, vbpl.ErrTooLarge) {
		return stage.Output{State: "gap", Gaps: []stage.Gap{{Kind: "response_too_large", Detail: id}}}, nil
	}
	if detail && errors.Is(err, vbpl.ErrNotFound) {
		return stage.Output{State: "gap", Gaps: []stage.Gap{{Kind: "not_found", Detail: id}}}, nil
	}
	return stage.Output{}, stage.Retryable(fmt.Errorf("fetch document %q: %w", id, err))
}
