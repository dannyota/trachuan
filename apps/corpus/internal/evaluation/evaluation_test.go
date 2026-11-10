package evaluation

import (
	"strings"
	"testing"

	"danny.vn/trachuan/internal/retrieval"
)

func TestLoadLegacy_ValidatesIDsAndFlags(t *testing.T) {
	for _, tc := range []struct {
		input string
		ok    bool
	}{
		{`[{"id":"a","question":"q","expected_citations":[{"doc_number":"12"}]}]`, true},
		{`[{"id":"a","question":"q","expect_abstain":true}]`, true},
		{`[{"id":"a","question":"q"}]`, false},
		{`[{"id":"a","question":"q","expect_abstain":true,"expected_citations":[{"doc_number":"12"}]}]`, false},
		{`[{"id":"a","question":"q","expected_citations":[{"doc_number":" "}]}]`, false},
		{`[{"id":"a","question":"q","expect_abstain":true},{"id":"a","question":"q","expect_abstain":true}]`, false},
	} {
		_, err := LoadLegacy(strings.NewReader(tc.input))
		if (err == nil) != tc.ok {
			t.Errorf("%s: %v", tc.input, err)
		}
	}
}

func TestValidate_MissingOrAmbiguousEntryIsPerCase(t *testing.T) {
	cases := []Case{{ID: "a", ExpectedCitations: []Expectation{{DocNumber: "12"}}}}
	m := IdentityMap{Version: 1, LegacySHA256: "l"}
	report, err := m.Validate(cases, "l", IdentityInventory{})
	if err != nil || len(report["a"]) == 0 {
		t.Fatalf("missing entry: %v %v", report, err)
	}
	m.Entries = []MapEntry{{CaseID: "a"}, {CaseID: "a"}}
	report, err = m.Validate(cases, "l", IdentityInventory{})
	if err != nil || len(report["a"]) == 0 {
		t.Fatalf("duplicate entry: %v %v", report, err)
	}
}

