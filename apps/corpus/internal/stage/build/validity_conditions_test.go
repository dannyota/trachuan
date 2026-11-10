package build

import (
	"reflect"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/validity"
)

func TestRuleA_Conditions(t *testing.T) {
	base := testDoc("a", "1/2020/NĐ-CP", "2020-01-01", "2021-01-01")
	base.statusCode = "CCHL"
	base.sourceClass = "not_yet"
	base.article = testArticle("1/2/2020", "")
	for _, tc := range []struct {
		name   string
		change func(*validityDocument)
		want   bool
	}{
		{"not yet", func(*validityDocument) {}, true},
		{"empty status", func(d *validityDocument) { d.sourceClass = "unknown"; d.statusCode = "" }, true},
		{"adverse name", func(d *validityDocument) {
			d.sourceClass = "unknown"
			d.statusCode = ""
			d.statusName = "Hết hiệu lực"
		}, false},
		{"already current", func(d *validityDocument) { d.sourceClass = "in_force"; d.statusCode = "CHL" }, false},
		{"missing issue", func(d *validityDocument) { d.issue = "" }, false},
		{"before issue", func(d *validityDocument) { d.issue = "2020-03-01" }, false},
		{"raw end", func(d *validityDocument) { d.rawTo = "bad" }, false},
		{"end", func(d *validityDocument) { d.to = "2022-01-01" }, false},
		{"date gap", func(d *validityDocument) { d.dateGap = true }, false},
		{"no article", func(d *validityDocument) { d.article = nil }, false},
		{"unresolved", func(d *validityDocument) { d.article = testArticle("1/2/2020", "Văn bản cũ hết hiệu lực.") }, false},
		{"consolidated", func(d *validityDocument) { d.consolidated = true }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := base
			tc.change(&d)
			v := runTest(t, []validityDocument{d}, nil, nil)
			got := v.documents["a"].values.Basis == "derived_from_text"
			if got != tc.want {
				t.Fatalf("got %v: %+v", got, v)
			}
		})
	}
}
func TestRuleB_EarliestAndIdentity(t *testing.T) {
	target := testDoc("a", "1/2020/NĐ-CP", "2020-01-01", "2020-01-01")
	for _, tc := range []struct {
		name   string
		mutate func(*validityDocument)
		want   string
	}{
		{"match", func(*validityDocument) {}, "2021-02-01"},
		{"wrong number", func(d *validityDocument) { d.number = "9/2020/NĐ-CP" }, ""},
		{"wrong type", func(d *validityDocument) { d.docType = "Luật" }, ""},
		{"wrong date", func(d *validityDocument) { d.issue = "2020-01-02" }, ""},
		{"target not started", func(d *validityDocument) { d.from = "2021-02-01" }, ""},
		{"earlier end", func(d *validityDocument) { d.to = "2020-12-01" }, "2020-12-01"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := target
			tc.mutate(&d)
			actor := testDoc("b", "2/2021/NĐ-CP", "2021-01-01", "2021-02-01")
			actor.article = testArticle("1/2/2021", replacement(target.number, "1/1/2020"))
			v := runTest(t, []validityDocument{d, actor}, nil, nil)
			if got := dateValue(v.documents["a"].values.To); got != tc.want {
				t.Fatalf("end=%s, result=%+v", got, v)
			}
		})
	}
}
func TestValidity_ManualStartPrecedesRuleA(t *testing.T) {
	d := testDoc("a", "1/2020/NĐ-CP", "2020-01-01", "2021-01-01")
	d.statusCode = "CCHL"
	d.sourceClass = "not_yet"
	d.article = testArticle("1/2/2020", "")
	manual := "2020-03-01"
	v := runTest(t, []validityDocument{d}, nil, map[string]source.Correction{"a": {Set: source.CorrectionSet{EffFrom: &manual}}})
	if dateValue(v.documents["a"].values.From) != manual || v.counts.ruleA != 0 {
		t.Fatal(v)
	}
}
func TestRuleC_RepealsOnlyReview(t *testing.T) {
	a := testDoc("a", "1/2020/NĐ-CP", "2020-01-01", "2020-01-01")
	b := testDoc("b", "2/2021/NĐ-CP", "2021-01-01", "2021-02-01")
	v := runTest(t, []validityDocument{a, b}, []validityRelation{{source: "b", target: "a", kind: "repeals", id: 1}}, nil)
	if v.documents["a"].values.Conflict || !hasReview(v, "repeals_current") {
		t.Fatal(v)
	}
}

