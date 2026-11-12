package retrieval

import "testing"

func TestWarning_RejectsDiagramAndUnresolvedTarget(t *testing.T) {
	anchor := "dieu-1"
	for _, tc := range []struct {
		name     string
		relation Relation
	}{
		{"diagram only", Relation{Kind: "repeals", Method: "vbpl_diagram", TargetInHit: true, TargetResolved: true, TargetAnchor: &anchor, TargetNodeID: &anchor}},
		{"unresolved target", Relation{Kind: "repeals", Method: "text_rule/1", TargetInHit: true, TargetResolved: true, TargetNodeID: &anchor}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got, ok := warning(tc.relation); ok {
				t.Fatalf("warning = %+v, want none", got)
			}
		})
	}
}

func TestWarning_Fields(t *testing.T) {
	anchor, source := "article/clause", "source"
	got, ok := warning(Relation{Kind: "repeals", Method: "text_rule/1", SourceKey: "vn:source", SourceDocKey: "vn:source", SourceResolved: true, SourceAnchor: &source, TargetDocKey: "vn:target", TargetResolved: true, TargetAnchor: &anchor, TargetNodeID: &anchor, TargetCitation: "Khoản 1 Điều 1"})
	if !ok || got.Target != "vn:target#article/clause" || got.Citation != "Khoản 1 Điều 1" || !got.Exact || got.Source != "vn:source#source" {
		t.Fatalf("warning = %+v, eligible = %v", got, ok)
	}
}

func TestWarnings_MethodsAndExactness(t *testing.T) {
	anchor, missing := "article", "article/clause"
	for _, tc := range []struct {
		method   string
		node     *string
		basis    string
		exact    bool
		named    string
		eligible bool
	}{
		{"text_rule/1", &anchor, "text", true, "", true},
		{"text_rule/1", &missing, "text", false, missing, true},
		{"vbpl_references", &missing, "vbpl_reference", true, "", true},
		{"vbpl_references,vbpl_diagram", &missing, "vbpl_reference", true, "", true},
		{"vbpl_diagram", &anchor, "", false, "", false},
		{"text_rule/1", nil, "", false, "", false},
	} {
		w, ok := warning(Relation{Kind: "suspends", Method: tc.method, TargetResolved: true, TargetAnchor: &anchor, TargetNodeID: tc.node})
		if ok != tc.eligible || (ok && (w.Basis != tc.basis || w.Exact != tc.exact || w.Named != tc.named)) {
			t.Errorf("%s: %+v %v", tc.method, w, ok)
		}
	}
}
