package classify

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRequest_CarriesDocumentAndFamilyMenu(t *testing.T) {
	r, _ := NewRecipe("claude-haiku-5-5", strings.Repeat("a", 64))
	d := Document{Key: "vn:vbpl:1", Title: "Synthetic title", Scope: "Synthetic scope", Contents: []string{"Chapter one", "Article one"}, Fields: []string{"Synthetic field"}}
	p, err := BuildRequest(r, d, []Family{{Code: "1.1", Name: "Synthetic family", ParentName: "Synthetic topic"}})
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{d.Title, d.Scope, d.Fields[0], d.Contents[0], "1.1", "Synthetic family", "Synthetic topic", "at most 5", "adaptive"} {
		if !strings.Contains(string(b), s) {
			t.Fatalf("missing %s", s)
		}
	}
	for _, s := range []string{"maxItems", "minItems"} {
		if strings.Contains(string(b), s) {
			t.Fatalf("structured output schema carries unsupported %s", s)
		}
	}
}

func TestRequest_AllowsRecipeFamilyAndEffortChanges(t *testing.T) {
	for _, setting := range []string{"families", "effort"} {
		t.Run(setting, func(t *testing.T) {
			r, _ := NewRecipe("claude-haiku-5-5", strings.Repeat("a", 64))
			if setting == "families" {
				r.F = 6
			} else {
				r.Effort = "low"
			}
			if _, err := BuildRequest(r, documents()[0], families()); err != nil {
				t.Fatal("valid recipe setting was refused", err)
			}
		})
	}
}

func TestValidate_FamiliesAndFailures(t *testing.T) {
	families := []Family{{Code: "1.1"}, {Code: "2.1"}}
	for _, tc := range []struct {
		name, stop, text, want string
		kept, dropped          int
	}{
		{"valid", "end_turn", `{"codes":["1.1","retired","2.1","1.1"]}`, "ok", 2, 1},
		{"empty", "end_turn", `{"codes":[]}`, "no_family", 0, 0},
		{"unknown", "end_turn", `{"codes":["retired"]}`, "no_family", 0, 1},
		{"bound", "end_turn", `{"codes":["1.1","1.1","1.1","1.1","1.1","1.1"]}`, "invalid_response", 0, 0},
		{"truncated", "max_tokens", `{"codes":["1.1"]}`, "invalid_response", 0, 0},
		{"refusal", "refusal", `{"codes":["1.1"]}`, "invalid_response", 0, 0},
		{"wrong schema", "end_turn", `{"codes":[3]}`, "invalid_response", 0, 0},
		{"extra", "end_turn", `{"codes":["1.1"],"other":true}`, "invalid_response", 0, 0},
		{"null", "end_turn", `{"codes":null}`, "invalid_response", 0, 0},
		{"null item", "end_turn", `{"codes":[null]}`, "invalid_response", 0, 0},
		{"missing", "end_turn", `{}`, "invalid_response", 0, 0},
		{"wrong case", "end_turn", `{"Codes":["1.1"]}`, "invalid_response", 0, 0},
		{"duplicate field", "end_turn", `{"codes":[],"codes":["1.1"]}`, "invalid_response", 0, 0},
		{"trailing", "end_turn", `{"codes":[]} {}`, "invalid_response", 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := Validate(tc.stop, tc.text, 5, families)
			if got.Outcome != tc.want || len(got.Codes) != tc.kept || len(got.Dropped) != tc.dropped {
				t.Fatalf("result %+v", got)
			}
		})
	}
}
