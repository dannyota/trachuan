package expand

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

func loadPinned(ctx context.Context, env stage.Env, r stage.Run, pin source.Pin) (store.GetPinnedArtifactRow, error) {
	return pinned.Load(ctx, store.New(env.DB), env.Artifacts, r.Country, pin, "expand")
}

func (s *Stage) processPinned(ctx context.Context, env stage.Env, r stage.Run, pin source.Pin, list source.List) (stage.Output, error) {
	loader := s.loadPinned
	if loader == nil {
		loader = loadPinned
	}
	row, err := loader(ctx, env, r, pin)
	if err != nil {
		return stage.Output{}, err
	}
	dir := env.Artifacts.Abs(row.Path)
	detail, err := os.ReadFile(filepath.Join(dir, "detail.json"))
	if err != nil {
		return stage.Output{}, fmt.Errorf("read pinned expand detail: %w", err)
	}
	diagram, err := os.ReadFile(filepath.Join(dir, "diagram.json"))
	if err != nil {
		return stage.Output{}, fmt.Errorf("read pinned expand diagram: %w", err)
	}
	observation, err := Qualify(pin.Key, detail, diagram, list.RelationCodes)
	if err != nil {
		return stage.Output{}, fmt.Errorf("qualify pinned expand: %w", err)
	}
	data, err := json.Marshal(observation)
	if err != nil {
		return stage.Output{}, fmt.Errorf("encode pinned observation: %w", err)
	}
	art, err := env.Artifacts.PublishContent(ctx, s.Name(), s.Recipe(r.Snapshot), func(dest string) error {
		for name, body := range map[string][]byte{"detail.json": detail, "diagram.json": diagram, "observation.json": data} {
			if writeErr := os.WriteFile(filepath.Join(dest, name), body, 0600); writeErr != nil {
				return fmt.Errorf("write pinned observation: %w", writeErr)
			}
		}
		return ctx.Err()
	})
	if err != nil {
		return stage.Output{}, fmt.Errorf("publish pinned expand: %w", err)
	}
	return stage.Output{State: "done", Message: stage.PinnedMessage, Artifact: &art}, nil
}
