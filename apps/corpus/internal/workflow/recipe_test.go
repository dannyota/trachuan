package workflow

import (
	"errors"
	"fmt"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/detector"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"go.temporal.io/sdk/temporal"
)

func TestActivityError_RecipeRefusalIsNonRetryable(t *testing.T) {
	err := activityError(&stage.RecipeError{Stage: "extract"})
	var application *temporal.ApplicationError
	if !errors.As(err, &application) || !application.NonRetryable() || application.Type() != "recipe_mismatch" {
		t.Fatalf("err=%v", err)
	}
}

func TestActivityError_UnregisteredGapIsNonRetryable(t *testing.T) {
	err := activityError(fmt.Errorf("validate gap: %w", detector.ValidateKind("future_gap")))
	var application *temporal.ApplicationError
	if !errors.As(err, &application) || !application.NonRetryable() || application.Type() != "unregistered_gap" || !errors.Is(err, detector.ErrUnregisteredGap) {
		t.Fatalf("err=%v", err)
	}
}

func TestActivityError_MissingDecisionIsNonRetryable(t *testing.T) {
	err := activityError(fmt.Errorf("plan: %w", stage.RequireDecision("vn", "20990101", stage.Snapshot{})))
	var application *temporal.ApplicationError
	if !errors.As(err, &application) || !application.NonRetryable() || application.Type() != "no_previous_report_decision" || !errors.Is(err, stage.ErrNoDecision) {
		t.Fatalf("err=%v", err)
	}
}
