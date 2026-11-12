package retrieval

import "testing"

func TestWarningPointers_RejectsDiagramAndUnresolvedTarget(t *testing.T) {
	anchor := "dieu-1"
	for _, tc := range []struct {
		name     string
		relation Relation
	}{
		{"diagram only", Relation{Kind: "repeals", Method: "vbpl_diagram", TargetInHit: true, TargetResolved: true, TargetAnchor: &anchor, TargetNodeID: &anchor}},
		{"unresolved target", Relation{Kind: "repeals", Method: "text_rule/1", TargetInHit: true, TargetResolved: true, TargetNodeID: &anchor}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got, _ := shapeWarnings([]SourceRelation{{Value: tc.relation, TargetInHit: true}}); len(got) != 0 {
				t.Fatalf("warnings = %+v, want none", got)
			}
		})
	}
}

func TestWarnings_FieldsAndLimit(t *testing.T) {
	anchor, named, source := "article/clause", "article/clause", "source"
	rels := make([]SourceRelation, 12)
	for i := range rels {
		rels[i] = SourceRelation{Value: Relation{Kind: "repeals", Method: "text_rule/1", SourceKey: "vn:source", SourceDocKey: "vn:source", SourceResolved: true, SourceAnchor: &source, TargetDocKey: "vn:target", TargetResolved: true, TargetAnchor: &anchor, TargetNodeID: &named, TargetCitation: "Khoản 1 Điều 1"}, TargetInHit: true, Ordinal: i}
	}
	warnings, omitted := shapeWarnings(rels)
	if len(warnings) != 10 || omitted != 2 || warnings[0].Target != "vn:target#article/clause" || warnings[0].Citation != "Khoản 1 Điều 1" || !warnings[0].Exact || warnings[0].Source != "vn:source#source" {
		t.Fatalf("warnings = %+v, omitted = %d", warnings, omitted)
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

func TestWarnings_OrderAndPresentation(t *testing.T) {
	anchor := "article/clause"
	node := anchor
	target := "vn:target"
	row := func(kind, source string, ordinal int) SourceRelation {
		return SourceRelation{Value: Relation{Kind: kind, Method: "text_rule/1", SourceKey: source, SourceDocKey: source, SourceResolved: true, TargetDocKey: target, TargetKey: &target, TargetAnchor: &anchor, TargetNodeID: &node, TargetResolved: true, TargetCitation: "Điều 1, Khoản 1"}, TargetInHit: true, Ordinal: ordinal}
	}
	rows := []SourceRelation{row("suspends", "vn:z", 2), row("cites", "vn:a", 1), row("repeals", "vn:b", 3), row("repeals", "vn:a", 4)}
	warnings, omitted := shapeWarnings(rows)
	if omitted != 0 || len(warnings) != 3 || warnings[0].Key != "vn:a" || warnings[1].Key != "vn:b" || warnings[2].Kind != "suspends" {
		t.Fatalf("warnings %+v omitted %d", warnings, omitted)
	}
	hit := presentationHit(target, "article")
	hit.Warnings = warnings
	got := present(Response{Hits: []Hit{hit}}, nil, DetailCompact).Hits[0]
	if got.Warnings[0].Citation != "Điều 1, Khoản 1" || got.Warnings[0].Target != "vn:target#article/clause" {
		t.Fatalf("presented warnings %+v", got.Warnings)
	}
	rows[0].TargetInHit = false
	rows[0].SourceInHit = true
	onlyOutgoing, _ := shapeWarnings(rows[:1])
	if len(onlyOutgoing) != 0 {
		t.Fatalf("outgoing warning %+v", onlyOutgoing)
	}
}
