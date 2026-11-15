package parse

import (
	"bytes"
	"strings"
	"testing"
)

func TestOSCAL_Links(t *testing.T) {
	data := []byte(`{"catalog":{"groups":[{"id":"xx","controls":[{"id":"xx-1","links":[{"rel":"related","href":"#xx-2"},{"rel":"moved-to","href":"#xx-2"},{"rel":"incorporated-into","href":"#xx-2"},{"rel":"related","href":"#xx-9"},{"rel":"reference","href":"#xx-8"},{"rel":"related","href":"#12345678-1234-1234-1234-123456789abc"},{"rel":"moved-to","href":"https://example.test/xx-9"},{"rel":"related","href":"#xx-2_smt"},{"rel":"related","href":"#appendix"}]},{"id":"xx-2"}]}]}}`)
	n := 3
	r, err := Parse("nist-oscal-800-53", data, "example", "1.0", &n)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Nodes[1].Mappings) != 4 {
		t.Fatalf("mapping count = %d, want 4", len(r.Nodes[1].Mappings))
	}
	for _, m := range r.Nodes[1].Mappings[:3] {
		if m.Unresolved || m.TargetCitation != "XX-2" {
			t.Error("resolved link lost")
		}
	}
	if !r.Nodes[1].Mappings[3].Unresolved {
		t.Error("missing target not marked unresolved")
	}
	var out bytes.Buffer
	if err := Report(&out, r); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "unresolved mappings: 1") {
		t.Error("unresolved statistic missing")
	}
	if strings.Contains(out.String(), "xx-9") {
		t.Error("target leaked")
	}
}

func TestParse_FixedErrorClasses(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
		want string
	}{{"nist-oscal-800-53", []byte(`{"catalog":"invented-private-value"}`), "invalid catalog JSON"}, {"cis-controls-8-xlsx", []byte("invented-private-value"), "invalid workbook"}} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(tc.name, tc.data, "alpha", "1.0", nil)
			if err == nil || err.Error() != tc.want {
				t.Errorf("unexpected error class: %v", err)
			}
		})
	}
}

func TestCSF_PartialStatistics(t *testing.T) {
	data := workbook(t, "CSF 2.0", [][]string{{}, {}, {"EXAMPLE (XX): invented"}, {"", "invalid invented input"}})
	n := 2
	r, err := Parse("nist-csf-2-xlsx", data, "alpha", "1.0", &n)
	if err == nil || err.Error() != "invalid category row" {
		t.Errorf("unexpected parse error: %v", err)
	}
	if r == nil || len(r.Nodes) != 1 {
		t.Fatal("partial result missing")
	}
	var out bytes.Buffer
	if err := Report(&out, r); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "controls: 1") {
		t.Error("partial statistics missing")
	}
}

func TestOSCAL_PrefersUnpaddedLabels(t *testing.T) {
	data := []byte(`{"catalog":{"groups":[{"id":"xx","controls":[{"id":"xx-1","props":[{"name":"label","class":"zero-padded","value":"XX-01"},{"name":"label","value":"XX-1"}],"controls":[{"id":"xx-1.1","props":[{"name":"label","class":"zero-padded","value":"XX-01(01)"},{"name":"label","value":"XX-1(1)"}]}],"links":[{"rel":"related","href":"#xx-1.1"}]},{"id":"xx-2","props":[{"name":"label","class":"zero-padded","value":"XX-02"}]},{"id":"xx-3"}]}]}}`)
	count := 5
	result, err := Parse("nist-oscal-800-53", data, "alpha", "1", &count)
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []string{"XX", "XX-1", "XX-1(1)", "XX-02", "XX-3"} {
		if result.Nodes[i].Citation != want {
			t.Errorf("citation %d: got %q want %q", i, result.Nodes[i].Citation, want)
		}
	}
	if result.Nodes[1].Mappings[0].TargetCitation != "XX-1(1)" {
		t.Error("link target not canonical")
	}
}
