package chunk

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/store"
)

// Stage produces provision-owned chunks.
type Stage struct{}

// New constructs a chunk stage.
func New() *Stage { return &Stage{} }

// Name returns the stage name.
func (*Stage) Name() string { return "chunk" }

// Recipe returns the chunk recipe hash.
func (*Stage) Recipe(stage.Snapshot) string { return Recipe1().Hash() }

func inputHash(key, fetch, extract string) string {
	return stage.RecipeHash("chunk-input", "1", key, fetch, extract)
}

// Plan selects completed extract and fetch artifacts.
func (*Stage) Plan(ctx context.Context, env stage.Env, run stage.Run) ([]stage.Item, error) {
	inputs, err := store.New(env.DB).ListChunkInputs(ctx, run.ID)
	if err != nil {
		return nil, fmt.Errorf("list chunk inputs: %w", err)
	}
	items := make([]stage.Item, 0, len(inputs))
	for _, in := range inputs {
		items = append(items, stage.Item{Key: in.ItemKey, InputHash: inputHash(in.ItemKey, in.FetchSha256, in.ExtractSha256)})
	}
	return items, nil
}

func verifyInput(env stage.Env, path, expected string) error {
	hash, err := artifact.Verify(env.Artifacts.Abs(path))
	if err != nil {
		return fmt.Errorf("verify input artifact: %w", err)
	}
	if hash != expected {
		return fmt.Errorf("input manifest hash mismatch")
	}
	return nil
}
func decode(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read artifact: %w", err)
	}
	if err = json.Unmarshal(data, value); err != nil {
		return fmt.Errorf("decode artifact: %w", err)
	}
	return nil
}

// Process verifies inputs and publishes a chunk artifact.
func (s *Stage) Process(ctx context.Context, env stage.Env, run stage.Run, item stage.Item) (stage.Output, error) {
	in, err := store.New(env.DB).GetChunkInput(ctx, store.GetChunkInputParams{RunID: run.ID, ItemKey: item.Key})
	if err != nil {
		return stage.Output{}, fmt.Errorf("load chunk input: %w", err)
	}
	if in.FetchKey != item.Key || in.ExtractKey != item.Key || in.ExtractInputHash != in.FetchSha256 {
		return stage.Output{}, fmt.Errorf("chunk input lineage mismatch for %q", item.Key)
	}
	if inputHash(item.Key, in.FetchSha256, in.ExtractSha256) != item.InputHash {
		return stage.Output{}, fmt.Errorf("chunk input changed for %q", item.Key)
	}
	for _, x := range []struct{ path, hash string }{{in.FetchPath, in.FetchSha256}, {in.ExtractPath, in.ExtractSha256}} {
		if err = verifyInput(env, x.path, x.hash); err != nil {
			return stage.Output{}, err
		}
	}
	var detail struct {
		Data struct{ ID, DocNum, Title, EffFrom, IssueDate string } `json:"data"`
	}
	if err = decode(filepath.Join(env.Artifacts.Abs(in.FetchPath), "detail.json"), &detail); err != nil {
		return stage.Output{}, err
	}
	if "vn:vbpl:"+detail.Data.ID != item.Key {
		return stage.Output{}, fmt.Errorf("fetch document key mismatch")
	}
	var nodes []Node
	if err = decode(filepath.Join(env.Artifacts.Abs(in.ExtractPath), "provisions.json"), &nodes); err != nil {
		return stage.Output{}, err
	}
	file, coverage, err := Split(nodes, Meta{Key: item.Key, Number: detail.Data.DocNum, Title: detail.Data.Title, EffFrom: detail.Data.EffFrom, IssueDate: detail.Data.IssueDate}, Recipe1())
	if err != nil {
		return stage.Output{}, err
	}
	file.FetchSHA256, file.ExtractSHA256 = in.FetchSha256, in.ExtractSha256
	art, err := env.Artifacts.Publish(ctx, s.Name(), item.InputHash, s.Recipe(run.Snapshot), func(dir string) error {
		for _, entry := range []struct {
			name  string
			value any
		}{{"chunks.json", file}, {"coverage.json", coverage}} {
			data, marshalErr := json.Marshal(entry.value)
			if marshalErr != nil {
				return fmt.Errorf("encode %s: %w", entry.name, marshalErr)
			}
			if writeErr := os.WriteFile(filepath.Join(dir, entry.name), data, 0600); writeErr != nil {
				return fmt.Errorf("write %s: %w", entry.name, writeErr)
			}
		}
		return ctx.Err()
	})
	if err != nil {
		return stage.Output{}, fmt.Errorf("publish chunk artifact: %w", err)
	}
	return stage.Output{Artifact: &art, State: "done"}, nil
}
