package relate

import (
	"bytes"
	"strings"
	"testing"

	"danny.vn/trachuan/internal/frameworks/parse"
	"danny.vn/trachuan/internal/frameworks/registry"
)

func TestBuild_ResolvesMergesAndProtectsEvidence(t *testing.T) {
	r := &registry.Registry{Frameworks: []registry.Framework{{Code: "alpha", Version: "1", Access: "free", Ordinal: 1}, {Code: "beta", Version: "1", Access: "licensed", Ordinal: 1}, {Code: "beta", Version: "2", Access: "licensed", Ordinal: 2}}}
	catalogs := map[string]*parse.Result{"alpha:1": {Nodes: []parse.Node{{Citation: "A", Mappings: []parse.Mapping{{TargetFramework: "beta", TargetCitation: "B", Relation: "moved-to"}, {TargetFramework: "missing", TargetCitation: "C", Relation: "related"}}}}}, "beta:2": {Nodes: []parse.Node{{Citation: "B"}}}}
	inputs := []Input{{From: Key{"alpha", "1", "A"}, To: Key{"beta", "2", "B"}, Relation: "related", Source: "book", Evidence: "invented evidence"}, {From: Key{"alpha", "1", "A"}, To: Key{"beta", "2", "B"}, Relation: "related", Source: "book", Evidence: "second"}}
	got, stats, err := Build(r, catalogs, inputs)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || stats.Duplicates != 1 || stats.Unresolved != 1 {
		t.Fatalf("unexpected counts: %d %+v", len(got), stats)
	}
	if got[0].Source != "book" || got[0].Evidence == nil || got[0].Evidence.Value != "invented evidence" || len(got[0].Evidence.Requires) != 1 || got[0].Evidence.Requires[0] != "fw:beta:2" {
		t.Fatal("evidence or order incorrect")
	}
	if !got[1].Resolved || got[1].To.Version != "2" || got[1].Relation != "related" {
		t.Fatal("resolution incorrect")
	}
	var out bytes.Buffer
	if err := Write(&out, got); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(out.String(), "\n") {
		t.Fatal("missing newline")
	}
}

func TestReport_OnlyCounts(t *testing.T) {
	edges := []Edge{{From: Key{"alpha", "1", "invented-source"}, To: Key{"beta", "2", "invented-target"}, Relation: "related", Source: "book", Resolved: true, Evidence: &Evidence{Value: "invented-private-evidence", Requires: []string{}}}}
	var out bytes.Buffer
	if err := Report(&out, edges, Stats{Dropped: 2, Duplicates: 1}, &registry.Registry{}); err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"invented-source", "invented-target", "invented-private-evidence"} {
		if strings.Contains(out.String(), forbidden) {
			t.Fatal("private fixture in output")
		}
	}
	if !strings.Contains(out.String(), "resolved: 1") || !strings.Contains(out.String(), "dropped: 2") {
		t.Fatal("missing counts")
	}
}

func TestBuild_EvidenceDependenciesAndMissingSource(t *testing.T) {
	reg := &registry.Registry{Frameworks: []registry.Framework{{Code: "alpha", Version: "1", Access: "free"}, {Code: "beta", Version: "1", Access: "free"}, {Code: "gamma", Version: "1", Access: "licensed"}, {Code: "delta", Version: "1", Access: "licensed"}}}
	catalogs := map[string]*parse.Result{"alpha:1": {Nodes: []parse.Node{{Citation: "A"}}}, "beta:1": {Nodes: []parse.Node{{Citation: "B"}}}, "gamma:1": {Nodes: []parse.Node{{Citation: "C"}}}, "delta:1": {Nodes: []parse.Node{{Citation: "D"}}}}
	in := []Input{{From: Key{"alpha", "1", "A"}, To: Key{"beta", "1", "B"}, Relation: "related", Source: "book", Evidence: "invented"}, {From: Key{"delta", "1", "D"}, To: Key{"gamma", "1", "C"}, Relation: "related", Source: "book", Evidence: "invented"}, {From: Key{"alpha", "1", "missing"}, To: Key{"beta", "1", "B"}, Relation: "related", Source: "book"}}
	edges, stats, err := Build(reg, catalogs, in)
	if err != nil {
		t.Fatal(err)
	}
	if len(edges[0].Evidence.Requires) != 0 || len(edges[2].Evidence.Requires) != 2 || stats.Unresolved != 1 || edges[1].Resolved {
		t.Fatal("incorrect dependencies or missing source resolution")
	}
}

