package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/run"
	"danny.vn/trachuan/apps/corpus/internal/store"
	failurepb "go.temporal.io/api/failure/v1"
	workflowpb "go.temporal.io/api/workflow/v1"
)

func TestStatus_ContainsFetchReads(t *testing.T) {
	result := struct {
		run.Report
		PausedSources []store.RunSource `json:"paused_sources"`
	}{}
	data, err := json.Marshal(result)
	if err != nil || !strings.Contains(string(data), `"fetch_reads":{"baseline_after_unavailable":0`) || !strings.Contains(string(data), `"reused":0`) || !strings.Contains(string(data), `"control_changed":0`) {
		t.Fatalf("status %s: %v", data, err)
	}
}

func TestStatus_ContainsPendingActivityFailure(t *testing.T) {
	result := struct {
		run.Report
		PausedSources []store.RunSource `json:"paused_sources"`
	}{Report: run.Report{PendingActivities: []*workflowpb.PendingActivityInfo{{LastFailure: &failurepb.Failure{Message: "1 items pending after retryable errors"}}}}}
	data, err := json.Marshal(result)
	if err != nil || !strings.Contains(string(data), `"last_failure":{"message":"1 items pending after retryable errors"`) {
		t.Fatalf("status %s: %v", data, err)
	}
}
