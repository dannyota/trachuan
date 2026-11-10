//go:build integration

package build

import (
	"context"
	"fmt"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/source"
)

func TestLoad_ConsolidatedStatus(t *testing.T) {
	f := fixture(t)
	docs := []struct {
		id           int
		date, code   string
		consolidated bool
	}{
		{1, "2024-01-01", "", true}, {2, "2024-01-01", "A", false},
		{3, "2024-01-01", "", true}, {4, "2024-01-01", "X", false},
		{5, "2024-01-01", "", true}, {6, "2024-02-01", "A", true},
		{7, "2024-01-01", "", true}, {8, "2024-01-01", "A", false},
		{9, "", "", true}, {10, "2024-01-01", "A", true},
		{11, "2024-01-01", "", true}, {12, "2024-01-01", "A", true},
		{13, "2024-01-01", "", true}, {14, "2024-01-01", "", true},
		{15, "2024-01-01", "", true}, {16, "2024-01-01", "A", false},
		{17, "2024-01-01", "", true}, {18, "2024-01-01", "X", false},
		{19, "2024-01-01", "", true}, {20, "2024-01-01", "A", false}, {21, "2024-02-01", "A", true},
	}
	fetches := map[int]string{}
	for _, d := range docs {
		key := fmt.Sprintf("vn:vbpl:%d", d.id)
		data := map[string]any{"id": fmt.Sprint(d.id), "docNum": fmt.Sprintf("%d/2024/TT", d.id), "issueDate": d.date, "isConsolidatedDocument": d.consolidated}
		if d.code != "" {
			data["effStatus"] = map[string]any{"code": d.code}
		}
		art := f.addArtifact(t, "fetch", key, map[string]any{"detail.json": map[string]any{"data": data}, "files.json": []any{}})
		fetches[d.id] = art.SHA256
	}
	for sourceID, targetID := range map[int]int{1: 2, 3: 4, 5: 2, 6: 2, 9: 8, 10: 8, 11: 4, 12: 4, 13: 14, 15: 16, 17: 18, 19: 20, 21: 20} {
		key := fmt.Sprintf("vn:vbpl:%d", sourceID)
		f.addArtifact(t, "relate", key, map[string]any{"relations.json": []map[string]any{{"ordinal": 0, "kind": "consolidates", "target_vbpl_id": fmt.Sprint(targetID)}}}, fetches[sourceID])
	}
	codes := map[string]source.StatusCode{"A": {Class: "in_force", Current: true}, "X": {Class: "expired"}}
	if _, err := Load(context.Background(), f.corpus, f.inventory(t), f.artifacts, f.recipes, codes); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[int]struct{ class, gap string }{
		1: {"unknown", "consolidated_status_unordered"}, 3: {"unknown", "consolidated_status_unordered"}, 5: {"unknown", "consolidated_status_unordered"}, 7: {"unknown", "consolidated_status_unresolved"}, 9: {"unknown", "consolidated_status_unordered"}, 11: {"unknown", "consolidated_status_unordered"}, 13: {"expired", ""}, 14: {"unknown", "consolidated_status_unresolved"}, 15: {"in_force", ""}, 17: {"expired", ""}, 19: {"expired", ""},
	} {
		key := fmt.Sprintf("vn:vbpl:%d", id)
		var class, basis string
		var current bool
		if err := f.corpus.QueryRow(context.Background(), `SELECT status_class,status_basis,is_current FROM document WHERE key=$1`, key).Scan(&class, &basis, &current); err != nil {
			t.Fatal(err)
		}
		if class != want.class || basis != "derived_from_consolidated" || current != (want.class == "in_force") {
			t.Errorf("%s got %s %s %v", key, class, basis, current)
		}
		if want.gap != "" {
			var exists bool
			if err := f.corpus.QueryRow(context.Background(), `SELECT EXISTS(SELECT 1 FROM coverage_gap WHERE document_key=$1 AND kind=$2)`, key, want.gap).Scan(&exists); err != nil || !exists {
				t.Errorf("%s missing gap %s: %v", key, want.gap, err)
			}
		}
	}
}