func TestBuild_CatalogVersionHint(t *testing.T) {
	reg := &registry.Registry{Frameworks: []registry.Framework{{Code: "alpha", Version: "1", Parser: "nist-csf-2-xlsx"}, {Code: "nist80053", Version: "r5", Aliases: []string{"NIST SP 800-53"}, Ordinal: 1}}}
	catalogs := map[string]*parse.Result{"alpha:1": {Nodes: []parse.Node{{Citation: "A", Mappings: []parse.Mapping{{TargetFramework: "NIST SP 800-53 Rev 5", TargetCitation: "AC-1", Relation: "related"}}}}}, "nist80053:r5": {Nodes: []parse.Node{{Citation: "AC-1"}}}}
	edges, _, err := Build(reg, catalogs, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(edges) != 1 || !edges[0].Resolved || edges[0].To.Code != "nist80053" || edges[0].To.Version != "r5" {
		t.Fatal("version hint not resolved")
	}
}

func TestBuild_RejectsUnknownEvidenceAccess(t *testing.T) {
	reg := &registry.Registry{Frameworks: []registry.Framework{{Code: "alpha", Version: "1", Access: "free"}, {Code: "beta", Version: "1"}}}
	_, _, err := Build(reg, nil, []Input{{From: Key{"alpha", "1", "A"}, To: Key{"beta", "1", "B"}, Relation: "related", Source: "book", Evidence: "invented"}})
	if err == nil {
		t.Fatal("unclassified evidence accepted")
	}
}

func TestResolveVersion_Tokens(t *testing.T) {
	reg := &registry.Registry{Frameworks: []registry.Framework{{Code: "alpha", Version: "r5", Aliases: []string{"Alpha"}}, {Code: "alpha", Version: "r5.2.0", Aliases: []string{"Alpha"}}, {Code: "alpha", Version: "r4", Aliases: []string{"Alpha"}}, {Code: "beta", Version: "v4.0", Aliases: []string{"Beta"}}, {Code: "gamma", Version: "1.1", Aliases: []string{"Gamma"}}, {Code: "delta", Version: "v8", Aliases: []string{"Delta"}}}}
	for _, tc := range []struct {
		name, version, code, want string
		ok                        bool
	}{{"Alpha Rev 5.2.0", "", "alpha", "r5.2.0", true}, {"Alpha Revision 5.1.1", "", "alpha", "r5", true}, {"Betav4.0", "", "beta", "v4.0", true}, {"Gamma v1.1", "", "gamma", "1.1", true}, {"Delta v8.0", "", "delta", "v8", true}, {"Alpha Rev 7.1", "", "alpha", "", false}, {"Alphabet v4.0", "", "Alphabet v4.0", "", false}, {"Alpha", "r5.2.0", "alpha", "r5.2.0", true}, {"Alpha", "v5.1.1", "alpha", "r5", true}} {
		t.Run(tc.name+tc.version, func(t *testing.T) {
			code, version, ok := resolveVersion(reg, tc.name, tc.version)
			if ok != tc.ok || code != tc.code || (tc.ok && version != tc.want) {
				t.Errorf("got %q %q %t, want %q %q %t", code, version, ok, tc.code, tc.want, tc.ok)
			}
		})
	}
}

func TestBuild_ResolvesPaddedOSCALCitations(t *testing.T) {
	reg := &registry.Registry{Frameworks: []registry.Framework{{Code: "alpha", Version: "1", CitationScheme: "csf-workbook"}, {Code: "beta", Version: "1", CitationScheme: "oscal-catalog"}}}
	catalogs := map[string]*parse.Result{"alpha:1": {Nodes: []parse.Node{{Citation: "A"}}}, "beta:1": {Nodes: []parse.Node{{Citation: "XX"}, {Citation: "XX-2(1)"}}}}
	inputs := []Input{{From: Key{"alpha", "1", "A"}, To: Key{"beta", "1", "XX-02(01)"}, Relation: "related", Source: "book"}, {From: Key{"beta", "1", "XX-02(01)"}, To: Key{"beta", "1", "XX"}, Relation: "related", Source: "book"}}
	edges, stats, err := Build(reg, catalogs, inputs)
	if err != nil {
		t.Fatal(err)
	}
	if len(edges) != 2 || stats.Unresolved != 0 || !edges[0].Resolved || edges[0].To.Citation != "XX-2(1)" || !edges[1].Resolved || edges[1].From.Citation != "XX-2(1)" || edges[1].To.Citation != "XX" {
		t.Fatal("padded citation did not resolve")
	}
}

func TestBuild_ResolvesCatalogPaddedOnlyLabels(t *testing.T) {
	data := []byte(`{"catalog":{"groups":[{"id":"xx","controls":[{"id":"xx-2","props":[{"name":"label","class":"zero-padded","value":"XX-02"}]}]}]}}`)
	count := 2
	catalog, err := parse.Parse("nist-oscal-800-53", data, "beta", "1", &count)
	if err != nil {
		t.Fatal(err)
	}
	reg := &registry.Registry{Frameworks: []registry.Framework{{Code: "alpha", Version: "1"}, {Code: "beta", Version: "1", CitationScheme: "oscal-catalog"}}}
	catalogs := map[string]*parse.Result{"alpha:1": {Nodes: []parse.Node{{Citation: "A"}}}, "beta:1": catalog}
	inputs := []Input{{From: Key{"alpha", "1", "A"}, To: Key{"beta", "1", "XX-2"}, Relation: "related", Source: "book"}, {From: Key{"beta", "1", "XX-2"}, To: Key{"alpha", "1", "A"}, Relation: "related", Source: "book"}}
	edges, stats, err := Build(reg, catalogs, inputs)
	if err != nil {
		t.Fatal(err)
	}
	if len(edges) != 2 || stats.Unresolved != 0 || !edges[0].Resolved || edges[0].To.Citation != "XX-02" || !edges[1].Resolved || edges[1].From.Citation != "XX-02" {
		t.Fatal("padded catalog identity not resolved")
	}
}

func TestBuild_UnresolvedDuplicatesCountOnce(t *testing.T) {
	reg := &registry.Registry{Frameworks: []registry.Framework{{Code: "alpha", Version: "1"}, {Code: "beta", Version: "1"}}}
	catalogs := map[string]*parse.Result{"alpha:1": {Nodes: []parse.Node{{Citation: "A"}}}}
	in := Input{From: Key{"alpha", "1", "A"}, To: Key{"beta", "1", "B"}, Relation: "related", Source: "book"}
	edges, stats, err := Build(reg, catalogs, []Input{in, in})
	if err != nil {
		t.Fatal(err)
	}
	if len(edges) != 1 || edges[0].Resolved || stats.Unresolved != 1 || stats.Duplicates != 1 {
		t.Fatal("unresolved duplicate counts incorrect")
	}
}
