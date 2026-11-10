// Package fetch downloads VBPL documents into content-addressed bundles.
package fetch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/filecache"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/discover"
	"danny.vn/trachuan/apps/corpus/internal/vbpl"
	"golang.org/x/sync/errgroup"
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
	var detail, tree, diagram json.RawMessage
	var decoded vbpl.Detail
	var files []vbpl.File
	var errs [4]error
	group, requestCtx := errgroup.WithContext(ctx)
	group.Go(func() error { detail, decoded, errs[0] = s.client.Detail(requestCtx, id); return nil })
	group.Go(func() error { tree, errs[1] = s.client.RawTree(requestCtx, id); return nil })
	group.Go(func() error { diagram, errs[2] = s.client.Diagram(requestCtx, id); return nil })
	group.Go(func() error { files, errs[3] = s.client.Files(requestCtx, id); return nil })
	_ = group.Wait()
	if errs[0] != nil {
		return apiError(errs[0], id, true)
	}
	if decoded.ID != id {
		return stage.Output{State: "gap", Gaps: []stage.Gap{{Kind: "detail_id_mismatch", Detail: fmt.Sprintf("requested %q, received %q", id, decoded.ID)}}}, nil
	}
	for _, err := range errs[1:] {
		if err != nil {
			return apiError(err, id, false)
		}
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
	wrapped := fmt.Errorf("fetch document %q: %w", id, err)
	var syntax *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &syntax) || errors.As(err, &typeErr) {
		return stage.Output{}, stage.SourceData(wrapped)
	}
	var status *vbpl.StatusError
	if errors.As(err, &status) && status.Code >= 500 && status.Code < 600 {
		detail := []rune(wrapped.Error())
		if len(detail) > 200 {
			detail = detail[:200]
		}
		return stage.Output{State: "gap", Gaps: []stage.Gap{{Kind: "source_unavailable", Detail: string(detail)}}}, nil
	}
	return stage.Output{}, stage.Retryable(wrapped)
}
