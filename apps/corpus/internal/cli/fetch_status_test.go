package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/run"
	"danny.vn/trachuan/apps/corpus/internal/store"
)

func TestStatus_ContainsFetchReads(t *testing.T) {
	result := struct {
		run.Report
		PausedSources []store.RunSource `json:"paused_sources"`
	}{}
	data, err := json.Marshal(result)
	if err != nil || !strings.Contains(string(data), `"fetch_reads":{"reused":0`) || !strings.Contains(string(data), `"control_changed":0`) {
		t.Fatalf("status %s: %v", data, err)
	}
}
