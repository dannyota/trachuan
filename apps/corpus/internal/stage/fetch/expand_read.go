package fetch

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"github.com/jackc/pgx/v5"
)

func loadExpand(ctx context.Context, env stage.Env, run stage.Run, item stage.Item) ([]byte, []byte, bool, error) {
	if env.DB == nil {
		return nil, nil, false, nil
	}
	return loadExpandFrom(ctx, store.New(env.DB), env, run, item)
}

type expandItemReader interface {
	GetRunItemArtifact(context.Context, store.GetRunItemArtifactParams) (store.GetRunItemArtifactRow, error)
}

func loadExpandFrom(ctx context.Context, q expandItemReader, env stage.Env, run stage.Run, item stage.Item) ([]byte, []byte, bool, error) {
	row, err := q.GetRunItemArtifact(ctx, store.GetRunItemArtifactParams{RunID: run.ID, Stage: "expand", ItemKey: item.Key})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, false, nil
	}
	if err != nil {
		return nil, nil, false, fmt.Errorf("read expand item: %w", err)
	}
	if row.State != "done" {
		return nil, nil, false, nil
	}
	art, ok, err := env.Artifacts.OpenRecorded("expand", row.Sha256, run.Snapshot.Recipes["expand"], row.Sha256)
	if err != nil {
		return nil, nil, false, fmt.Errorf("verify expand artifact: %w", err)
	}
	if !ok || filepath.Clean(env.Artifacts.Abs(row.Path)) != filepath.Clean(env.Artifacts.Abs(art.Path)) {
		slog.Warn("invalid expand artifact", "run", run.Country+"/"+run.Rel, "key", item.Key)
		return nil, nil, false, nil
	}
	data := make([][]byte, 2)
	for i, name := range []string{"detail.json", "diagram.json"} {
		data[i], err = os.ReadFile(filepath.Join(env.Artifacts.Abs(art.Path), name))
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil, false, nil
		}
		if err != nil {
			return nil, nil, false, fmt.Errorf("read expand %s: %w", name, err)
		}
	}
	return data[0], data[1], true, nil
}
