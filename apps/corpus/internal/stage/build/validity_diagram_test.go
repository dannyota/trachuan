package build

import "testing"

func runAt2099(t *testing.T, docs []validityDocument, relations []validityRelation) validityResult {
	t.Helper()
	v, err := runValidity(docs, relations, nil, "2099-06-01")
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func diagramPair() (target, actor validityDocument) {
	return testDoc("a", "1/2099/NĐ-CP", "2099-01-01", "2099-01-01"), testDoc("b", "2/2099/NĐ-CP", "2099-02-01", "2099-03-01")
}

func TestRuleC_OnlyDiagramRelationsSetConflict(t *testing.T) {
	for _, tc := range []struct {
		name, method string
		want         bool
	}{
		{"diagram", "vbpl_diagram", true},
		{"merged diagram", "vbpl_references,vbpl_diagram", true},
		{"text rule", "text_rule/1", false},
		{"references only", "vbpl_references", false},
		{"no method", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			target, actor := diagramPair()
			actor.article = testArticle("1/3/2099", "")
			v := runAt2099(t, []validityDocument{target, actor}, []validityRelation{{source: "b", target: "a", kind: "replaces", method: tc.method, id: 5}})
			if v.documents["a"].values.Conflict != tc.want || hasReview(v, "rule_c_conflict") != tc.want || (v.counts.ruleC == 1) != tc.want {
				t.Fatalf("conflict=%v reviews=%+v", v.documents["a"].values.Conflict, v.reviews)
			}
		})
	}
}

func TestRuleC_ReplacesUnverifiedOnlyForDiagramRelations(t *testing.T) {
	for _, tc := range []struct {
		method string
		want   bool
	}{{"vbpl_diagram", true}, {"text_rule/1", false}} {
		for _, article := range []bool{false, true} {
			target, actor := diagramPair()
			if article {
				actor.article = testArticle("1/3/2099", "Nghị định này hết hiệu lực theo quy định khác.")
			}
			v := runAt2099(t, []validityDocument{target, actor}, []validityRelation{{source: "b", target: "a", kind: "replaces", method: tc.method, id: 5}})
			if v.documents["a"].values.Conflict || hasReview(v, "rule_c_conflict") || hasReview(v, "replaces_unverified") != tc.want {
				t.Fatalf("%s article=%v: reviews=%+v", tc.method, article, v.reviews)
			}
		}
	}
}

func TestRuleC_PartialTargetAndRepealsReviewOnlyForDiagramRelations(t *testing.T) {
	for _, tc := range []struct {
		kind, class, review string
	}{{"replaces", "partial", "partial_target"}, {"repeals", "in_force", "repeals_current"}} {
		for method, want := range map[string]bool{"vbpl_diagram": true, "vbpl_references,vbpl_diagram": true, "text_rule/1": false, "vbpl_references": false, "": false} {
			target, actor := diagramPair()
			target.sourceClass = tc.class
			actor.article = testArticle("1/3/2099", "")
			v := runAt2099(t, []validityDocument{target, actor}, []validityRelation{{source: "b", target: "a", kind: tc.kind, method: method, id: 5}})
			if v.documents["a"].values.Conflict || hasReview(v, tc.review) != want {
				t.Fatalf("%s %q: reviews=%+v", tc.review, method, v.reviews)
			}
		}
	}
}

func TestRuleB_AgreementOnlyFromDiagramRelations(t *testing.T) {
	text := validityRelation{source: "b", target: "a", kind: "replaces", method: "text_rule/1", id: 4}
	for _, tc := range []struct {
		name      string
		relations []validityRelation
		want      int64
	}{
		{"text rule only", []validityRelation{text}, 0},
		{"diagram beside text rule", []validityRelation{text, {source: "b", target: "a", kind: "replaces", method: "vbpl_diagram", id: 9}}, 9},
		{"merged diagram", []validityRelation{{source: "b", target: "a", kind: "replaces", method: "vbpl_references,vbpl_diagram", id: 6}}, 6},
		{"references only", []validityRelation{{source: "b", target: "a", kind: "replaces", method: "vbpl_references", id: 6}}, 0},
		{"diagram of another verb", []validityRelation{{source: "b", target: "a", kind: "repeals", method: "vbpl_diagram", id: 3}}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			target, actor := diagramPair()
			actor.article = testArticle("1/3/2099", replacement(target.number, "1/1/2099"))
			v := runAt2099(t, []validityDocument{target, actor}, tc.relations)
			h := v.documents["a"].history
			if len(h) != 2 || h[1].layer != "B" || h[1].relationID != tc.want {
				t.Fatalf("history=%+v", h)
			}
		})
	}
}
