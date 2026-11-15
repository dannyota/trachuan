package parse

import (
	"bytes"
	"testing"

	"github.com/xuri/excelize/v2"
)

func workbook(t *testing.T, sheet string, rows [][]string) []byte {
	t.Helper()
	f := excelize.NewFile()
	if err := f.SetSheetName("Sheet1", sheet); err != nil {
		t.Fatal(err)
	}
	for i, row := range rows {
		for j, value := range row {
			cell, err := excelize.CoordinatesToCellName(j+1, i+1)
			if err != nil {
				t.Fatal(err)
			}
			if err = f.SetCellStr(sheet, cell, value); err != nil {
				t.Fatal(err)
			}
		}
	}
	b, err := f.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestParsers_TreesAndValidation(t *testing.T) {
	fixtures := []struct {
		name  string
		data  []byte
		kinds []string
	}{
		{"nist-oscal-800-53", []byte(`{"catalog":{"groups":[{"id":"aa","title":"invented family","props":[{"name":"label","value":"AA"}],"controls":[{"id":"aa-1","title":"invented control","props":[{"name":"label","value":"AA-1"},{"name":"status","value":"withdrawn"}],"links":[{"rel":"moved-to","href":"#aa-2"}],"controls":[{"id":"aa-2","title":"invented enhancement","props":[{"name":"label","value":"AA-2"}]}]}]}]}}`), []string{"family", "control", "enhancement"}},
		{"nist-csf-2-xlsx", workbook(t, "CSF 2.0", [][]string{{}, {}, {"EXAMPLE (AA): invented function"}, {"", "invented category (AA.BB): invented body"}, {"", "", "AA.BB-01: invented subcategory", "invented example", "Other: ZZ-1"}}), []string{"function", "category", "subcategory"}},
		{"cis-controls-8-xlsx", workbook(t, "Controls v8.1.2", [][]string{{"header"}, {"1", "", "", "", "invented control", "invented description"}, {"1", "1.1", "invented asset", "invented function", "invented safeguard", "invented body", "x", "", "x"}}), []string{"control", "safeguard"}},
	}
	for _, f := range fixtures {
		t.Run(f.name, func(t *testing.T) {
			n := len(f.kinds)
			got, err := Parse(f.name, f.data, "example", "1.0", &n)
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Nodes) != n {
				t.Fatalf("count = %d", len(got.Nodes))
			}
			for i, k := range f.kinds {
				if got.Nodes[i].Kind != k || got.Nodes[i].Parent != i-1 || got.Nodes[i].Title != "" {
					t.Fatalf("node %d wrong shape", i)
				}
			}
			wrong := n + 1
			if _, err = Parse(f.name, f.data, "example", "1.0", &wrong); err == nil {
				t.Error("count mismatch accepted")
			}
			if _, err = Parse(f.name, f.data, "example", "1.0", nil); err == nil {
				t.Error("unset count accepted")
			}
			if f.name != "cis-controls-8-xlsx" && len(got.Nodes[n-1].Mappings)+len(got.Nodes[1].Mappings) == 0 {
				t.Error("missing mapping")
			}
			if f.name == "nist-oscal-800-53" && (got.Nodes[1].Status != "withdrawn" || got.Nodes[1].Mappings[0].Relation != "moved-to" || got.Nodes[1].Mappings[0].TargetCitation != "AA-2") {
				t.Error("wrong withdrawal mapping")
			}
			if f.name == "nist-csf-2-xlsx" && (got.Nodes[2].Mappings[0].TargetCitation != "ZZ-1" || got.Nodes[2].Mappings[0].Relation != "related") {
				t.Error("wrong informative reference")
			}
			if f.name == "cis-controls-8-xlsx" && (got.Nodes[1].Attributes.AssetType == "" || len(got.Nodes[1].Attributes.ImplementationGroups) != 2) {
				t.Error("missing attributes")
			}
		})
	}
}

