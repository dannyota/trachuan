package evaluation

import (
	"encoding/json"
	"testing"

	"danny.vn/trachuan/internal/retrieval"
)

func TestQuality_NonRegressionGating(t *testing.T) {
	for _, tc := range []struct {
		name                        string
		mrr, abstention             float64
		quality                     bool
		mrrStatus, abstentionStatus string
	}{
		{"abstention falls", .8, .7, true, "pass", "below"},
		{"mrr falls", .7, .8, false, "fail", "pass"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			makeReport := func(mrr, abstention float64) Report {
				var metrics []Metric
				for _, mode := range []retrieval.Mode{"exact", "ann"} {
					for _, m := range []struct {
						name  string
						limit int
						value float64
					}{
						{"current_law_precision", 8, 1}, {"recall", 20, .9}, {"mrr", 8, mrr}, {"abstention_balanced_accuracy", 8, abstention},
					} {
						v := m.value
						metrics = append(metrics, Metric{Mode: mode, Arms: "hybrid", Name: m.name, Limit: m.limit, Value: &v})
					}
				}
				return Report{Cohorts: map[string]Cohort{"old_product": {CaseCount: 1, Metrics: metrics}, "development": {CaseCount: 1, Metrics: metrics}}, Rejections: []Rejection{{Passed: true}, {Passed: true}, {Passed: true}}}
			}
			current, previous := makeReport(tc.mrr, tc.abstention), makeReport(.8, .8)
			finishQuality(&current, &previous)
			if current.QualityPass != tc.quality {
				t.Fatalf("quality_pass = %t, want %t", current.QualityPass, tc.quality)
			}
			data, err := json.Marshal(current)
			if err != nil {
				t.Fatal(err)
			}
			type comparison struct {
				Name     string   `json:"name"`
				Status   string   `json:"status"`
				Gating   *bool    `json:"gating"`
				Value    *float64 `json:"value"`
				Previous *float64 `json:"previous_value"`
			}
			var report struct {
				NonRegression []comparison `json:"non_regression"`
				Cohorts       map[string]struct {
					NonRegression []comparison `json:"non_regression"`
				} `json:"cohorts"`
			}
			if err := json.Unmarshal(data, &report); err != nil {
				t.Fatal(err)
			}
			for name, entries := range map[string][]comparison{"top": report.NonRegression, "old_product": report.Cohorts["old_product"].NonRegression, "development": report.Cohorts["development"].NonRegression} {
				if len(entries) != 6 {
					t.Fatalf("%s: got %d comparisons", name, len(entries))
				}
				for _, entry := range entries {
					gating, status, value, prior := true, "pass", .9, .9
					switch entry.Name {
					case "mrr":
						status, value, prior = tc.mrrStatus, tc.mrr, .8
					case "abstention_balanced_accuracy":
						gating, status, value, prior = false, tc.abstentionStatus, tc.abstention, .8
					case "recall":
					default:
						t.Fatalf("unexpected comparison %q", entry.Name)
					}
					if entry.Gating == nil || *entry.Gating != gating || entry.Status != status || entry.Value == nil || *entry.Value != value || entry.Previous == nil || *entry.Previous != prior {
						t.Errorf("%s: unexpected comparison %+v", name, entry)
					}
				}
			}
		})
	}
}
