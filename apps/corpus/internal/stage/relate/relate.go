// Package relate extracts relations from VBPL fetch artifacts.
package relate

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/discover"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"github.com/jackc/pgx/v5"
)

// Stage extracts and resolves document relations.
type Stage struct{}

// New constructs a relate stage.
func New() *Stage { return &Stage{} }

// Name returns the stage name.
func (*Stage) Name() string { return "relate" }

// Recipe returns the recipe hash for the frozen relation code table.
func (*Stage) Recipe(snapshot stage.Snapshot) string {
	list, _ := source.Parse(snapshot.SourceList)
	return stage.RecipeHash("relate", "vbpl-relations/1", list.RelationCodesHash())
}

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

// Process publishes relations and unmapped codes from a fetch artifact.
func (s *Stage) Process(ctx context.Context, env stage.Env, run stage.Run, item stage.Item) (stage.Output, error) {
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
	detail, err := os.ReadFile(filepath.Join(env.Artifacts.Abs(input.Path), "detail.json"))
	if err != nil {
		return stage.Output{}, fmt.Errorf("read detail: %w", err)
	}
	diagram, err := os.ReadFile(filepath.Join(env.Artifacts.Abs(input.Path), "diagram.json"))
	if err != nil {
		return stage.Output{}, fmt.Errorf("read diagram: %w", err)
	}
	list, err := source.Parse(run.Snapshot.SourceList)
	if err != nil {
		return stage.Output{}, fmt.Errorf("parse source list: %w", err)
	}
	rows, unmapped, err := relations(detail, diagram, list.RelationCodes)
	if err != nil {
		return stage.Output{}, fmt.Errorf("extract relations: %w", err)
	}
	art, err := env.Artifacts.Publish(ctx, s.Name(), item.InputHash, s.Recipe(run.Snapshot), func(dir string) error {
		for _, file := range []struct {
			name  string
			value any
		}{{"relations.json", rows}, {"report.json", unmapped}} {
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
		return stage.Output{}, fmt.Errorf("publish relate artifact: %w", err)
	}
	return stage.Output{Artifact: &art, State: "done"}, nil
}

func readJSON(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if err = json.Unmarshal(data, value); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

// Rows loads relation rows from the artifact.
func (*Stage) Rows(_ context.Context, env stage.Env, art artifact.Result) (func(context.Context, pgx.Tx, int64) error, error) {
	var rows []relation
	if err := readJSON(filepath.Join(env.Artifacts.Abs(art.Path), "relations.json"), &rows); err != nil {
		return nil, err
	}
	return func(ctx context.Context, tx pgx.Tx, id int64) error {
		q := store.New(tx)
		for _, row := range rows {
			err := q.InsertRelation(ctx, store.InsertRelationParams{ArtifactID: id, Ordinal: int32(row.Ordinal), Kind: row.Kind, RawCode: row.RawCode, RawLabel: row.RawLabel, Method: row.Method, Evidence: row.Evidence, TargetVbplID: row.TargetVBPLID, TargetNumber: row.TargetNumber, SourceNodeID: row.SourceNodeID, TargetNodeID: row.TargetNodeID})
			if err != nil {
				return fmt.Errorf("insert relation: %w", err)
			}
		}
		return nil
	}, nil
}

// Resolve checks unmapped codes and targets against this run.
func (*Stage) Resolve(ctx context.Context, env stage.Env, run stage.Run, _ stage.Item, art artifact.Result) (stage.Resolution, error) {
	var rows []relation
	if err := readJSON(filepath.Join(env.Artifacts.Abs(art.Path), "relations.json"), &rows); err != nil {
		return stage.Resolution{}, err
	}
	var unmapped []string
	if err := readJSON(filepath.Join(env.Artifacts.Abs(art.Path), "report.json"), &unmapped); err != nil {
		return stage.Resolution{}, err
	}
	entries, err := discover.ExpectedSet(ctx, env.DB, run)
	if err != nil {
		return stage.Resolution{}, fmt.Errorf("load expected documents: %w", err)
	}
	expected := make(map[string]bool, len(entries))
	for _, entry := range entries {
		expected[entry.Key] = true
	}
	return stage.Resolution{Gaps: resolveGaps(rows, unmapped, expected)}, nil
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
