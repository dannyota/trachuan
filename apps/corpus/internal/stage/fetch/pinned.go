package fetch

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"danny.vn/trachuan/apps/corpus/internal/pinned"
	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/store"
)

func loadPinned(ctx context.Context, env stage.Env, r stage.Run, pin source.Pin) (pinned.Acquisition, error) {
	return pinned.Acquire(ctx, store.New(env.DB), env.Artifacts, r.Country, pin)
}

func (s *Stage) processPinned(ctx context.Context, env stage.Env, r stage.Run, pin source.Pin) (stage.Output, error) {
	loader := s.loadPinned
	if loader == nil {
		loader = loadPinned
	}
	acquired, err := loader(ctx, env, r, pin)
	if err != nil {
		return stage.Output{}, err
	}
	dir := env.Artifacts.Abs(acquired.Artifact.Path)
	observations, err := pinnedObservations(dir, acquired, pin.Key)
	if err != nil {
		return stage.Output{}, err
	}
	art, err := env.Artifacts.PublishContent(ctx, s.Name(), s.Recipe(r.Snapshot), func(dest string) error { return pinned.Copy(ctx, dir, dest) })
	if err != nil {
		return stage.Output{}, fmt.Errorf("publish pinned fetch: %w", err)
	}
	if art.SHA256 != pin.Fetch {
		return stage.Output{}, fmt.Errorf("pinned bundle changed while copying")
	}
	return stage.Output{State: "done", Message: stage.PinnedMessage, Artifact: &art, Observations: observations}, nil
}

func pinnedObservations(dir string, acquired pinned.Acquisition, key string) ([]stage.FetchObservation, error) {
	var observations []stage.FetchObservation
	for _, res := range []struct{ endpoint, path string }{{"doc/detail", "detail.json"}, {"doc/diagram", "diagram.json"}, {"doc/provision/tree", "tree.json"}} {
		data, err := os.ReadFile(filepath.Join(dir, res.path))
		if err != nil {
			return nil, fmt.Errorf("read pinned payload: %w", err)
		}
		resource, _ := json.Marshal([3]any{res.endpoint, nil, nil})
		observations = append(observations, stage.FetchObservation{ResourceKey: string(resource), Source: "vbpl", Path: res.path, ContentSHA256: digest(data), Size: int64(len(data)), Acquisition: "pinned"})
	}
	data, err := os.ReadFile(filepath.Join(dir, "files.json"))
	if err != nil {
		return nil, fmt.Errorf("read pinned files: %w", err)
	}
	var files []file
	if err = json.Unmarshal(data, &files); err != nil {
		return nil, fmt.Errorf("decode pinned files: %w", err)
	}
	for _, f := range files {
		name := f.Name
		resource, _ := json.Marshal([3]any{"attachment", name, nil})
		observations = append(observations, stage.FetchObservation{ResourceKey: string(resource), Source: "vbpl", Name: &name, Path: "files/" + f.SHA256, ContentSHA256: f.SHA256, Size: f.Size, Acquisition: "pinned"})
	}
	result := []stage.FetchObservation{}
	seen := map[string]bool{}
	for _, o := range observations {
		matched := false
		for i := range acquired.Observations {
			origin := &acquired.Observations[i]
			if origin.ArtifactSha256 != acquired.Artifact.Sha256 || origin.RunID != acquired.Artifact.RunID || origin.ItemKey != key || origin.Generation != acquired.Artifact.Generation || origin.Attempt != acquired.Artifact.Attempt || origin.Source != o.Source || origin.Path != o.Path || origin.ContentSha256 != o.ContentSHA256 || origin.Size != o.Size {
				continue
			}
			var resource [3]*string
			if json.Unmarshal([]byte(origin.ResourceKey), &resource) != nil || resource[0] == nil {
				continue
			}
			if o.Name == nil {
				if origin.ResourceKey != o.ResourceKey {
					continue
				}
			} else if *resource[0] != "attachment" || resource[1] == nil || *resource[1] != *o.Name || !equalURL(resource[2], origin.Url) {
				continue
			}
			reused := adopted(origin)
			reused.Acquisition = "pinned"
			if !seen[reused.ResourceKey] {
				result = append(result, reused)
				seen[reused.ResourceKey] = true
			}
			matched = true
		}
		if !matched && !seen[o.ResourceKey] {
			result = append(result, o)
			seen[o.ResourceKey] = true
		}
	}
	return result, nil
}

func equalURL(a, b *string) bool { return a == nil && b == nil || a != nil && b != nil && *a == *b }
