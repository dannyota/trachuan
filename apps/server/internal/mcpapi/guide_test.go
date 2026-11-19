package mcpapi

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestGuide_PastPeriodSendsInForceOnlyFalse(t *testing.T) {
	out := resultJSON(t, callTool(t, newHarness(t, nil), "guide", map[string]any{"country": "vn"}))
	guide := out["guide"].(string)
	for _, text := range []string{"past period", "in_force_only:false", "in force today", "in non_current", "abstain:true is advisory and keeps its hits", "judges only hits, not non_current"} {
		if !strings.Contains(guide, text) {
			t.Errorf("guide missing %q", text)
		}
	}
}

func TestSearchSchema_InForceOnlyIsBoolean(t *testing.T) {
	h := newHarness(t, nil)
	listed, err := h.session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range listed.Tools {
		if tool.Name != "search" {
			continue
		}
		raw, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Fatal(err)
		}
		var schema struct {
			Properties map[string]struct {
				Type any `json:"type"`
			} `json:"properties"`
		}
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Fatal(err)
		}
		prop, ok := schema.Properties["in_force_only"]
		if !ok {
			t.Fatal("search schema lacks in_force_only")
		}
		if types, isList := prop.Type.([]any); isList {
			for _, v := range types {
				if v == "boolean" {
					return
				}
			}
		} else if prop.Type == "boolean" {
			return
		}
		t.Fatalf("in_force_only type = %v", prop.Type)
	}
	t.Fatal("search tool not listed")
}
