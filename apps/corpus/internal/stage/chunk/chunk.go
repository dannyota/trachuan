package chunk

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/fetch"
	"danny.vn/trachuan/apps/corpus/internal/stage/membership"
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
	q := store.New(env.DB)
	active, err := membership.FinalActiveKeys(ctx, q, run)
	if err != nil {
		return nil, err
	}
	inputs, err := q.ListChunkInputs(ctx, run.ID)
	if err != nil {
		return nil, fmt.Errorf("list chunk inputs: %w", err)
	}
	items := make([]stage.Item, 0, len(inputs))
	for _, in := range inputs {
		if active != nil && !active[in.ItemKey] {
			continue
		}
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
	if in.FetchKey != item.Key || in.ExtractKey != item.Key {
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
	if err = checkExtractLineage(env.Artifacts.Abs(in.ExtractPath), in.FetchSha256, in.ExtractInputHash); err != nil {
		return stage.Output{}, fmt.Errorf("chunk input lineage: %w", err)
	}

	file, coverage, err := chunkInputs(env.Artifacts.Abs(in.FetchPath), env.Artifacts.Abs(in.ExtractPath), item.Key)
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

func chunkInputs(fetchDir, extractDir, key string) (File, Coverage, error) {
	doc, err := fetch.ReadDocument(fetchDir, key)
	if err != nil {
		return File{}, Coverage{}, fmt.Errorf("read fetch document: %w", err)
	}
	var nodes []Node
	if err = decode(filepath.Join(extractDir, "provisions.json"), &nodes); err != nil {
		return File{}, Coverage{}, err
	}
	return Split(nodes, Meta{Key: key, Number: doc.Number, Title: doc.Title, EffFrom: doc.EffFrom, IssueDate: doc.IssueDate}, Recipe1())
}

func checkExtractLineage(dir, fetchHash, inputHash string) error {
	return fetch.CheckExtractLineage(dir, fetchHash, inputHash)
}
