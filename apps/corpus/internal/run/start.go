package run

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"runtime/debug"
	"time"

	"danny.vn/traquydinh/apps/corpus/internal/stage"
	"danny.vn/traquydinh/apps/corpus/internal/store"
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/sdk/client"
)

func workflowOpen(ctx context.Context, d Deps, id string) (bool, error) {
	desc, err := d.Temporal.DescribeWorkflowExecution(ctx, id, "")
	if err != nil {
		var missing *serviceerror.NotFound
		if errors.As(err, &missing) {
			return false, nil
		}
		return false, fmt.Errorf("describe workflow: %w", err)
	}
	return desc.WorkflowExecutionInfo.GetStatus() == enumspb.WORKFLOW_EXECUTION_STATUS_RUNNING, nil
}

// Start starts or resumes a release workflow after fencing earlier generations.
func Start(ctx context.Context, d Deps, cc, rel, until, previousManifest string) error {
	if !regexp.MustCompile(`^[a-z]{2}$`).MatchString(cc) {
		return fmt.Errorf("invalid country %q", cc)
	}
	if err := validateRel(rel); err != nil {
		return err
	}
	if len(d.Stages) == 0 {
		return errors.New("no compiled stages")
	}
	if until != "" && !containsStage(d.Stages, until) {
		return fmt.Errorf("unknown until stage %q", until)
	}
	if d.RecipesFor == nil {
		return errors.New("recipe function is required")
	}
	if !json.Valid(d.SourceList) {
		return errors.New("invalid source list JSON")
	}
	sum := sha256.Sum256(d.SourceList)
	version := workerVersion()
	snap := stage.Snapshot{SourceList: d.SourceList, SourceListHash: hex.EncodeToString(sum[:]), Stages: d.Stages, TaxonomyVersion: "none", WorkerVersion: version, PreviousManifest: previousManifest}
	raw, err := json.Marshal(snap)
	if err != nil {
		return fmt.Errorf("encode snapshot: %w", err)
	}
	tx, err := d.DB.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin start: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := store.New(tx)
	if err = q.CreateRun(ctx, store.CreateRunParams{Country: cc, Rel: rel, Snapshot: raw}); err != nil {
		return fmt.Errorf("create run: %w", err)
	}
	row, err := q.GetRunForUpdate(ctx, store.GetRunForUpdateParams{Country: cc, Rel: rel})
	if err != nil {
		return fmt.Errorf("lock run: %w", err)
	}
	if row.State == "evaluated" || row.State == "released" {
		return fmt.Errorf("cannot start %s run", row.State)
	}
	now := time.Now()
	if d.Now != nil {
		now = d.Now()
	}
	if row.State == "starting" && now.Sub(row.UpdatedAt) < 2*time.Minute {
		return errors.New("run is already starting")
	}
	id := WorkflowID(cc, rel)
	open, openErr := workflowOpen(ctx, d, id)
	if openErr != nil {
		return openErr
	}
	if open {
		return errors.New("workflow is open; use corpus cancel or corpus recover")
	}
	oldStageCount := 0
	if row.Generation > 0 {
		snap = stage.Snapshot{}
		if err = json.Unmarshal(row.Snapshot, &snap); err != nil {
			return fmt.Errorf("decode snapshot: %w", err)
		}
		oldStageCount = len(snap.Stages)
		snap, _, err = extendSnapshot(snap, d.Stages, d.RecipesFor(snap))
		if err != nil {
			return err
		}
	} else {
		snap.Recipes = d.RecipesFor(snap)
		for _, name := range d.Stages {
			if _, ok := snap.Recipes[name]; !ok {
				return fmt.Errorf("missing recipe for stage %q", name)
			}
		}
	}
	raw, err = json.Marshal(snap)
	if err != nil {
		return fmt.Errorf("encode snapshot: %w", err)
	}
	next := d.Stages[0]
	if row.NextStage != nil {
		if *row.NextStage == "" {
			if oldStageCount >= len(snap.Stages) {
				return errors.New("run is complete")
			}
			next = snap.Stages[oldStageCount]
		} else {
			next = *row.NextStage
		}
	}
	if !containsStage(d.Stages, next) {
		return fmt.Errorf("unknown next stage %q", next)
	}
	if until != "" {
		nextIndex, untilIndex := -1, -1
		for i, name := range snap.Stages {
			if name == next {
				nextIndex = i
			}
			if name == until {
				untilIndex = i
			}
		}
		if untilIndex < nextIndex {
			return fmt.Errorf("until stage %q is before resume stage %q", until, next)
		}
	}
	var untilPtr *string
	if until != "" {
		untilPtr = &until
	}
	if err = q.StartRun(ctx, store.StartRunParams{ID: row.ID, NextStage: &next, UntilStage: untilPtr, WorkflowID: &id, Snapshot: raw}); err != nil {
		return fmt.Errorf("mark starting: %w", err)
	}
	if err = q.ResetFailedItems(ctx, store.ResetFailedItemsParams{RunID: row.ID, Stage: next}); err != nil {
		return fmt.Errorf("reset failed items: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit start: %w", err)
	}
	taskQueue := d.TaskQueue
	if taskQueue == "" {
		taskQueue = TaskQueue
	}
	_, err = d.Temporal.ExecuteWorkflow(ctx, client.StartWorkflowOptions{ID: id, TaskQueue: taskQueue, WorkflowIDConflictPolicy: enumspb.WORKFLOW_ID_CONFLICT_POLICY_FAIL, WorkflowExecutionErrorWhenAlreadyStarted: true, WorkflowIDReusePolicy: enumspb.WORKFLOW_ID_REUSE_POLICY_ALLOW_DUPLICATE}, WorkflowName, BuildInput{RunID: row.ID, Country: cc, Rel: rel, Generation: int(row.Generation) + 1, Stage: next, Until: until, Begin: true, HistoryLimit: 5000, BatchesInFlight: d.BatchesInFlight, HeartbeatTimeout: d.HeartbeatTimeout})
	if err != nil {
		var already *serviceerror.WorkflowExecutionAlreadyStarted
		if errors.As(err, &already) {
			return fmt.Errorf("workflow is open; use corpus cancel or corpus recover: %w", err)
		}
		return fmt.Errorf("start workflow: %w", err)
	}
	return nil
}

// workerVersion returns the VCS revision the binary was built from, or "unknown".
func workerVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	version, dirty := "unknown", false
	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" && setting.Value != "" {
			version = setting.Value
		}
		if setting.Key == "vcs.modified" && setting.Value == "true" {
			dirty = true
		}
	}
	if dirty {
		version += "+dirty"
	}
	return version
}

func containsStage(stages []string, name string) bool {
	for _, s := range stages {
		if s == name {
			return true
		}
	}
	return false
}
