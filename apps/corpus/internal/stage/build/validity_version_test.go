package build

import (
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/source"
)

func TestDecisions_RuleVersion3(t *testing.T) {
	commenced := testDoc("n", "3/2099/NĐ-CP", "2099-01-01", "")
	commenced.statusCode, commenced.sourceClass = "CCHL", "not_yet"
	commenced.article = testArticle("1/2/2099", "")
	replaced, actor := diagramPair()
	actor.article = testArticle("1/3/2099", replacement(replaced.number, "1/1/2099"))
	conflicted := testDoc("c", "4/2099/NĐ-CP", "2099-01-01", "2099-01-01")
	corrected := testDoc("d", "5/2099/NĐ-CP", "2099-01-01", "2099-01-01")
	eligible := false
	corrections := map[string]source.Correction{"d": {Eligible: &eligible, Evidence: source.CorrectionEvidence{Anchor: "dieu-1", Quote: "quote"}}}
	v, err := runValidity([]validityDocument{commenced, replaced, actor, conflicted, corrected}, []validityRelation{{source: "b", target: "c", kind: "replaces", method: "vbpl_diagram", id: 1}}, corrections, "2099-06-01")
	if err != nil {
		t.Fatal(err)
	}
	for key, layer := range map[string]string{"n": "A", "a": "B", "c": "C", "d": "manual"} {
		h := v.documents[key].history
		if len(h) != 2 || h[1].layer != layer {
			t.Fatalf("%s history=%+v", key, h)
		}
		for _, decision := range h {
			if decision.version != "validity/3" {
				t.Errorf("%s %s version=%q", key, decision.layer, decision.version)
			}
		}
	}
}

func TestCorrections_UnverifiedSourceDecisionVersion3(t *testing.T) {
	l := gapLoader()
	entries, held, err := l.validateCorrectionGaps([]source.Correction{gapCorrection()}, map[string]correctionDocument{"a": {number: "A"}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := runValidity([]validityDocument{testDoc("a", "A", "2099-01-01", "2099-01-01")}, nil, entries, "2099-06-01")
	if err != nil {
		t.Fatal(err)
	}
	if err = l.recordCorrectionGaps(&result, held); err != nil {
		t.Fatal(err)
	}
	h := result.documents["a"].history
	if len(h) != 2 || h[0].layer != "source" || h[0].version != "validity/3" || h[1].version != "correction_unverified" {
		t.Fatalf("history=%+v", h)
	}
}
