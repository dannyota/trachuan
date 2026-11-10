package workflow

import (
	"encoding/json"
	"testing"
)

func TestClose_FrozenRunWithoutExpandSkipsIt(t *testing.T) {
	snapshot := json.RawMessage(`{"stages":["discover","fetch","extract"]}`)
	next, err := frozenFollowing(snapshot, "discover")
	if err != nil || next != "fetch" {
		t.Fatalf("next=%q err=%v", next, err)
	}
}

func TestClose_FrozenRunEndsAndRejectsUnknownStage(t *testing.T) {
	snapshot := json.RawMessage(`{"stages":["discover","fetch"]}`)
	next, err := frozenFollowing(snapshot, "fetch")
	if err != nil || next != "" {
		t.Fatalf("next=%q err=%v", next, err)
	}
	if _, err = frozenFollowing(snapshot, "expand"); err == nil {
		t.Fatal("accepted a stage absent from the snapshot")
	}
	if _, err = frozenFollowing(json.RawMessage(`{`), "fetch"); err == nil {
		t.Fatal("accepted an invalid snapshot")
	}
}
