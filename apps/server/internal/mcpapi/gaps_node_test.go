package mcpapi

import (
	"encoding/json"
	"testing"

	"danny.vn/trachuan/internal/retrieval"
)

func TestGaps_NodeInputAndError(t *testing.T) {
	var in gapsInput
	if err := json.Unmarshal([]byte(`{"country":"vn","node":"1.1"}`), &in); err != nil || in.Node != "1.1" {
		t.Fatalf("input=%+v error=%v", in, err)
	}
	h := newHarness(t, nil)
	h.engine.err = retrieval.ErrInvalidRequest
	wantError(t, callTool(t, h, "quality_gaps", map[string]any{"country": "vn", "node": "999"}), "invalid_request")
}
