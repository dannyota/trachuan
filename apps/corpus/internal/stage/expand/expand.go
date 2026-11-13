package expand

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/sourcegate"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/membership"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"danny.vn/trachuan/apps/corpus/internal/vbpl"
	"github.com/jackc/pgx/v5"
)

type Stage struct {
	client     *vbpl.Client
	loadPinned func(context.Context, stage.Env, stage.Run, source.Pin) (store.GetPinnedArtifactRow, error)
}

func New(client *vbpl.Client) *Stage { return &Stage{client: client} }

func (*Stage) Name() string { return "expand" }

func (*Stage) Recipe(snapshot stage.Snapshot) string {
	list, _ := source.Parse(snapshot.SourceList)
	return stage.RecipeHash("expand", "vbpl-expand/2", list.RelationCodesHash(), "legal_basis,implements,guided_by,amends_supplements,replaces,repeals,suspends,consolidates,cites,references", "outward", "one-hop")
}
func seedItems(entries []membership.Entry) []stage.Item {
	items := []stage.Item{}
	for _, e := range entries {
		if e.Source == "vbpl" && e.Class == "primary" && e.Disposition == "active" && !strings.HasPrefix(strings.ToLower(e.SourceID), "vbpqta_") {
			items = append(items, stage.Item{Key: e.DocumentKey})
		}
	}
	return items
}

func (*Stage) Plan(ctx context.Context, env stage.Env, r stage.Run) ([]stage.Item, error) {
	list, err := source.Parse(r.Snapshot.SourceList)
	if err != nil {
		return nil, fmt.Errorf("parse source list: %w", err)
	}
	if list.VBPL.SelectionVersion == 0 {
		return nil, nil
	}
	s, err := membership.ReadPhase(ctx, store.New(env.DB), r.ID, membership.Seeds)
	if err != nil {
		return nil, fmt.Errorf("read seeds: %w", err)
	}
	return seedItems(s.Entries), nil
}

func (s *Stage) Process(ctx context.Context, env stage.Env, r stage.Run, item stage.Item) (out stage.Output, err error) {
	list, err := source.Parse(r.Snapshot.SourceList)
	if err != nil {
		return out, fmt.Errorf("parse expand sources: %w", err)
	}
	if pin, ok := list.Pin(item.Key); ok {
		return s.processPinned(ctx, env, r, pin, list)
	}
	defer func() { out.Sources = []string{"vbpl"} }()
	prefix := r.Country + ":vbpl:"
	if r.Country == "" || !strings.HasPrefix(item.Key, prefix) || len(item.Key) == len(prefix) {
		return out, fmt.Errorf("invalid seed key %q", item.Key)
	}
	if s.client == nil {
		return out, fmt.Errorf("VBPL client is required")
	}
	id := strings.TrimPrefix(item.Key, prefix)
	type result struct {
		index   int
		raw     json.RawMessage
		decoded vbpl.Detail
		err     error
	}
	detailCtx, cancelDetail := context.WithCancel(ctx)
	defer cancelDetail()
	diagramCtx, cancelDiagram := context.WithCancel(ctx)
	defer cancelDiagram()
	diagramCtx, finishDiagram := sourcegate.WithBodyValidation(diagramCtx)
	results := make(chan result, 2)
	go func() {
		raw, decoded, callErr := s.client.Detail(detailCtx, id)
		results <- result{index: 0, raw: raw, decoded: decoded, err: callErr}
	}()
	go func() {
		raw, callErr := s.client.Diagram(diagramCtx, id)
		results <- result{index: 1, raw: raw, err: callErr}
	}()
	var calls [2]result
	for range 2 {
		res := <-results
		calls[res.index] = res
		if errors.Is(res.err, sourcegate.ErrSourcePaused) {
			cancelDetail()
			cancelDiagram()
		} else if res.index == 0 && (res.err != nil || res.decoded.ID != id) {
			cancelDiagram()
		}
	}
	for _, call := range calls {
		if errors.Is(call.err, sourcegate.ErrSourcePaused) {
			return apiError(call.err, id, call.index == 0)
		}
	}
	detail, decoded, err := calls[0].raw, calls[0].decoded, calls[0].err
	if err != nil {
		return apiError(err, id, true)
	}
	if decoded.ID != id {
		return stage.Output{State: "gap", Gaps: []stage.Gap{{Kind: "detail_id_mismatch", Detail: fmt.Sprintf("requested %q, received %q", id, decoded.ID)}}}, nil
	}
	diagram, err := calls[1].raw, calls[1].err
	if err != nil {
		return apiError(err, id, false)
	}
	observation, err := Qualify(item.Key, detail, diagram, list.RelationCodes)
	if err != nil {
		var diagramErr *diagramDecodeError
		finishDiagram(errors.As(err, &diagramErr))
		return out, stage.SourceData(err)
	}
	data, err := json.Marshal(observation)
	if err != nil {
		return out, fmt.Errorf("encode observation: %w", err)
	}
	art, err := env.Artifacts.PublishContent(ctx, s.Name(), s.Recipe(r.Snapshot), func(dir string) error {
		for _, f := range []struct {
			name string
			data []byte
		}{{"detail.json", detail}, {"diagram.json", diagram}, {"observation.json", data}} {
			if e := os.WriteFile(filepath.Join(dir, f.name), f.data, 0600); e != nil {
				return fmt.Errorf("write %s: %w", f.name, e)
			}
		}
		return ctx.Err()
	})
	if err != nil {
		return out, fmt.Errorf("publish expand artifact: %w", err)
	}
	return stage.Output{State: "done", Artifact: &art}, nil
}

func (*Stage) Rows(context.Context, stage.Env, artifact.Result) (func(context.Context, pgx.Tx, int64) error, error) {
	return nil, nil
}

func (*Stage) Resolve(context.Context, stage.Env, stage.Run, stage.Item, artifact.Result) (stage.Resolution, error) {
	return stage.Resolution{}, nil
}
