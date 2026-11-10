// Package expand freezes one-hop VBPL relation context.
package expand

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/membership"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"danny.vn/trachuan/apps/corpus/internal/vbpl"
	"github.com/jackc/pgx/v5"
)

// Stage observes direct relations of frozen primary seeds.
type Stage struct{ client *vbpl.Client }

// New constructs an expand stage.
func New(client *vbpl.Client) *Stage { return &Stage{client: client} }

// Name returns the stage name.
func (*Stage) Name() string { return "expand" }

// Recipe binds the qualifying kinds and relation table.
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

// Plan selects only active primary seeds from the verified frozen phase.
func (*Stage) Plan(ctx context.Context, env stage.Env, r stage.Run) ([]stage.Item, error) {
	s, err := membership.ReadPhase(ctx, store.New(env.DB), r.ID, membership.Seeds)
	if err != nil {
		return nil, fmt.Errorf("read seeds: %w", err)
	}
	return seedItems(s.Entries), nil
}

// Process downloads detail and diagram and publishes their qualified relations.
func (s *Stage) Process(ctx context.Context, env stage.Env, r stage.Run, item stage.Item) (out stage.Output, err error) {
	defer func() { out.Sources = []string{"vbpl"} }()
	prefix := r.Country + ":vbpl:"
	if r.Country == "" || !strings.HasPrefix(item.Key, prefix) || len(item.Key) == len(prefix) {
		return out, fmt.Errorf("invalid seed key %q", item.Key)
	}
	if s.client == nil {
		return out, fmt.Errorf("VBPL client is required")
	}
	id := strings.TrimPrefix(item.Key, prefix)
	detail, decoded, err := s.client.Detail(ctx, id)
	if err != nil {
		return apiError(err, id, true)
	}
	if decoded.ID != id {
		return stage.Output{State: "gap", Gaps: []stage.Gap{{Kind: "detail_id_mismatch", Detail: fmt.Sprintf("requested %q, received %q", id, decoded.ID)}}}, nil
	}
	diagram, err := s.client.Diagram(ctx, id)
	if err != nil {
		return apiError(err, id, false)
	}
	list, err := source.Parse(r.Snapshot.SourceList)
	if err != nil {
		return out, fmt.Errorf("parse source list: %w", err)
	}
	observation, err := Qualify(item.Key, detail, diagram, list.RelationCodes)
	if err != nil {
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

// Rows writes no artifact projections.
func (*Stage) Rows(context.Context, stage.Env, artifact.Result) (func(context.Context, pgx.Tx, int64) error, error) {
	return nil, nil
}

// Resolve writes no run-specific rows or gaps.
func (*Stage) Resolve(context.Context, stage.Env, stage.Run, stage.Item, artifact.Result) (stage.Resolution, error) {
	return stage.Resolution{}, nil
}
