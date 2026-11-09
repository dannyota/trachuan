package workflow

import (
	"errors"
	"fmt"
	"time"

	"github.com/dannyota/traquydinh/apps/corpus/internal/run"
	"github.com/dannyota/traquydinh/apps/corpus/internal/stage"
	"go.temporal.io/sdk/temporal"
	temporalworkflow "go.temporal.io/sdk/workflow"
)

// errHistoryLimit reports a run already marked failed for its history size.
var errHistoryLimit = errors.New("history limit exceeded")

func superseded(err error) bool {
	var appErr *temporal.ApplicationError
	return errors.As(err, &appErr) && appErr.Type() == "superseded"
}

// BuildRelease runs one stage and continues with the next stage in a new history.
func BuildRelease(ctx temporalworkflow.Context, in run.BuildInput) error {
	retry := &temporal.RetryPolicy{InitialInterval: 30 * time.Second, BackoffCoefficient: 2, MaximumInterval: 10 * time.Minute, MaximumAttempts: 5, NonRetryableErrorTypes: []string{"superseded"}}
	control := temporalworkflow.WithActivityOptions(ctx, temporalworkflow.ActivityOptions{StartToCloseTimeout: 10 * time.Minute, RetryPolicy: retry})
	batch := temporalworkflow.WithActivityOptions(ctx, temporalworkflow.ActivityOptions{StartToCloseTimeout: time.Hour, HeartbeatTimeout: 2 * time.Minute, RetryPolicy: retry})
	input := stage.StageInput{RunID: in.RunID, Generation: in.Generation, Stage: in.Stage}
	guard := func() error {
		if temporalworkflow.GetInfo(ctx).GetCurrentHistoryLength() <= in.HistoryLimit {
			return nil
		}
		message := fmt.Sprintf("history limit exceeded in %s", in.Stage)
		if err := temporalworkflow.ExecuteActivity(control, "MarkRun", stage.MarkInput{RunID: in.RunID, Generation: in.Generation, From: []string{"starting", "running", "cancelling"}, To: "failed", Message: message}).Get(control, nil); err != nil {
			return err
		}
		return fmt.Errorf("%w in %s", errHistoryLimit, in.Stage)
	}
	execute := func(name string, arg, result any) error {
		if err := guard(); err != nil {
			return err
		}
		return temporalworkflow.ExecuteActivity(control, name, arg).Get(control, result)
	}
	finish := func(err error) error {
		if err == nil || superseded(err) {
			return nil
		}
		if errors.Is(err, errHistoryLimit) {
			return err
		}
		// Record the failure on the run row so status never shows a dead run as running.
		mark := stage.MarkInput{RunID: in.RunID, Generation: in.Generation, From: []string{"starting", "running", "cancelling"}, To: "failed", Message: err.Error()}
		_ = temporalworkflow.ExecuteActivity(control, "MarkRun", mark).Get(control, nil)
		return err
	}
	change := func(from, to, message string) error {
		return execute("MarkRun", stage.MarkInput{RunID: in.RunID, Generation: in.Generation, From: []string{from}, To: to, Message: message}, nil)
	}
	if in.Begin {
		if err := change("starting", "running", ""); err != nil {
			return finish(err)
		}
	}
	var plan stage.PlanResult
	if err := execute("PlanStage", input, &plan); err != nil {
		return finish(err)
	}
	if plan.Cancelling {
		return finish(change("cancelling", "cancelled", ""))
	}
	limit := in.BatchesInFlight
	if limit <= 0 {
		limit = 2
	}
	next, active := 0, 0
	cancelling := false
	var batchErr error
	historyExceeded := false
	selector := temporalworkflow.NewSelector(ctx)
	for next < len(plan.Batches) || active > 0 {
		for next < len(plan.Batches) && active < limit && !cancelling && batchErr == nil {
			if temporalworkflow.GetInfo(ctx).GetCurrentHistoryLength() > in.HistoryLimit {
				historyExceeded = true
				break
			}
			future := temporalworkflow.ExecuteActivity(batch, in.Stage, stage.BatchInput{RunID: in.RunID, Generation: in.Generation, Stage: in.Stage, Batch: plan.Batches[next]})
			next++
			active++
			selector.AddFuture(future, func(f temporalworkflow.Future) {
				var result stage.BatchResult
				err := f.Get(batch, &result)
				if err != nil && (batchErr == nil || superseded(err)) {
					batchErr = err
				}
				if result.Cancelling {
					cancelling = true
				}
				active--
			})
		}
		if active == 0 {
			break
		}
		selector.Select(ctx)
	}
	if batchErr != nil {
		return finish(batchErr)
	}
	if historyExceeded {
		return finish(guard())
	}
	if cancelling {
		return finish(change("cancelling", "cancelled", ""))
	}
	var closed stage.CloseResult
	if err := execute("CloseStage", input, &closed); err != nil {
		return finish(err)
	}
	if closed.Failed {
		return finish(change("running", "failed", fmt.Sprintf("%d items failed in %s", closed.Counts.Failed, in.Stage)))
	}
	if err := execute("Checkpoint", input, nil); err != nil {
		return finish(err)
	}
	if in.Until == in.Stage || closed.NextStage == "" {
		return finish(change("running", "paused", ""))
	}
	in.Stage = closed.NextStage
	in.Begin = false
	return temporalworkflow.NewContinueAsNewError(ctx, BuildRelease, in)
}