func TestMatchLegacy_NumberEqualityThenContainment(t *testing.T) {
	for _, n := range []string{" 12/AB ", "XX12-ABYY"} {
		if r, _ := MatchLegacy(Expectation{DocNumber: "12/ab"}, []retrieval.Hit{{Number: n}}); r != 1 {
			t.Errorf("%q rank %d", n, r)
		}
	}
}
func TestMatchLegacy_ArticleClausePointTokens(t *testing.T) {
	e := Expectation{DocNumber: "12", Article: "3", Clause: "2", Point: "a"}
	for _, tc := range []struct {
		citation string
		want     int
	}{{"Điều (3)., khoản 2. điểm a", 1}, {"Điều 30 khoản 2 điểm a", 0}} {
		r, _ := MatchLegacy(e, []retrieval.Hit{{Number: "12", Citation: tc.citation}})
		if r != tc.want {
			t.Errorf("%q: %d", tc.citation, r)
		}
	}
}
func TestMatchLegacy_RelationOkDocumentOnly(t *testing.T) {
	h := retrieval.Hit{Identity: retrieval.Identity{DocumentKey: "s"}, Number: "other", Relations: []retrieval.Relation{{Complete: true, SourceKey: "s", SourceNumber: "other", TargetNumber: "12", SourceResolved: true, TargetResolved: true}}}
	if r, rel := MatchLegacy(Expectation{DocNumber: "12", RelationOK: true}, []retrieval.Hit{h}); r != 1 || !rel {
		t.Fatal(r, rel)
	}
	if r, _ := MatchLegacy(Expectation{DocNumber: "12", Article: "3", RelationOK: true}, []retrieval.Hit{h}); r != 0 {
		t.Fatal(r)
	}
}
func TestMatchProduct_ExactKeyAndAnchor(t *testing.T) {
	a := "a"
	e := MapEntry{State: "resolved", Alternatives: []Alternative{{Country: "vn", DocumentKey: "d", Anchor: &a, Kind: "provision"}}}
	h := retrieval.Hit{Identity: retrieval.Identity{Country: "vn", DocumentKey: "d", Anchor: "a"}}
	if MatchProduct(e, []retrieval.Hit{h}) != 1 {
		t.Fatal("miss")
	}
	h.Identity.Anchor = "b"
	if MatchProduct(e, []retrieval.Hit{h}) != 0 {
		t.Fatal("wrong anchor")
	}
}
func TestMatchProduct_NoSubstringCredit(t *testing.T) {
	e := MapEntry{State: "resolved", Alternatives: []Alternative{{Country: "vn", DocumentKey: "09/2020/TT-NHNN", Kind: "document"}}}
	if MatchProduct(e, []retrieval.Hit{{Identity: retrieval.Identity{Country: "vn", DocumentKey: "109/2020/TT-NHNN"}}}) != 0 {
		t.Fatal("substring credited")
	}
}
func TestMatchProduct_ParentChildCreditNeedsCompleteChild(t *testing.T) {
	a := "child"
	e := MapEntry{State: "resolved", Alternatives: []Alternative{{Country: "vn", DocumentKey: "d", Anchor: &a, Kind: "provision"}}}
	h := retrieval.Hit{Identity: retrieval.Identity{Country: "vn", DocumentKey: "d", Anchor: "parent"}, Fragments: []retrieval.Fragment{{Identity: retrieval.Identity{Country: "vn", DocumentKey: "d", Anchor: "child"}}}}
	if MatchProduct(e, []retrieval.Hit{h}) != 0 {
		t.Fatal("partial credited")
	}
	h.Fragments[0].Complete = true
	if MatchProduct(e, []retrieval.Hit{h}) != 1 {
		t.Fatal("complete missed")
	}
}
func TestMatchProduct_AbsentAlwaysMiss(t *testing.T) {
	if MatchProduct(MapEntry{State: "absent"}, []retrieval.Hit{{}}) != 0 {
		t.Fatal("absent matched")
	}
}
func TestMatchProduct_RelationAlternativeNeedsEdgeAndHash(t *testing.T) {
	target := "t"
	e := MapEntry{State: "resolved", Alternatives: []Alternative{{Country: "vn", DocumentKey: "t", Kind: "relation", SourceKey: "s", TargetKey: "t", RelationKind: "amends", EvidenceSHA256: "hash"}}}
	h := retrieval.Hit{Identity: retrieval.Identity{Country: "vn", DocumentKey: "s"}, Relations: []retrieval.Relation{{Complete: true, SourceKey: "s", TargetKey: &target, Kind: "amends", EvidenceSHA256: "wrong"}}}
	if MatchProduct(e, []retrieval.Hit{h}) != 0 {
		t.Fatal("wrong hash")
	}
	h.Relations[0].EvidenceSHA256 = "hash"
	if MatchProduct(e, []retrieval.Hit{h}) != 1 {
		t.Fatal("edge missed")
	}
}
func TestMetrics_RecallMicroMacroMRR(t *testing.T) {
	m := Metrics{Cases: []CaseScore{{Ranks: []int{1, 0}}, {Ranks: []int{2}}}}
	micro, macro := m.RecallAt(8)
	if *micro != 2.0/3 || *macro != 0.75 || *m.MRRAt8() != 0.75 {
		t.Fatal(micro, macro, m.MRRAt8())
	}
}
func TestMetrics_PrecisionCountsEveryPrimaryHitNullOnZero(t *testing.T) {
	m := Metrics{}
	if m.CurrentLawPrecision(false) != nil {
		t.Fatal("empty")
	}
	m.Cases = []CaseScore{{Response: retrieval.Response{Hits: []retrieval.Hit{{Stamp: retrieval.Validity{Current: true, StatusClass: "in_force"}}, {}}}}}
	if *m.CurrentLawPrecision(false) != 0.5 {
		t.Fatal("precision")
	}
}
func TestMetrics_LegacyPrecisionTrimsTrailingNonCurrent(t *testing.T) {
	m := Metrics{Cases: []CaseScore{{Response: retrieval.Response{Hits: []retrieval.Hit{{Stamp: retrieval.Validity{Current: true, StatusClass: "in_force"}}, {}}}}}}
	if *m.CurrentLawPrecision(true) != 1 {
		t.Fatal("trim")
	}
}
func TestMetrics_AbstentionConfusion(t *testing.T) {
	m := Metrics{Cases: []CaseScore{{Negative: true, Response: retrieval.Response{Abstain: true}}, {Negative: true, Response: retrieval.Response{Hits: []retrieval.Hit{{}}}}, {Response: retrieval.Response{Abstain: true}}, {Response: retrieval.Response{Hits: []retrieval.Hit{{}}}}}}
	a := m.Abstention()
	if a.TP != 1 || a.FN != 1 || a.FP != 1 || a.TN != 1 {
		t.Fatal(a)
	}
}
func TestQuantile_NearestRank(t *testing.T) {
	if *Quantile([]float64{1, 2, 3, 4}, 0.95) != 4 || Quantile(nil, 0.5) != nil {
		t.Fatal("quantile")
	}
}