func TestRuleB_AmbiguousAndIssuerCode(t *testing.T) {
	a := testDoc("a", "1/2020/NĐ-CP", "2020-01-01", "2020-01-01")
	actor := testDoc("b", "2/2021/NĐ-CP", "2021-01-01", "2021-02-01")
	actor.article = testArticle("1/2/2021", "Nghị định này thay thế Nghị định số 1/2020/NĐ-CP ngày 1/1/2020.")
	v := runTest(t, []validityDocument{a, actor}, nil, nil)
	if dateValue(v.documents["a"].values.To) != "2021-02-01" {
		t.Fatal(v)
	}
	other := a
	other.key = "other"
	v = runTest(t, []validityDocument{a, other, actor}, nil, nil)
	if v.documents["a"].values.To != nil || !hasReview(v, "rule_b_skipped") {
		t.Fatal(v)
	}
}
func TestRuleB_EarliestEventWins(t *testing.T) {
	a := testDoc("a", "1/2020/NĐ-CP", "2020-01-01", "2020-01-01")
	b := testDoc("b", "2/2021/NĐ-CP", "2021-01-01", "2021-02-01")
	b.article = testArticle("1/2/2021", replacement(a.number, "1/1/2020"))
	c := testDoc("c", "3/2022/NĐ-CP", "2022-01-01", "2022-02-01")
	c.article = testArticle("1/2/2022", replacement(a.number, "1/1/2020"))
	v := runTest(t, []validityDocument{c, a, b}, nil, nil)
	if dateValue(v.documents["a"].values.To) != "2021-02-01" || v.counts.ruleB != 2 {
		t.Fatal(v)
	}
}
func TestRuleC_FutureAndNonCurrentActor(t *testing.T) {
	a := testDoc("a", "1/2020/NĐ-CP", "2020-01-01", "2020-01-01")
	b := testDoc("b", "2/2027/NĐ-CP", "2027-01-01", "2027-02-01")
	rel := []validityRelation{{source: "b", target: "a", kind: "replaces", id: 1}}
	v := runTest(t, []validityDocument{a, b}, rel, nil)
	if v.documents["a"].values.Conflict {
		t.Fatal(v)
	}
	b.from = "2021-01-01"
	b.sourceClass = "expired"
	v = runTest(t, []validityDocument{a, b}, rel, nil)
	if v.documents["a"].values.Conflict {
		t.Fatal(v)
	}
}
func TestRuleC_WholeEventConfirmsRelation(t *testing.T) {
	a := testDoc("a", "1/2020/NĐ-CP", "2020-01-01", "2020-01-01")
	b := testDoc("b", "2/2021/NĐ-CP", "2021-01-01", "2021-02-01")
	b.article = testArticle("1/2/2021", replacement(a.number, "1/1/2020"))
	v := runTest(t, []validityDocument{a, b}, []validityRelation{{source: "b", target: "a", kind: "replaces", id: 1}}, nil)
	if v.documents["a"].values.Conflict || v.counts.ruleC != 0 {
		t.Fatal(v)
	}
}
func TestValidity_ReviewAndCounts(t *testing.T) {
	a := testDoc("a", "1/2020/NĐ-CP", "2020-01-01", "2020-01-01")
	a.sourceClass = "unknown"
	a.statusCode = ""
	b := testDoc("b", "2/2021/NĐ-CP", "2021-01-01", "2021-02-01")
	b.article = testArticle("1/2/2021", "Văn bản khác hết hiệu lực.")
	v := runTest(t, []validityDocument{a, b}, nil, nil)
	if !hasReview(v, "unknown") || !hasReview(v, "unresolved_article") || v.counts.review["unknown"] != 1 || v.counts.basis["source_status"] != 2 {
		t.Fatal(v)
	}
	for _, r := range v.reviews {
		if r.key == "b" && r.proposed == nil {
			t.Fatal("missing proposed correction")
		}
	}
}

