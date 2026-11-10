package chunk

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrefix_RecordedVBPLDates(t *testing.T) {
	paths, err := filepath.Glob("../../vbpl/testdata/detail-*.json")
	if err != nil || len(paths) == 0 {
		t.Fatalf("recorded details: %v", err)
	}
	count := 0
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var detail struct {
			Data struct {
				EffFrom   string `json:"effFrom"`
				IssueDate string `json:"issueDate"`
			} `json:"data"`
		}
		if err = json.Unmarshal(data, &detail); err != nil {
			t.Fatal(err)
		}
		raw := detail.Data.EffFrom
		label := "Hiệu lực: "
		if raw == "" {
			raw = detail.Data.IssueDate
			label = "Ban hành: "
		}
		if raw == "" {
			continue
		}
		count++
		got := prefix(Meta{EffFrom: detail.Data.EffFrom, IssueDate: detail.Data.IssueDate}, &treeNode{}, Recipe1())
		if !strings.Contains(got, label+raw[:10]) {
			t.Errorf("%s: prefix %q", filepath.Base(path), got)
		}
	}
	if count == 0 {
		t.Fatal("no dates in recorded details")
	}
}
