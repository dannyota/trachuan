//go:build integration

package worker

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dannyota/traquydinh/apps/corpus/internal/artifact"
	"github.com/dannyota/traquydinh/apps/corpus/internal/migrate"
	"github.com/dannyota/traquydinh/apps/corpus/internal/run"
	"github.com/dannyota/traquydinh/apps/corpus/internal/stage"
	"github.com/dannyota/traquydinh/apps/corpus/internal/store"
	"github.com/dannyota/traquydinh/apps/corpus/internal/testdb"
	"github.com/dannyota/traquydinh/apps/corpus/internal/workflow"
	"github.com/jackc/pgx/v5"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/testsuite"
)

type fakeStage struct{}

func (fakeStage) Name() string                 { return "fake" }
func (fakeStage) Recipe(stage.Snapshot) string { return stage.RecipeHash("fake", "v1") }
func (fakeStage) Plan(context.Context, stage.Env, stage.Run) ([]stage.Item, error) {
	return []stage.Item{{Key: "one", InputHash: stage.RecipeHash("input", "one")}, {Key: "two", InputHash: stage.RecipeHash("input", "two")}}, nil
}
func (s fakeStage) Process(ctx context.Context, env stage.Env, release stage.Run, item stage.Item) (stage.Output, error) {
	result, err := env.Artifacts.Publish(ctx, s.Name(), item.InputHash, s.Recipe(release.Snapshot), func(dir string) error {
		return os.WriteFile(filepath.Join(dir, "data"), []byte(item.Key), 0600)
	})
	if err != nil {
		return stage.Output{}, err
	}
	return stage.Output{Artifact: &result}, nil
}
func (fakeStage) Rows(context.Context, stage.Env, artifact.Result) (func(context.Context, pgx.Tx, int64) error, error) {
	return nil, nil
}
func (fakeStage) Resolve(context.Context, stage.Env, stage.Run, stage.Item, artifact.Result) (stage.Resolution, error) {
	return stage.Resolution{}, nil
}

func TestWorker_RunsRegisteredStageEndToEnd(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	url := testdb.New(t)
	if err := migrate.Up(ctx, url); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	server, err := testsuite.StartDevServer(ctx, testsuite.DevServerOptions{ClientOptions: &client.Options{Namespace: "corpus"}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if stopErr := server.Stop(); stopErr != nil {
			t.Error(stopErr)
		}
	}()
	registry := workflow.NewRegistry(fakeStage{})
	env := stage.Env{DB: db, Artifacts: artifact.Store{Root: t.TempDir()}, Now: time.Now}
	w, err := New(ctx, Deps{Env: env, Temporal: server.Client(), Registry: registry})
	if err != nil {
		t.Fatal(err)
	}
	workerCtx, stop := context.WithCancel(ctx)
	workerDone := make(chan error, 1)
	go func() { workerDone <- w.Run(workerCtx) }()
	defer func() {
		stop()
		if runErr := <-workerDone; runErr != nil {
			t.Error(runErr)
		}
	}()
	if err := run.Start(ctx, run.Deps{DB: db, Temporal: server.Client(), Stages: registry.Names(), RecipesFor: registry.Recipes, SourceList: []byte(`{}`)}, "vn", "20261109", "", ""); err != nil {
		t.Fatal(err)
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		var state string
		if err := db.QueryRow(ctx, `SELECT state FROM run`).Scan(&state); err != nil {
			t.Fatal(err)
		}
		if state == "paused" {
			break
		}
		if state == "failed" {
			t.Fatal("run failed")
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-ticker.C:
		}
	}
	var done, artifacts int
	if err := db.QueryRow(ctx, `SELECT (SELECT count(*) FROM run_item WHERE state='done'), (SELECT count(*) FROM artifact)`).Scan(&done, &artifacts); err != nil {
		t.Fatal(err)
	}
	if done != 2 || artifacts != 2 {
		t.Fatalf("done = %d, artifacts = %d", done, artifacts)
	}
}
