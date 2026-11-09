// Package extract converts VBPL trees into provision artifacts.
package extract

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/dannyota/traquydinh/apps/corpus/internal/artifact"
	"github.com/dannyota/traquydinh/apps/corpus/internal/stage"
	"github.com/dannyota/traquydinh/apps/corpus/internal/store"
	"github.com/dannyota/traquydinh/apps/corpus/internal/vbpl"
	"github.com/jackc/pgx/v5"
)

// Stage extracts provisions from fetch artifacts.
type Stage struct{}

// New constructs an extract stage.
func New() *Stage { return &Stage{} }

// Name returns the stage name.
func (*Stage) Name() string { return "extract" }

// Recipe returns the extract recipe hash.
func (*Stage) Recipe(stage.Snapshot) string { return stage.RecipeHash("extract", "tree/1") }

// Plan selects completed fetch artifacts for this run.
func (*Stage) Plan(ctx context.Context, env stage.Env, run stage.Run) ([]stage.Item, error) {
	inputs, err := store.New(env.DB).ListExtractInputs(ctx, run.ID)
	if err != nil {
		return nil, fmt.Errorf("list fetch artifacts: %w", err)
	}
	items := make([]stage.Item, 0, len(inputs))
	for _, input := range inputs {
		items = append(items, stage.Item{Key: input.ItemKey, InputHash: input.Sha256})
	}
	return items, nil
}

// Process publishes the extracted provisions and gaps.
func (s *Stage) Process(ctx context.Context, env stage.Env, run stage.Run, item stage.Item) (stage.Output, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	input, err := store.New(env.DB).GetExtractInput(ctx, store.GetExtractInputParams{RunID: run.ID, ItemKey: item.Key})
	if err != nil {
		return stage.Output{}, fmt.Errorf("load fetch artifact: %w", err)
	}
	if input.Sha256 != item.InputHash {
		return stage.Output{}, stage.Retryable(fmt.Errorf("fetch artifact hash changed for %q", item.Key))
	}
	if verifyErr := verifyFetch(env.Artifacts.Abs(input.Path), item.InputHash); verifyErr != nil {
		return stage.Output{}, verifyErr
	}
	data, err := os.ReadFile(filepath.Join(env.Artifacts.Abs(input.Path), "tree.json"))
	if err != nil {
		return stage.Output{}, fmt.Errorf("read tree: %w", err)
	}
	var envelope struct {
		Data []vbpl.Node `json:"data"`
	}
	if err = json.Unmarshal(data, &envelope); err != nil {
		return stage.Output{}, fmt.Errorf("decode tree: %w", err)
	}
	rows, gaps, err := extractTree(ctx, envelope.Data)
	if err != nil {
		return stage.Output{}, fmt.Errorf("extract tree: %w", err)
	}
	art, err := env.Artifacts.Publish(ctx, s.Name(), item.InputHash, s.Recipe(run.Snapshot), func(dir string) error {
		for _, file := range []struct {
			name  string
			value any
		}{{"provisions.json", rows}, {"report.json", gaps}} {
			b, marshalErr := json.Marshal(file.value)
			if marshalErr != nil {
				return fmt.Errorf("encode %s: %w", file.name, marshalErr)
			}
			if writeErr := os.WriteFile(filepath.Join(dir, file.name), b, 0600); writeErr != nil {
				return fmt.Errorf("write %s: %w", file.name, writeErr)
			}
		}
		return ctx.Err()
	})
	if err != nil {
		return stage.Output{}, fmt.Errorf("publish extract artifact: %w", err)
	}
	return stage.Output{Artifact: &art, State: "done"}, nil
}

// Rows loads provision rows from the artifact.
func (*Stage) Rows(_ context.Context, env stage.Env, art artifact.Result) (func(context.Context, pgx.Tx, int64) error, error) {
	data, err := os.ReadFile(filepath.Join(env.Artifacts.Abs(art.Path), "provisions.json"))
	if err != nil {
		return nil, fmt.Errorf("read provisions: %w", err)
	}
	var rows []provision
	if err = json.Unmarshal(data, &rows); err != nil {
		return nil, fmt.Errorf("decode provisions: %w", err)
	}
	return func(ctx context.Context, tx pgx.Tx, id int64) error {
		q := store.New(tx)
		for _, row := range rows {
			if insertErr := q.InsertProvision(ctx, store.InsertProvisionParams{ArtifactID: id, NodeID: row.NodeID, ParentNodeID: row.ParentNodeID, Kind: row.Kind, Label: row.Label, Heading: row.Heading, Ordinal: int32(row.Ordinal), Path: row.Path, Text: row.Text}); insertErr != nil {
				return fmt.Errorf("insert provision: %w", insertErr)
			}
		}
		return nil
	}, nil
}

// Resolve returns gaps recorded in the artifact.
func (*Stage) Resolve(_ context.Context, env stage.Env, _ stage.Run, _ stage.Item, art artifact.Result) (stage.Resolution, error) {
	data, err := os.ReadFile(filepath.Join(env.Artifacts.Abs(art.Path), "report.json"))
	if err != nil {
		return stage.Resolution{}, fmt.Errorf("read extract report: %w", err)
	}
	var gaps []stage.Gap
	if err = json.Unmarshal(data, &gaps); err != nil {
		return stage.Resolution{}, fmt.Errorf("decode extract report: %w", err)
	}
	return stage.Resolution{Gaps: gaps}, nil
}

func verifyFetch(path, expected string) error {
	sum, err := artifact.Verify(path)
	if err != nil {
		return stage.Retryable(fmt.Errorf("verify fetch artifact: %w", err))
	}
	if sum != expected {
		return stage.Retryable(fmt.Errorf("fetch artifact hash mismatch: got %s, want %s", sum, expected))
	}
	return nil
}
