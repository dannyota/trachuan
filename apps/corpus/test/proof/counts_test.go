//go:build integration && faultinject

package proof

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

type counts struct {
	items  map[string]int
	tables map[string]int
	dirs   int
}

func (h *harness) countRows() counts {
	h.t.Helper()
	result := counts{items: make(map[string]int), tables: make(map[string]int)}
	rows, err := h.db.Query(h.ctx, "SELECT stage, state, count(*) FROM run_item GROUP BY stage, state")
	if err != nil {
		h.t.Fatal(err)
	}
	for rows.Next() {
		var stage, state string
		var count int
		if err := rows.Scan(&stage, &state, &count); err != nil {
			h.t.Fatal(err)
		}
		result.items[stage+":"+state] = count
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		h.t.Fatal(rowsErr)
	}
	rows.Close()
	for _, table := range []string{"artifact", "document", "provision", "relation", "gap", "listing", "fetched_file"} {
		var count int
		if err := h.db.QueryRow(h.ctx, "SELECT count(*) FROM "+table).Scan(&count); err != nil {
			h.t.Fatal(err)
		}
		result.tables[table] = count
	}
	entries, err := os.ReadDir(h.artifacts)
	if err != nil {
		h.t.Fatal(err)
	}
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		dirs, readErr := os.ReadDir(filepath.Join(h.artifacts, entry.Name()))
		if readErr != nil {
			h.t.Fatal(readErr)
		}
		for _, dir := range dirs {
			if dir.IsDir() {
				result.dirs++
			}
		}
	}
	return result
}

func (h *harness) assertCounts(want counts) {
	h.t.Helper()
	got := h.countRows()
	if !reflect.DeepEqual(got, want) {
		h.t.Fatalf("counts = %+v, want %+v", got, want)
	}
	for _, stage := range []string{"discover", "fetch", "extract", "relate"} {
		count := 30
		if stage == "discover" {
			count = 2
		}
		if got.items[stage+":done"] != count {
			h.t.Errorf("%s done = %d, want %d", stage, got.items[stage+":done"], count)
		}
	}
	if len(got.items) != 4 {
		h.t.Errorf("unexpected item states: %v", got.items)
	}
	if got.dirs != got.tables["artifact"] {
		h.t.Errorf("artifact directories = %d, rows = %d", got.dirs, got.tables["artifact"])
	}
	for _, query := range []string{
		"SELECT count(*) FROM (SELECT stage,item_key,count(*) FROM run_item GROUP BY stage,item_key HAVING count(*) <> 1) d",
		"SELECT count(*) FROM (SELECT artifact_id,name,count(*) FROM fetched_file GROUP BY artifact_id,name HAVING count(*) <> 1) d",
	} {
		var duplicates int
		if err := h.db.QueryRow(h.ctx, query).Scan(&duplicates); err != nil {
			h.t.Fatal(err)
		}
		if duplicates != 0 {
			h.t.Errorf("duplicate rows: %d", duplicates)
		}
	}
}
