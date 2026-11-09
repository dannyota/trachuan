// Package worker runs corpus workflows and activities.
package worker

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"time"

	"github.com/dannyota/traquydinh/apps/corpus/internal/artifact"
	"github.com/dannyota/traquydinh/apps/corpus/internal/checkpoint"
	"github.com/dannyota/traquydinh/apps/corpus/internal/config"
	"github.com/dannyota/traquydinh/apps/corpus/internal/run"
	"github.com/dannyota/traquydinh/apps/corpus/internal/stage"
	"github.com/dannyota/traquydinh/apps/corpus/internal/store"
	"github.com/dannyota/traquydinh/apps/corpus/internal/vbpl"
	"github.com/dannyota/traquydinh/apps/corpus/internal/workflow"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	temporalworker "go.temporal.io/sdk/worker"
	temporalworkflow "go.temporal.io/sdk/workflow"
)

// Deps supplies the worker's clients and registered stages.
type Deps struct {
	Env          stage.Env
	Temporal     client.Client
	Registry     workflow.Registry
	Checkpointer workflow.Checkpointer
}

// Worker runs a registered Temporal worker.
type Worker struct{ worker temporalworker.Worker }

// New constructs a worker using explicit dependencies.
func New(_ context.Context, deps Deps) (*Worker, error) {
	if deps.Env.DB == nil || deps.Temporal == nil {
		return nil, errors.New("worker requires a workshop pool and Temporal client")
	}
	w := temporalworker.New(deps.Temporal, run.TaskQueue, temporalworker.Options{})
	w.RegisterWorkflowWithOptions(workflow.BuildRelease, temporalworkflow.RegisterOptions{Name: run.WorkflowName})
	checkpointer := deps.Checkpointer
	if checkpointer == nil {
		checkpointer = workflow.NoCheckpoint{}
	}
	activities := &workflow.Activities{Env: deps.Env, Registry: deps.Registry, Checkpointer: checkpointer, Batch: stage.BatchConfig{HeartbeatEvery: 20 * time.Second, MaxAttempts: 5}}
	w.RegisterActivity(activities)
	for _, st := range deps.Registry.Stages() {
		w.RegisterActivityWithOptions(activities.RunBatch, activity.RegisterOptions{Name: st.Name()})
	}
	return &Worker{worker: w}, nil
}

// Run starts the worker and stops it when the context is cancelled.
func (w *Worker) Run(ctx context.Context) error {
	if err := w.worker.Start(); err != nil {
		return fmt.Errorf("start worker: %w", err)
	}
	defer w.worker.Stop()
	<-ctx.Done()
	return nil
}

// Run wires and runs the default corpus worker.
func Run(ctx context.Context, cfg config.Config) error {
	if cfg.CheckpointDir == "" {
		return errors.New("CORPUS_CHECKPOINT_DIR is required")
	}
	if err := checkpoint.CheckVersion(ctx, cfg.PgDump); err != nil {
		return fmt.Errorf("check pg_dump: %w", err)
	}
	db, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("open workshop: %w", err)
	}
	defer db.Close()
	temporalClient, err := client.Dial(client.Options{HostPort: cfg.TemporalAddress, Namespace: cfg.Namespace})
	if err != nil {
		return fmt.Errorf("connect Temporal: %w", err)
	}
	defer temporalClient.Close()
	env := stage.Env{DB: db, Artifacts: artifact.Store{Root: cfg.Artifacts}, Now: time.Now}
	w, err := New(ctx, Deps{Env: env, Temporal: temporalClient, Registry: workflow.Default(workflow.Deps{Env: env, VBPL: vbpl.NewWithBase(cfg.VBPLURL, workerVersion())}), Checkpointer: checkpoint.Taker{Config: checkpoint.Config{Dir: cfg.CheckpointDir, ArtifactsRoot: cfg.Artifacts, DatabaseURL: cfg.DatabaseURL, PgDump: cfg.PgDump, Keep: 3}}})
	if err != nil {
		return fmt.Errorf("construct worker: %w", err)
	}
	return w.Run(ctx)
}

func workerVersion() string {
	info, ok := debug.ReadBuildInfo()
	if ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" && setting.Value != "" {
				return setting.Value
			}
		}
	}
	return "unknown"
}