func TestValidate_AbsentNamesAllNumbers(t *testing.T) {
	cases := []Case{{ID: "a", ExpectedCitations: []Expectation{{DocNumber: "12", AltDocNumbers: []string{"13"}}}}}
	m := IdentityMap{Version: 2, LegacySHA256: "l", Entries: []MapEntry{{CaseID: "a", State: "absent", ExpectedNumber: "12", Reason: "searched 12", ReviewedAt: "2026-01-01"}}}
	report, err := m.Validate(cases, "l", IdentityInventory{})
	if err != nil || len(report["a"]) == 0 {
		t.Fatal("absence omits alternate number")
	}
	m.Entries[0].Reason = "searched 12 and 13"
	report, err = m.Validate(cases, "l", IdentityInventory{})
	if err != nil || len(report["a"]) != 0 {
		t.Fatalf("report %v: %v", report, err)
	}
}
func TestValidate_AbsentNumberPresentOnlyWithProvisions(t *testing.T) {
	cases := []Case{{ID: "a", ExpectedCitations: []Expectation{{DocNumber: "12", Article: "5"}}}}
	m := IdentityMap{Version: 2, LegacySHA256: "l", Entries: []MapEntry{{CaseID: "a", State: "absent", ExpectedNumber: "12", Reason: "searched 12", ReviewedAt: "2026-01-01"}}}
	inventory := IdentityInventory{Documents: map[string]IdentityDocument{"vn:x:1": {Number: "12"}}}
	report, err := m.Validate(cases, "l", inventory)
	if err != nil || len(report["a"]) != 0 {
		t.Fatalf("document without provisions contradicted absence: %v %v", report, err)
	}
	for _, documentLevel := range []Expectation{{DocNumber: "12"}, {DocNumber: "12", Article: "5", RelationOK: true}} {
		cases[0].ExpectedCitations[0] = documentLevel
		report, err = m.Validate(cases, "l", inventory)
		if err != nil || len(report["a"]) != 1 || report["a"][0].Field != "state" {
			t.Fatalf("document-level expectation %+v accepted as absent: %v %v", documentLevel, report, err)
		}
	}
	cases[0].ExpectedCitations[0] = Expectation{DocNumber: "12", Article: "5"}
	inventory.Documents["vn:x:1"] = IdentityDocument{Number: "12", Anchors: map[string]bool{"dieu-1": true}}
	report, err = m.Validate(cases, "l", inventory)
	if err != nil || len(report["a"]) != 1 || report["a"][0].Field != "state" {
		t.Fatalf("document with provisions accepted as absent: %v %v", report, err)
	}
}
func TestMetrics_UnknownIsNotCurrent(t *testing.T) {
	m := Metrics{Cases: []CaseScore{{Response: retrieval.Response{Hits: []retrieval.Hit{{Stamp: retrieval.Validity{Current: true}}}}}}}
	if *m.CurrentLawPrecision(false) != 0 {
		t.Fatal("unknown counted current")
	}
}
func TestMetrics_EvidenceCharacters(t *testing.T) {
	m := Metrics{Cases: []CaseScore{{Response: retrieval.Response{Hits: []retrieval.Hit{{Text: "đi", Relations: []retrieval.Relation{{Complete: true, Evidence: "á"}}}}, NonCurrent: []retrieval.Hit{{Text: "x"}}}}, {}}}
	v := m.EvidenceCharacters()
	if len(v) != 2 || v[0] != 4 || v[1] != 0 {
		t.Fatal(v)
	}
}
func TestMatchProduct_RelationAnchorNamesMappedEndpoint(t *testing.T) {
	target, anchor := "t", "child"
	e := MapEntry{State: "resolved", Alternatives: []Alternative{{Country: "vn", DocumentKey: "t", Anchor: &anchor, Kind: "relation", SourceKey: "s", TargetKey: "t", RelationKind: "amends", EvidenceSHA256: "hash"}}}
	h := retrieval.Hit{Identity: retrieval.Identity{Country: "vn", DocumentKey: "s", Anchor: "source"}, Relations: []retrieval.Relation{{Complete: true, SourceKey: "s", TargetKey: &target, TargetAnchor: &anchor, Kind: "amends", EvidenceSHA256: "hash"}}}
	if MatchProduct(e, []retrieval.Hit{h}) != 1 {
		t.Fatal("mapped target anchor missed")
	}
	wrong := "other"
	h.Relations[0].TargetAnchor = &wrong
	if MatchProduct(e, []retrieval.Hit{h}) != 0 {
		t.Fatal("wrong target anchor credited")
	}
}