func TestCorrections_HoldAlsoSuppressesCommencement(t *testing.T) {
	d := testDoc("a", "1/2020/NĐ-CP", "2020-01-01", "2021-01-01")
	d.statusCode = "CCHL"
	d.sourceClass = "not_yet"
	d.article = testArticle("1/2/2020", "")
	v := runTest(t, []validityDocument{d}, nil, map[string]source.Correction{"a": {Hold: true}})
	if v.documents["a"].values.Class != "not_yet" {
		t.Fatal(v)
	}
}
func TestValidity_ManualCanSetConflict(t *testing.T) {
	d := testDoc("a", "1/2020/NĐ-CP", "2020-01-01", "2020-01-01")
	eligible := false
	v := runTest(t, []validityDocument{d}, nil, map[string]source.Correction{"a": {Eligible: &eligible}})
	if !v.documents["a"].values.Conflict {
		t.Fatal(v)
	}
}
func TestValidity_ShuffledRelations(t *testing.T) {
	a := testDoc("a", "1/2020/NĐ-CP", "2020-01-01", "2020-01-01")
	b := testDoc("b", "2/2021/NĐ-CP", "2021-01-01", "2021-02-01")
	c := testDoc("c", "3/2022/NĐ-CP", "2022-01-01", "2022-02-01")
	r1 := []validityRelation{{source: "b", target: "a", kind: "replaces", id: 2}, {source: "c", target: "a", kind: "replaces", id: 1}}
	r2 := []validityRelation{r1[1], r1[0]}
	x := runTest(t, []validityDocument{a, b, c}, r1, nil)
	y := runTest(t, []validityDocument{c, a, b}, r2, nil)
	if !reflect.DeepEqual(x, y) {
		t.Fatal("result depends on relation order")
	}
}

func TestValidity_PreservesDerivedConsolidatedStatus(t *testing.T) {
	d := testDoc("a", "1/2020/NĐ-CP", "2020-01-01", "2020-01-01")
	d.sourceClass = "unknown"
	d.initialClass = "in_force"
	d.basis = "derived_from_consolidated"
	d.consolidated = true
	v := runTest(t, []validityDocument{d}, nil, nil)
	if v.documents["a"].values.Class != "in_force" || v.documents["a"].values.Basis != "derived_from_consolidated" {
		t.Fatal(v)
	}
}

func TestReview_IncludesCommencementEvidence(t *testing.T) {
	d := testDoc("a", "1/2020/NĐ-CP", "2020-01-01", "2020-03-01")
	d.article = testArticle("1/2/2020", "")
	d.quote = "Nghị định này có hiệu lực từ ngày 1/2/2020."
	d.extractHash = "extract"
	v := runTest(t, []validityDocument{d}, nil, nil)
	if len(v.reviews) == 0 || len(v.reviews[0].sentences) == 0 || v.reviews[0].sentences[0] != d.quote || v.reviews[0].extractHash != "extract" {
		t.Fatal(v.reviews)
	}
}

func TestReview_HeldTargetListsSkippedEvent(t *testing.T) {
	a := testDoc("a", "1/2020/NĐ-CP", "2020-01-01", "2020-01-01")
	b := testDoc("b", "2/2021/NĐ-CP", "2021-01-01", "2021-02-01")
	b.article = testArticle("1/2/2021", replacement(a.number, "1/1/2020"))
	v := runTest(t, []validityDocument{a, b}, nil, map[string]source.Correction{"a": {Hold: true}})
	if !hasReview(v, "rule_b_skipped") {
		t.Fatal(v.reviews)
	}
}

