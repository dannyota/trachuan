package evaluation

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/internal/retrieval"
)

func TestRun_RejectsLegacySiblingMergePreviousReport(t *testing.T) {
	date := retrieval.Date{Year: 2026, Month: 11, Day: 9}
	baseline, _, err := Run(context.Background(), &fakeEngine{date: date}, recordedSuite(), Options{EvaluationDate: date, Modes: []retrieval.Mode{"exact"}})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(baseline)
	if err != nil {
		t.Fatal(err)
	}
	paths := [][]string{
		{}, {"cases"}, {"presented"}, {"evidence_characters"}, {"floors"}, {"targets"}, {"non_regression"},
		{"cohorts", "old_product", "metrics"}, {"cohorts", "old_product", "floors"}, {"cohorts", "development", "targets"}, {"cohorts", "development", "non_regression"},
		{"filter_gate", "modes"}, {"previous_diff", "cases"}, {"ann_loss", "old_product", "differences"},
	}
	for _, path := range paths {
		name := strings.Join(path, "/")
		if name == "" {
			name = "report"
		}
		for _, enabled := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%t", name, enabled), func(t *testing.T) {
				var root map[string]any
				if err := json.Unmarshal(raw, &root); err != nil {
					t.Fatal(err)
				}
				root["merge_siblings"] = false
				target := root
				for i, key := range path {
					if i == len(path)-1 {
						target[key] = []any{map[string]any{"merge_siblings": enabled}}
						break
					}
					nested, ok := target[key].(map[string]any)
					if !ok {
						nested = map[string]any{}
						target[key] = nested
					}
					target = nested
				}
				if len(path) == 0 {
					root["merge_siblings"] = enabled
				}
				marked, err := json.Marshal(root)
				if err != nil {
					t.Fatal(err)
				}
				previous := filepath.Join(t.TempDir(), "report.json")
				if err = os.WriteFile(previous, marked, 0600); err != nil {
					t.Fatal(err)
				}
				current, _, err := Run(context.Background(), &fakeEngine{date: date}, recordedSuite(), Options{EvaluationDate: date, Modes: []retrieval.Mode{"exact"}, PreviousReport: previous})
				if enabled {
					if err == nil || !strings.Contains(err.Error(), "incompatible previous sibling merge override") {
						t.Fatalf("previous report error %v", err)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				encoded, err := json.Marshal(current)
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(string(encoded), `"merge_siblings":`) {
					t.Fatal("new report wrote legacy marker")
				}
			})
		}
	}
}