func TestMatchProduct_RelationNeedsCompleteEvidence(t *testing.T) {
	target := "t"
	e := MapEntry{State: "resolved", Alternatives: []Alternative{{Country: "vn", DocumentKey: "t", Kind: "relation", SourceKey: "s", TargetKey: "t", RelationKind: "amends", EvidenceSHA256: "hash"}}}
	h := retrieval.Hit{Identity: retrieval.Identity{Country: "vn", DocumentKey: "s"}, Relations: []retrieval.Relation{{SourceKey: "s", TargetKey: &target, Kind: "amends", EvidenceSHA256: "hash", Complete: false}}}
	if MatchProduct(e, []retrieval.Hit{h}) != 0 {
		t.Fatal("cut relation evidence earned credit")
	}
	h.Relations[0].Complete = true
	if MatchProduct(e, []retrieval.Hit{h}) != 1 {
		t.Fatal("complete relation evidence missed")
	}
}

func TestValidate_EvidenceURLOptionalNodeIDOnlyForDocuments(t *testing.T) {
	cases := []Case{{ID: "a", ExpectedCitations: []Expectation{{DocNumber: "12"}}}}
	entry := MapEntry{CaseID: "a", State: "resolved", ExpectedNumber: "12", Reason: "reviewed", ReviewedAt: "2026-01-01", Evidence: []Evidence{{DocumentKey: "d", DocumentNumber: "12", FetchSHA256: "hash", QuotedSpan: "title"}}, Alternatives: []Alternative{{Country: "vn", DocumentKey: "d", DocumentNumber: "12", Kind: "document"}}}
	m := IdentityMap{Version: 2, LegacySHA256: "l", Entries: []MapEntry{entry}}
	inv := IdentityInventory{Documents: map[string]IdentityDocument{"d": {Number: "12", Anchors: map[string]bool{"dieu-3": true}, Nodes: map[string]bool{"n1": true}}}}
	report, err := m.Validate(cases, "l", inv)
	if err != nil || len(report["a"]) != 0 {
		t.Fatalf("document: %v %v", report, err)
	}
	anchor := "dieu-3"
	m.Entries[0].Alternatives[0].Kind = "provision"
	m.Entries[0].Alternatives[0].Anchor = &anchor
	report, err = m.Validate(cases, "l", inv)
	if err != nil || len(report["a"]) == 0 {
		t.Fatal("missing node accepted")
	}
	m.Entries[0].Evidence[0].NodeID = "n1"
	report, err = m.Validate(cases, "l", inv)
	if err != nil || len(report["a"]) != 0 {
		t.Fatalf("provision: %v %v", report, err)
	}
}