func TestRuleA_IncomingRelationKinds(t *testing.T) {
	for _, kind := range []string{"amends_supplements", "suspends", "replaces", "repeals"} {
		t.Run(kind, func(t *testing.T) {
			d := testDoc("a", "1/2020/NĐ-CP", "2020-01-01", "")
			d.statusCode = "CCHL"
			d.sourceClass = "not_yet"
			d.article = testArticle("1/2/2020", "")
			actor := testDoc("b", "2/2021/NĐ-CP", "2021-01-01", "2021-01-01")
			v := runTest(t, []validityDocument{d, actor}, []validityRelation{{source: "b", target: "a", kind: kind, id: 1}}, nil)
			if v.documents["a"].values.Class != "not_yet" {
				t.Fatal(v)
			}
		})
	}
}
func TestRuleB_MissingStatedIssueDateUnresolved(t *testing.T) {
	a := testDoc("a", "1/2020/NĐ-CP", "2020-01-01", "2020-01-01")
	b := testDoc("b", "2/2021/NĐ-CP", "2021-01-01", "2021-02-01")
	b.article = testArticle("1/2/2021", "Nghị định này thay thế Nghị định số 1/2020/NĐ-CP.")
	v := runTest(t, []validityDocument{a, b}, nil, nil)
	if v.documents["a"].values.To != nil || !hasReview(v, "unresolved_article") {
		t.Fatal(v)
	}
}
func TestDecisions_QuoteIsCapped(t *testing.T) {
	d := testDoc("a", "1/2020/NĐ-CP", "2020-01-01", "2021-01-01")
	d.statusCode = "CCHL"
	d.sourceClass = "not_yet"
	d.article = testArticle("1/2/2020", "")
	d.quote = strings.Repeat("đ", 500)
	v := runTest(t, []validityDocument{d}, nil, nil)
	if n := len([]rune(v.documents["a"].history[1].quote)); n != 400 {
		t.Fatalf("quote length %d", n)
	}
}

func TestReview_RecordsBothDocumentsEvidence(t *testing.T) {
	a := testDoc("a", "1/2020/NĐ-CP", "2020-01-01", "2020-01-01")
	a.fetchHash = "target-hash"
	b := testDoc("b", "2/2021/NĐ-CP", "2021-01-01", "2021-02-01")
	b.fetchHash = "actor-hash"
	v := runTest(t, []validityDocument{a, b}, []validityRelation{{source: "b", target: "a", kind: "replaces", id: 1}}, nil)
	for _, r := range v.reviews {
		if r.kind == "rule_c_conflict" {
			if r.targetStatusCode != "CHL" || r.targetRawFrom != "2020-01-01" || r.targetFetchHash != "target-hash" {
				t.Fatal(r)
			}
			return
		}
	}
	t.Fatal("missing conflict review")
}

func TestRuleB_RecordsRelationAgreement(t *testing.T) {
	a := testDoc("a", "1/2020/NĐ-CP", "2020-01-01", "2020-01-01")
	b := testDoc("b", "2/2021/NĐ-CP", "2021-01-01", "2021-02-01")
	b.article = testArticle("1/2/2021", replacement(a.number, "1/1/2020"))
	v := runTest(t, []validityDocument{a, b}, []validityRelation{{source: "b", target: "a", kind: "replaces", id: 7}}, nil)
	h := v.documents["a"].history
	if len(h) != 2 || h[1].relationID != 7 {
		t.Fatal(h)
	}
}

func TestReview_HoldDoesNotHideCurrentRepeal(t *testing.T) {
	a := testDoc("a", "1/2020/NĐ-CP", "2020-01-01", "2020-01-01")
	b := testDoc("b", "2/2021/NĐ-CP", "2021-01-01", "2021-02-01")
	v := runTest(t, []validityDocument{a, b}, []validityRelation{{source: "b", target: "a", kind: "repeals", id: 1}}, map[string]source.Correction{"a": {Hold: true}})
	if !hasReview(v, "repeals_current") {
		t.Fatal(v.reviews)
	}
}

