package parse

import (
	"bytes"
	"strings"
	"testing"
)

func TestCSF_RejectsInvalidMappings(t *testing.T) {
	data := workbook(t, "CSF 2.0", [][]string{{}, {}, {"EXAMPLE (XX): invented"}, {"", "invented (XX.YY): [Withdrawn: Moved to XX.ZZ, invented narrative]"}, {"", "", "XX.YY-01: invented", "", "ISO/IEC 27001: 2022: Annex A Controls: 9.9\nISO/IEC 27001: 2022: Control 9.8,\nISO/IEC 27001: 2022: Annex A Controls: None\nISO/IEC 27001: 2022: Mandatory Clause: N/A\nISO/IEC 27001: 2022: unknown value\nISO/IEC 27001: 2022: Annex A Controls:\nISO/IEC 27001:\nISO/IEC 27001"}})
	n := 3
	r, err := Parse("nist-csf-2-xlsx", data, "alpha", "1.0", &n)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Nodes[1].Mappings) != 1 || r.Nodes[1].Mappings[0].TargetCitation != "XX.ZZ" {
		t.Error("withdrawal targets include narrative")
	}
	if len(r.Nodes[2].Mappings) != 2 || r.Nodes[2].Mappings[0].TargetCitation != "A.9.9" || r.Nodes[2].Mappings[1].TargetCitation != "A.9.8" {
		t.Error("invalid ISO references emitted")
	}
	var out bytes.Buffer
	if err := Report(&out, r); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "dropped mappings: 7") {
		t.Error("dropped mappings not counted")
	}
}

func TestCSF_DescribedFunctionReferences(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		t.Run(map[bool]string{false: "bare first", true: "described first"}[reverse], func(t *testing.T) {
			bare := []string{"EXAMPLE (XX)", "", "", "", "Other: bare"}
			described := []string{"EXAMPLE (XX): invented detail", "", "", "", "Other: described"}
			if reverse {
				bare, described = described, bare
			}
			data := workbook(t, "CSF 2.0", [][]string{{}, {}, bare, described})
			n := 1
			r, err := Parse("nist-csf-2-xlsx", data, "alpha", "1.0", &n)
			if err != nil {
				t.Fatal(err)
			}
			if r.Nodes[0].PublisherTitle != "invented detail" || len(r.Nodes[0].Mappings) != 1 || r.Nodes[0].Mappings[0].TargetCitation != "described" {
				t.Error("described row references missing")
			}
		})
	}
}

func TestOSCAL_ParameterInserts(t *testing.T) {
	data := []byte(`{"catalog":{"params":[{"id":"global","label":"invented global"}],"groups":[{"id":"xx","controls":[{"id":"xx-1","params":[{"id":"choice","label":"invented choice label","select":{"how-many":"one-or-more","choice":["invented first","invented second"]}},{"id":"assignment","label":"invented assignment"}],"parts":[{"prose":"{{ insert: param, global }}; {{ insert: param, assignment }}; {{ insert: param, choice }}","parts":[{"prose":"{{ insert: param, assignment }}"}]}]}]}]}}`)
	n := 2
	r, err := Parse("nist-oscal-800-53", data, "alpha", "1.0", &n)
	if err != nil {
		t.Fatal(err)
	}
	body := r.Nodes[1].Body
	for _, s := range []string{"invented global", "invented assignment", "invented choice label", "invented first", "invented second"} {
		if !strings.Contains(body, s) {
			t.Error("parameter value missing")
		}
	}
	if strings.Contains(body, "{{ insert:") {
		t.Error("parameter not resolved")
	}
}

func TestReport_NISTReferencePrefix(t *testing.T) {
	r := &Result{Nodes: []Node{{Parent: -1, Kind: "subcategory", Mappings: []Mapping{{TargetFramework: "SP 800-53 Rev 5", Relation: "related"}}}}}
	var out bytes.Buffer
	if err := Report(&out, r); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "target nist80053: 1") {
		t.Error("target grouping missing")
	}
}

func TestOSCAL_ParameterDefinedInAnotherControl(t *testing.T) {
	data := []byte(`{"catalog":{"groups":[{"id":"xx","controls":[{"id":"xx-1","parts":[{"prose":"{{ insert: param, shared }}"}]},{"id":"xx-2","params":[{"id":"shared","label":"invented shared definition"}]}]}]}}`)
	n := 3
	r, err := Parse("nist-oscal-800-53", data, "alpha", "1.0", &n)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.Nodes[1].Body, "invented shared definition") {
		t.Error("shared parameter not resolved")
	}
}
