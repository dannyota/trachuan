package evidence

import (
	"context"
	"sync"

	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/store"
)

func lazyArtifactDirs(ctx context.Context, q func() store.Querier, env stage.Env, run stage.Run) func(string, string) (string, bool, error) {
	var once sync.Once
	var inv inventory
	var loadErr error
	return func(stageName, itemKey string) (string, bool, error) {
		once.Do(func() { inv, loadErr = readInventory(ctx, q(), run.ID) })
		if loadErr != nil {
			return "", false, loadErr
		}
		path, _, ok := inv.doneArtifact(stageName, itemKey)
		if !ok {
			return "", false, nil
		}
		return env.Artifacts.Abs(path), true, nil
	}
}

func lazyBuild(ctx context.Context, q store.Querier, env stage.Env, run stage.Run) func() (string, string, error) {
	return func() (string, string, error) {
		inv, err := readInventory(ctx, q, run.ID)
		if err != nil {
			return "", "", err
		}
		path, sum, ok := inv.doneArtifact("build", itemKey(run))
		if !ok {
			return "", "", refuse("run has no done build item")
		}
		return env.Artifacts.Abs(path), sum, nil
	}
}