func TestValidate_Citations(t *testing.T) {
	for _, citations := range [][]string{{"a", "A"}, {"bad  space"}, {"bad\nvalue"}, {string(bytes.Repeat([]byte("a"), 65))}} {
		nodes := make([]Node, len(citations))
		for i, c := range citations {
			nodes[i].Citation = c
		}
		count := len(nodes)
		if err := validate(nodes, &count); err == nil {
			t.Error("invalid citations accepted")
		}
	}
}

func TestParsers_RejectDuplicateAndInvalidCitations(t *testing.T) {
	cases := []struct {
		name string
		data []byte
	}{
		{"nist-oscal-800-53", []byte(`{"catalog":{"groups":[{"id":"a","props":[{"name":"label","value":"A"}],"controls":[{"id":"b","props":[{"name":"label","value":"a"}]}]}]}}`)},
		{"nist-oscal-800-53", []byte(`{"catalog":{"groups":[{"id":"a","props":[{"name":"label","value":"bad  id"}]}]}}`)},
		{"nist-csf-2-xlsx", workbook(t, "CSF 2.0", [][]string{{}, {}, {"EXAMPLE (AA): invented"}, {"", "invented (AA.BB): invented"}, {"", "invented (AA.BB): invented"}})},
		{"nist-csf-2-xlsx", workbook(t, "CSF 2.0", [][]string{{}, {}, {"EXAMPLE (AA): invented"}, {"", "invalid"}})},
		{"cis-controls-8-xlsx", workbook(t, "Controls v8", [][]string{{}, {"1"}, {"1"}})},
		{"cis-controls-8-xlsx", workbook(t, "Controls v8", [][]string{{}, {"bad  id"}})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			n := 3
			result, err := Parse(tc.name, tc.data, "example", "1.0", &n)
			if err == nil || (err.Error() != "invalid category row" && result != nil && result.InvalidCitations == 0 && result.Duplicates == 0) {
				t.Error("invalid fixture accepted")
			}
		})
	}
}

func TestCSF_WithdrawnWithoutCategory(t *testing.T) {
	data := workbook(t, "CSF 2.0", [][]string{{}, {}, {"EXAMPLE (AA): invented"}, {"", "", "AA.BB-01: [Withdrawn: Moved to AA.CC-01]", "invented example"}})
	n := 2
	result, err := Parse("nist-csf-2-xlsx", data, "alpha", "1.0", &n)
	if err != nil {
		t.Fatal(err)
	}
	if result.Nodes[1].Parent != 0 || result.Nodes[1].Status != "withdrawn" || len(result.Nodes[1].Mappings) != 1 || result.Nodes[1].Mappings[0].TargetCitation != "AA.CC-01" {
		t.Error("withdrawn hierarchy or mapping lost")
	}
}

func TestCSF_ReferenceAndMissingParent(t *testing.T) {
	data := workbook(t, "CSF 2.0", [][]string{{}, {}, {"EXAMPLE (AA): invented"}, {"", "invented (AA.BB): invented"}, {"", "", "AA.BB-01: invented detail", "", "ISO/IEC 27001: 2022: Annex A Controls: 9.9"}})
	n := 3
	result, err := Parse("nist-csf-2-xlsx", data, "alpha", "1.0", &n)
	if err != nil {
		t.Fatal(err)
	}
	m := result.Nodes[2].Mappings[0]
	if m.TargetCitation != "A.9.9" || m.TargetVersion != "2022" || result.Nodes[2].PublisherTitle != "invented detail" {
		t.Error("reference or publisher title lost")
	}
	data = workbook(t, "CSF 2.0", [][]string{{}, {}, {"EXAMPLE (AA): invented"}, {"", "", "AA.BB-01: invented detail"}})
	if _, err = Parse("nist-csf-2-xlsx", data, "alpha", "1.0", &n); err == nil {
		t.Error("missing active parent accepted")
	}
}
