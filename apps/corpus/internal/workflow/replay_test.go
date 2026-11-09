package workflow

import (
	"path/filepath"
	"testing"

	"go.temporal.io/sdk/worker"
)

func TestReplay_Histories(t *testing.T) {
	paths, err := filepath.Glob("testdata/histories/*.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			replayer := worker.NewWorkflowReplayer()
			replayer.RegisterWorkflow(BuildRelease)
			if err := replayer.ReplayWorkflowHistoryFromJSONFile(nil, path); err != nil {
				t.Fatal(err)
			}
		})
	}
}
