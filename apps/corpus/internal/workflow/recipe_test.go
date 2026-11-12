package workflow

import (
	"errors"
	"testing"

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
