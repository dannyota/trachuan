package evaluation

import (
	"encoding/json"
	"strings"
	"testing"

	"danny.vn/trachuan/internal/retrieval"
)

func decisionReport(t *testing.T, mutate func(map[string]any)) []byte {
	t.Helper()
	recipe := retrieval.Recipe3()
	raw, err := recipe.JSON()
	if err != nil {
		t.Fatal(err)
	}
	var recipeValue any
	if err = json.Unmarshal(raw, &recipeValue); err != nil {
		t.Fatal(err)
	}
	report := map[string]any{"version": 2, "status": "complete", "country": "vn", "rel": "20990101_1", "retrieval_recipe": recipeValue, "recipes": map[string]string{"retrieval": recipe.Hash()}}
	if mutate != nil {
		mutate(report)
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestParseDecisionReport_AcceptsCompleteVersion2And3(t *testing.T) {
	for _, version := range []int{2, 3} {
		report, err := ParseDecisionReport(decisionReport(t, func(r map[string]any) { r["version"] = version }))
		if err != nil {
			t.Fatal(err)
		}
		if report.Version != version || report.Country != "vn" || report.Rel != "20990101_1" {
			t.Fatalf("report = %d %s/%s", report.Version, report.Country, report.Rel)
		}
	}
}

func TestParseDecisionReport_Refuses(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(map[string]any)
		want   string
	}{
		{"version 1", func(r map[string]any) { r["version"] = 1 }, "want version 2 or 3"},
		{"version 4", func(r map[string]any) { r["version"] = 4 }, "want version 2 or 3"},
		{"version 3 recipe missing", func(r map[string]any) { r["version"] = 3; delete(r, "retrieval_recipe") }, "retrieval recipe missing"},
		{"historical policy", func(r map[string]any) { r["historical_policy"] = "agent" }, "historical_policy"},
		{"recipe missing", func(r map[string]any) { delete(r, "retrieval_recipe") }, "retrieval recipe missing"},
		{"recipe hash differs", func(r map[string]any) { r["recipes"] = map[string]string{"retrieval": "other"} }, "hash differs"},
		{"incomplete", func(r map[string]any) { r["status"] = "incomplete" }, "not complete"},
		{"research merge", func(r map[string]any) { r["merge_siblings"] = true }, "sibling merge"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ParseDecisionReport(decisionReport(t, tc.mutate)); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
	if _, err := ParseDecisionReport([]byte("{")); err == nil {
		t.Fatal("malformed report accepted")
	}
}

func TestComparedRelease_ReadsPreviousDiff(t *testing.T) {
	sum := strings.Repeat("a", 64)
	for _, tc := range []struct {
		name string
		diff any
		want *ComparedRelease
	}{
		{"none", nil, nil},
		{"reason only", map[string]any{"reason": "no previous report"}, nil},
		{"in memory", map[string]any{"previous": map[string]string{"country": "vn", "rel": "20990101_1", "report_sha256": sum}}, &ComparedRelease{"vn", "20990101_1", sum}},
		{"decoded", map[string]any{"previous": map[string]any{"country": "vn", "rel": "20990101_1", "report_sha256": sum}}, &ComparedRelease{"vn", "20990101_1", sum}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Report{PreviousDiff: tc.diff}.ComparedRelease()
			if err != nil {
				t.Fatal(err)
			}
			if (got == nil) != (tc.want == nil) || (got != nil && *got != *tc.want) {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
	if _, err := (Report{PreviousDiff: map[string]any{"previous": "bad"}}).ComparedRelease(); err == nil {
		t.Fatal("malformed previous accepted")
	}
}
