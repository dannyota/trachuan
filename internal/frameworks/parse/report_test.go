package parse

import (
	"bytes"
	"strings"
	"testing"
)

func TestReport_OnlyStatistics(t *testing.T) {
	r := &Result{Nodes: []Node{{Citation: "secret-citation", Kind: "control", Parent: -1, PublisherTitle: "invented-publisher-title", Body: "invented-private-body", Status: "withdrawn", Mappings: []Mapping{{TargetFramework: "invented-private-hint", TargetCitation: "invented-target", Relation: "invented-private-relation"}}}}}
	var out bytes.Buffer
	if err := Report(&out, r); err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"secret-citation", "invented-publisher-title", "invented-private-body", "invented-private-hint", "invented-target", "invented-private-relation"} {
		if strings.Contains(out.String(), s) {
			t.Error("report contains fixture text")
		}
	}
	for _, s := range []string{"controls: 1", "withdrawn: 1", "duplicates: 0", "invalid citations: 0"} {
		if !strings.Contains(out.String(), s) {
			t.Errorf("missing statistic %q", s)
		}
	}
}

func TestReport_RegisteredTarget(t *testing.T) {
	r := &Result{Nodes: []Node{{Parent: -1, Kind: "control", Mappings: []Mapping{{TargetFramework: "invented standard v2", Relation: "related"}}}}}
	var out bytes.Buffer
	err := Report(&out, r, map[string]string{"invented standard": "alpha"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "target alpha: 1") {
		t.Error("missing registered target")
	}
	if strings.Contains(out.String(), "invented standard") {
		t.Error("source hint leaked")
	}
}