func TestCorrections_MatchingAutomaticFieldDoesNotConflict(t *testing.T) {
	a := testDoc("a", "1/2020/NĐ-CP", "2020-01-01", "2020-01-01")
	b := testDoc("b", "2/2021/NĐ-CP", "2021-01-01", "2021-02-01")
	b.article = testArticle("1/2/2021", replacement(a.number, "1/1/2020"))
	end := "2021-02-01"
	status := "expired"
	v := runTest(t, []validityDocument{a, b}, nil, map[string]source.Correction{"a": {Set: source.CorrectionSet{EffTo: &end, StatusClass: &status}}})
	if hasReview(v, "correction_conflict") {
		t.Fatal(v.reviews)
	}
}

func TestReview_KnownSkippedTargetEvidence(t *testing.T) {
	a := testDoc("a", "1/2020/NĐ-CP", "2020-01-01", "2020-01-01")
	a.issuer = "Bộ khác"
	a.fetchHash = "target-hash"
	b := testDoc("b", "2/2021/NĐ-CP", "2021-01-01", "2021-02-01")
	b.article = testArticle("1/2/2021", replacement(a.number, "1/1/2020"))
	v := runTest(t, []validityDocument{a, b}, nil, nil)
	for _, r := range v.reviews {
		if r.kind == "rule_b_skipped" {
			if r.targetKey != "a" || r.targetFetchHash != "target-hash" {
				t.Fatal(r)
			}
			return
		}
	}
	t.Fatal("missing skipped item")
}

func TestRuleC_PartialWithManyNumbers(t *testing.T) {
	a := testDoc("a", "1/2020/NĐ-CP", "2020-01-01", "2020-01-01")
	b := testDoc("b", "2/2021/NĐ-CP", "2021-01-01", "2021-02-01")
	text := "Điều 3 của " + strings.Repeat("5/2020/NĐ-CP ", 9) + "1/2020/NĐ-CP"
	b.article = &validity.Result{Usable: true, Commencement: validity.Date{Year: 2021, Month: 2, Day: 1}, Items: []validity.Item{{Scope: "partial", Text: text}}}
	v := runTest(t, []validityDocument{a, b}, []validityRelation{{source: "b", target: "a", kind: "replaces", id: 1}}, nil)
	if v.documents["a"].values.Conflict {
		t.Fatal(v)
	}
}

func TestRuleC_PartialNumberRequiresTokenBoundary(t *testing.T) {
	a := testDoc("a", "1/2020/NĐ-CP", "2020-01-01", "2020-01-01")
	b := testDoc("b", "2/2021/NĐ-CP", "2021-01-01", "2021-02-01")
	b.article = &validity.Result{Usable: true, Commencement: validity.Date{Year: 2021, Month: 2, Day: 1}, Items: []validity.Item{{Scope: "partial", Text: "Điều 3 của X1/2020/NĐ-CP"}}}
	v := runTest(t, []validityDocument{a, b}, []validityRelation{{source: "b", target: "a", kind: "replaces", id: 1}}, nil)
	if !v.documents["a"].values.Conflict {
		t.Fatal(v)
	}
}

func TestReview_HoldDoesNotHidePartialTarget(t *testing.T) {
	a := testDoc("a", "1/2020/NĐ-CP", "2020-01-01", "2020-01-01")
	a.sourceClass = "partial"
	b := testDoc("b", "2/2021/NĐ-CP", "2021-01-01", "2021-02-01")
	v := runTest(t, []validityDocument{a, b}, []validityRelation{{source: "b", target: "a", kind: "replaces", id: 1}}, map[string]source.Correction{"a": {Hold: true}})
	if !hasReview(v, "partial_target") {
		t.Fatal(v.reviews)
	}
}
