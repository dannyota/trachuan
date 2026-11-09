package workflow

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/run"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"github.com/stretchr/testify/mock"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/testsuite"
	temporalworkflow "go.temporal.io/sdk/workflow"
)

func workflowTest(t *testing.T) *testsuite.TestWorkflowEnvironment {
	t.Helper()
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(BuildRelease)
	a := &Activities{}
	env.RegisterActivity(a)
	env.RegisterActivityWithOptions(func(context.Context, stage.BatchInput) (stage.BatchResult, error) { return stage.BatchResult{}, nil }, activity.RegisterOptions{Name: "fetch"})
	return env
}

func testInput() run.BuildInput {
	return run.BuildInput{RunID: 1, Generation: 1, Stage: "fetch", HistoryLimit: 5000}
}
func markMock(env *testsuite.TestWorkflowEnvironment, to string) *testsuite.MockCallWrapper {
	return markMockResult(env, to, true)
}
func markMockResult(env *testsuite.TestWorkflowEnvironment, to string, result bool) *testsuite.MockCallWrapper {
	return env.OnActivity("MarkRun", mock.Anything, mock.MatchedBy(func(in stage.MarkInput) bool { return in.To == to })).Return(result, nil).Once()
}
func planMock(env *testsuite.TestWorkflowEnvironment, result stage.PlanResult) {
	env.OnActivity("PlanStage", mock.Anything, mock.Anything).Return(result, nil).Once()
}
func finishMock(env *testsuite.TestWorkflowEnvironment, result stage.CloseResult) {
	env.OnActivity("CloseStage", mock.Anything, mock.Anything).Return(result, nil).Once()
	env.OnActivity("Checkpoint", mock.Anything, mock.Anything).Return("", nil).Once()
}
func TestBuildRelease_BeginMarksRunning(t *testing.T) {
	env := workflowTest(t)
	in := testInput()
	in.Begin = true
	markMock(env, "running")
	planMock(env, stage.PlanResult{Cancelling: true})
	markMock(env, "cancelled")
	env.ExecuteWorkflow(BuildRelease, in)
	if err := env.GetWorkflowError(); err != nil {
		t.Fatal(err)
	}
	env.AssertExpectations(t)
}
func TestBuildRelease_ContinuesAsNewWithNextStage(t *testing.T) {
	env := workflowTest(t)
	planMock(env, stage.PlanResult{})
	finishMock(env, stage.CloseResult{NextStage: "extract"})
	env.ExecuteWorkflow(BuildRelease, testInput())
	if !env.IsWorkflowCompleted() || !temporalworkflow.IsContinueAsNewError(env.GetWorkflowError()) {
		t.Fatalf("error: %v", env.GetWorkflowError())
	}
	env.AssertExpectations(t)
}
func TestBuildRelease_StopsAtUntilAndMarksPaused(t *testing.T) {
	env := workflowTest(t)
	in := testInput()
	in.Until = in.Stage
	planMock(env, stage.PlanResult{})
	finishMock(env, stage.CloseResult{NextStage: "extract"})
	markMock(env, "paused")
	env.ExecuteWorkflow(BuildRelease, in)
	if err := env.GetWorkflowError(); err != nil {
		t.Fatal(err)
	}
	env.AssertExpectations(t)
}
func TestBuildRelease_DrainsBatchesThenCancels(t *testing.T) {
	env := workflowTest(t)
	planMock(env, stage.PlanResult{Batches: []int{0, 1}})
	env.OnActivity("fetch", mock.Anything, mock.Anything).Return(stage.BatchResult{Cancelling: true}, nil).Once()
	env.OnActivity("fetch", mock.Anything, mock.Anything).Return(stage.BatchResult{}, nil).Once()
	markMock(env, "cancelled")
	env.ExecuteWorkflow(BuildRelease, testInput())
	if err := env.GetWorkflowError(); err != nil {
		t.Fatal(err)
	}
	env.AssertExpectations(t)
}
func TestBuildRelease_CancelledBeforePlanning(t *testing.T) {
	env := workflowTest(t)
	planMock(env, stage.PlanResult{Cancelling: true})
	markMock(env, "cancelled")
	env.ExecuteWorkflow(BuildRelease, testInput())
	if err := env.GetWorkflowError(); err != nil {
		t.Fatal(err)
	}
	env.AssertExpectations(t)
}
func TestBuildRelease_FailedItemsFailRun(t *testing.T) {
	env := workflowTest(t)
	planMock(env, stage.PlanResult{})
	env.OnActivity("CloseStage", mock.Anything, mock.Anything).Return(stage.CloseResult{Failed: true, Counts: stage.Counts{Failed: 2}}, nil).Once()
	markMock(env, "failed")
	env.ExecuteWorkflow(BuildRelease, testInput())
	if err := env.GetWorkflowError(); err != nil {
		t.Fatal(err)
	}
	env.AssertExpectations(t)
}
func TestBuildRelease_SupersededEndsQuietly(t *testing.T) {
	env := workflowTest(t)
	env.OnActivity("PlanStage", mock.Anything, mock.Anything).Return(stage.PlanResult{}, temporal.NewNonRetryableApplicationError("superseded", "superseded", nil)).Once()
	env.ExecuteWorkflow(BuildRelease, testInput())
	if err := env.GetWorkflowError(); err != nil {
		t.Fatal(err)
	}
	env.AssertExpectations(t)
}
func TestBuildRelease_FailsAboveHistoryLimit(t *testing.T) {
	env := workflowTest(t)
	in := testInput()
	in.HistoryLimit = 10
	env.SetCurrentHistoryLength(11)
	markMock(env, "failed")
	env.ExecuteWorkflow(BuildRelease, in)
	if env.GetWorkflowError() == nil {
		t.Fatal("expected history limit error")
	}
	env.AssertExpectations(t)
}
func TestBuildRelease_RunsOnlyListedBatches(t *testing.T) {
	env := workflowTest(t)
	planMock(env, stage.PlanResult{Batches: []int{0, 2}})
	for _, n := range []int{0, 2} {
		env.OnActivity("fetch", mock.Anything, mock.MatchedBy(func(in stage.BatchInput) bool { return in.Batch == n })).Return(stage.BatchResult{}, nil).Once()
	}
	finishMock(env, stage.CloseResult{})
	markMock(env, "paused")
	env.ExecuteWorkflow(BuildRelease, testInput())
	if err := env.GetWorkflowError(); err != nil {
		t.Fatal(err)
	}
	env.AssertExpectations(t)
}
func TestBuildRelease_ActivityErrorMarksRunFailed(t *testing.T) {
	env := workflowTest(t)
	env.OnActivity("PlanStage", mock.Anything, mock.Anything).Return(stage.PlanResult{}, temporal.NewNonRetryableApplicationError("broken", "broken", nil)).Once()
	markMock(env, "failed")
	env.ExecuteWorkflow(BuildRelease, testInput())
	if env.GetWorkflowError() == nil {
		t.Fatal("expected the activity error")
	}
	env.AssertExpectations(t)
}

