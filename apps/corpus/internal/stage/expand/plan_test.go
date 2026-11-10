package expand

import (
	"context"
	"encoding/json"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/membership"
)

func TestExpand_ContextNeverSeeds(t *testing.T) {
	entries := []membership.Entry{{DocumentKey: "vn:vbpl:1", Source: "vbpl", SourceID: "1", Class: "primary", Disposition: "active"}, {DocumentKey: "vn:vbpl:2", Source: "vbpl", SourceID: "2", Class: "relation_context", Disposition: "active"}, {DocumentKey: "vn:vbpl:3", Source: "vbpl", SourceID: "3", Class: "primary", Disposition: "withdrawn"}, {DocumentKey: "vn:vbpl:VBPQTA_4", Source: "vbpl", SourceID: "VBPQTA_4", Class: "primary", Disposition: "active"}}
	got := seedItems(entries)
	if len(got) != 1 || got[0].Key != "vn:vbpl:1" {
		t.Fatalf("items = %#v", got)
	}
}

func TestExpand_SelectionVersionZeroPlansNothing(t *testing.T) {
	run := stage.Run{Snapshot: stage.Snapshot{SourceList: json.RawMessage(`{"vbpl":{"selection_version":0}}`)}}
	defer func() {
		if v := recover(); v != nil {
			t.Errorf("legacy plan or finalize accessed scope: %v", v)
		}
	}()
	items, err := New(nil).Plan(context.Background(), stage.Env{}, run)
	if err != nil || len(items) != 0 {
		t.Fatalf("items=%v err=%v", items, err)
	}
	_, err = New(nil).Finalize(context.Background(), stage.Env{}, nil, run)
	if err != nil {
		t.Fatalf("finalize: %v", err)
	}
}
