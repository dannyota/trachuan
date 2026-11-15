package relate

import (
	"bytes"
	"testing"

	"danny.vn/trachuan/internal/frameworks/parse"
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
	var b bytes.Buffer
	if err := f.Write(&b); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func TestWorkbook_OLIR(t *testing.T) {
	data := workbook(t, "Relationships-AC", [][]string{{"Focal\nDocument Element", "Focal Document Element Description", "Security Control Baseline", "Reference Document Element", "Reference Document Element Description", "Comments (optional)", "Strength of Relationship (optional)"}, {"AC-01(02)", "invented body", "x", "5.2", "invented body", "invented note", "x"}, {"AC-01", "", "", "-"}})
	in, dropped, err := Workbook("nist-olir", data, nil)
	if err != nil {
		t.Fatal(err)
	}
	if dropped != 1 || len(in) != 1 || in[0].From.Citation != "AC-1(2)" || in[0].Evidence != "invented note" || in[0].Relation != "related" {
		t.Fatal("incorrect OLIR mapping")
	}
}
func TestWorkbook_CIS(t *testing.T) {
	data := workbook(t, "All CIS Controls & Safeguards", [][]string{{"", "", "", "", "", "", "", "", "", "", "Relationship", "Control #"}, {"", "4.1", "", "", "", "invented title", "", "", "", "", "Subset", "A8.9"}, {"", "4.1", "", "", "", "invented title", "", "", "", "", "", ""}})
	nodes := []parse.Node{{Kind: "safeguard", Citation: "4.10", PublisherTitle: "invented title"}}
	in, dropped, err := Workbook("cis-iso27001", data, nodes)
	if err != nil {
		t.Fatal(err)
	}
	if dropped != 0 || len(in) != 1 || in[0].From.Citation != "4.10" || in[0].To.Citation != "A.8.9" || in[0].Relation != "subset" {
		t.Fatal("incorrect CIS mapping")
	}
}

func TestWorkbook_CISTargetsAndRelations(t *testing.T) {
	nodes := []parse.Node{{Kind: "safeguard", Citation: "1.1", PublisherTitle: "invented title"}}
	for _, tc := range []struct{ parser, target, want, relation, expected string }{{"cis-nistcsf", "AA.BB-01", "AA.BB-01", "Equivalent", "equivalent"}, {"cis-nist80053", "AC-01(02)", "AC-1(2)", "Superset", "superset"}, {"cis-iso27001", "5.2", "5.2", "", "related"}} {
		t.Run(tc.parser, func(t *testing.T) {
			data := workbook(t, "All CIS Controls & Safeguards", [][]string{{}, {"", "", "", "", "", "invented title", "", "", "", "", tc.relation, tc.target}})
			got, dropped, err := Workbook(tc.parser, data, nodes)
			if err != nil || dropped != 0 || len(got) != 1 {
				t.Fatal("mapping missing")
			}
			if got[0].To.Citation != tc.want || got[0].Relation != tc.expected {
				t.Fatal("wrong mapping")
			}
		})
	}
}

func TestWorkbook_OLIRMalformedFocal(t *testing.T) {
	data := workbook(t, "Relationships-XX", [][]string{{"Focal Document Element", "Focal Document Element Description", "Security Control Baseline", "Reference Document Element", "Reference Document Element Description", "Comments (optional)"}, {"invented malformed", "", "", "5.2"}, {"invented malformed", "", "", ""}, {"", "", "", "5.3"}})
	inputs, dropped, err := Workbook("nist-olir", data, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(inputs) != 0 || dropped != 1 {
		t.Fatal("incorrect malformed focal count")
	}
}

func TestWorkbook_CISAmbiguousTitleDropped(t *testing.T) {
	data := workbook(t, "All CIS Controls & Safeguards", [][]string{{}, {"", "", "", "", "", "invented title", "", "", "", "", "Equivalent", "5.2"}})
	nodes := []parse.Node{{Kind: "safeguard", Citation: "1.1", PublisherTitle: "invented title"}, {Kind: "safeguard", Citation: "1.2", PublisherTitle: "invented title"}}
	inputs, dropped, err := Workbook("cis-iso27001", data, nodes)
	if err != nil {
		t.Fatal(err)
	}
	if len(inputs) != 0 || dropped != 1 {
		t.Fatal("ambiguous title was not dropped")
	}
}