func TestBuildRelease_EightBatchesStartBeforeAnyFinishes(t *testing.T) {
	env := workflowTest(t)
	planMock(env, stage.PlanResult{Batches: []int{0, 1, 2, 3, 4, 5, 6, 7}})
	var started atomic.Int32
	gate := make(chan struct{})
	env.OnActivity("fetch", mock.Anything, mock.Anything).Return(func(ctx context.Context, _ stage.BatchInput) (stage.BatchResult, error) {
		if started.Add(1) == 8 {
			close(gate)
		}
		select {
		case <-gate:
			return stage.BatchResult{}, nil
		case <-time.After(2 * time.Second):
			return stage.BatchResult{}, context.DeadlineExceeded
		}
	}).Times(8)
	finishMock(env, stage.CloseResult{})
	markMock(env, "paused")
	in := testInput()
	in.BatchesInFlight = 8
	env.ExecuteWorkflow(BuildRelease, in)
	if err := env.GetWorkflowError(); err != nil {
		t.Fatal(err)
	}
	if started.Load() != 8 {
		t.Fatalf("started %d batches", started.Load())
	}
}

func TestBuildRelease_RefusedPauseFinishesCancel(t *testing.T) {
	env := workflowTest(t)
	planMock(env, stage.PlanResult{})
	finishMock(env, stage.CloseResult{})
	markMockResult(env, "paused", false)
	markMock(env, "cancelled")
	env.ExecuteWorkflow(BuildRelease, testInput())
	if err := env.GetWorkflowError(); err != nil {
		t.Fatal(err)
	}
	env.AssertExpectations(t)
}

func TestBuildRelease_RefusedBeginStops(t *testing.T) {
	env := workflowTest(t)
	in := testInput()
	in.Begin = true
	markMockResult(env, "running", false)
	markMock(env, "cancelled")
	env.ExecuteWorkflow(BuildRelease, in)
	if err := env.GetWorkflowError(); err != nil {
		t.Fatal(err)
	}
	env.AssertExpectations(t)
}

func TestBuildRelease_RefusedFailureFinishesCancel(t *testing.T) {
	env := workflowTest(t)
	planMock(env, stage.PlanResult{})
	env.OnActivity("CloseStage", mock.Anything, mock.Anything).Return(stage.CloseResult{Failed: true, Counts: stage.Counts{Failed: 1}}, nil).Once()
	markMockResult(env, "failed", false)
	markMock(env, "cancelled")
	env.ExecuteWorkflow(BuildRelease, testInput())
	if err := env.GetWorkflowError(); err != nil {
		t.Fatal(err)
	}
	env.AssertExpectations(t)
}

func TestBuildRelease_HeartbeatTimeout(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input time.Duration
		want  time.Duration
	}{{"older history", 0, 2 * time.Minute}, {"configured", 2 * time.Second, 2 * time.Second}} {
		t.Run(tc.name, func(t *testing.T) {
			env := workflowTest(t)
			planMock(env, stage.PlanResult{Batches: []int{0}})
			var got time.Duration
			env.OnActivity("fetch", mock.Anything, mock.Anything).Return(func(ctx context.Context, _ stage.BatchInput) (stage.BatchResult, error) {
				got = activity.GetInfo(ctx).HeartbeatTimeout
				return stage.BatchResult{}, nil
			}).Once()
			finishMock(env, stage.CloseResult{})
			markMock(env, "paused")
			in := testInput()
			in.HeartbeatTimeout = tc.input
			env.ExecuteWorkflow(BuildRelease, in)
			if err := env.GetWorkflowError(); err != nil || got != tc.want {
				t.Fatalf("heartbeat timeout = %v, want %v; error = %v", got, tc.want, err)
			}
		})
	}
}

func TestBuildRelease_EvalEndsEvaluated(t *testing.T) {
	env := workflowTest(t)
	in := testInput()
	in.Stage = "eval"
	planMock(env, stage.PlanResult{})
	finishMock(env, stage.CloseResult{})
	markMock(env, "evaluated")
	env.ExecuteWorkflow(BuildRelease, in)
	if err := env.GetWorkflowError(); err != nil {
		t.Fatal(err)
	}
	env.AssertExpectations(t)
}
