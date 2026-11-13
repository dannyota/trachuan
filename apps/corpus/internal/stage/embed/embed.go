// Package embed produces dense vectors for chunk artifacts.
package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/chunk"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"danny.vn/trachuan/internal/embedding"
)

// Stage embeds chunks using a pinned document recipe.
type Stage struct {
	client   *embedding.Client
	expected embedding.Recipe
}

// New constructs an embed stage.
func New(client *embedding.Client, expected embedding.Recipe) *Stage {
	return &Stage{client: client, expected: expected}
}

// Name returns the stage name.
func (*Stage) Name() string { return "embed" }

// Recipe returns the complete stage recipe hash.
func (s *Stage) Recipe(stage.Snapshot) string {
	v := struct {
		Version        int              `json:"version"`
		Document       embedding.Recipe `json:"document"`
		DocumentHash   string           `json:"document_hash"`
		Input          string           `json:"input"`
		BatchTexts     int              `json:"batch_texts"`
		BodyBytes      int              `json:"body_bytes"`
		ArtifactFormat int              `json:"artifact_format"`
	}{1, s.expected, s.expected.Hash(), `strip(context_prefix)+"\n"+strip(content)`, 16, 1048576, 1}
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	_ = e.Encode(v)
	return stage.RecipeHash("embed", "dense/1", string(bytes.TrimSuffix(b.Bytes(), []byte("\n"))))
}

// Plan checks the service recipe and selects completed chunk artifacts.
func (s *Stage) Plan(ctx context.Context, env stage.Env, run stage.Run) ([]stage.Item, error) {
	report, err := s.client.Recipe(ctx)
	if err != nil {
		return nil, fmt.Errorf("read embedder recipe: %w", err)
	}
	if report.Recipe != s.expected || report.RecipeHash != s.expected.Hash() {
		return nil, fmt.Errorf("embedder recipe mismatch")
	}
	inputs, err := store.New(env.DB).ListEmbedInputs(ctx, run.ID)
	if err != nil {
		return nil, fmt.Errorf("list chunk artifacts: %w", err)
	}
	items := make([]stage.Item, 0, len(inputs))
	for _, in := range inputs {
		items = append(items, stage.Item{Key: in.ItemKey, InputHash: in.Sha256})
	}
	return items, nil
}

// Process verifies a chunk artifact and publishes its vectors.
func (s *Stage) Process(ctx context.Context, env stage.Env, run stage.Run, item stage.Item) (stage.Output, error) {
	in, err := store.New(env.DB).GetEmbedInput(ctx, store.GetEmbedInputParams{RunID: run.ID, ItemKey: item.Key})
	if err != nil {
		return stage.Output{}, fmt.Errorf("load chunk artifact: %w", err)
	}
	path, hash := in.Path, in.Sha256
	sum, err := artifact.Verify(env.Artifacts.Abs(path))
	if err != nil {
		return stage.Output{}, fmt.Errorf("verify chunk artifact: %w", err)
	}
	if sum != hash || hash != item.InputHash {
		return stage.Output{}, fmt.Errorf("chunk artifact hash mismatch")
	}
	file, coverage, err := chunk.ReadArtifact(env.Artifacts.Abs(path))
	if err != nil {
		return stage.Output{}, err
	}
	if file.Version != 1 || file.DocumentKey != item.Key || coverage.Version != 1 || coverage.DocumentKey != item.Key {
		return stage.Output{}, fmt.Errorf("chunk artifact identity mismatch")
	}
	hits, err := s.lookup(ctx, env, run, item, file)
	if err != nil {
		return stage.Output{}, err
	}
	idx, vectors, err := s.buildCached(ctx, file, hash, hits)
	hitCount := 0
	for _, c := range file.Chunks {
		if _, ok := hits[hashText(strings.TrimSpace(c.ContextPrefix)+"\n"+strings.TrimSpace(c.Content))]; ok {
			hitCount++
		}
	}
	slog.Info("embed cache", "run", run.Country+"/"+run.Rel, "key", item.Key, "hits", hitCount, "misses", len(file.Chunks)-hitCount)
	if err != nil {
		return stage.Output{}, err
	}
	report := makeReport(idx, coverage)
	art, err := env.Artifacts.Publish(ctx, s.Name(), hash, s.Recipe(run.Snapshot), func(dir string) error {
		for _, entry := range []struct {
			name  string
			value any
		}{{"index.json", idx}, {"report.json", report}} {
			data, e := json.Marshal(entry.value)
			if e != nil {
				return fmt.Errorf("encode %s: %w", entry.name, e)
			}
			if e = os.WriteFile(filepath.Join(dir, entry.name), data, 0600); e != nil {
				return fmt.Errorf("write %s: %w", entry.name, e)
			}
		}
		if e := os.WriteFile(filepath.Join(dir, "vectors.f32"), vectors, 0600); e != nil {
			return fmt.Errorf("write vectors: %w", e)
		}
		return ctx.Err()
	})
	if err != nil {
		return stage.Output{}, fmt.Errorf("publish embed artifact: %w", err)
	}
	return stage.Output{Artifact: &art, State: "done"}, nil
}
func classify(err error) error {
	var status *embedding.Error
	if errors.As(err, &status) {
		if status.Status == 429 || status.Status == 503 || status.Status == 504 {
			return stage.Retryable(err)
		}
		return err
	}
	var transport *url.Error
	if errors.As(err, &transport) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return stage.Retryable(err)
	}
	return fmt.Errorf("invalid embedder response: %v", err)
}
