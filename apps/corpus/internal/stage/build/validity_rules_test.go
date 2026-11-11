package build

import (
	"reflect"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/validity"
)

func testArticle(date string, suffix string) *validity.Result {
	r := validity.Parse("Nghị định", "Điều 1. Hiệu lực thi hành\nNghị định này có hiệu lực từ ngày "+date+". "+suffix)
	return &r
}
func testDoc(key, number, issue, from string) validityDocument {
	return validityDocument{key: key, number: number, docType: "Nghị định", issuer: "Chính phủ", issue: issue, from: from, rawFrom: from, statusCode: "CHL", sourceClass: "in_force"}
}
func runTest(t *testing.T, docs []validityDocument, relations []validityRelation, corrections map[string]source.Correction) validityResult {
	t.Helper()
	v, err := runValidity(docs, relations, corrections, "2026-11-10")
	if err != nil {
		t.Fatal(err)
	}
	return v
}
func hasReview(r validityResult, kind string) bool {
	for _, v := range r.reviews {
		if v.kind == kind {
			return true
		}
	}
	return false
}
func dateValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
func replacement(number, date string) string {
	return "Nghị định này thay thế Nghị định số " + number + " ngày " + date + " của Chính phủ."
}

func TestValidity_ChainBothOrders(t *testing.T) {
	a := testDoc("a", "1/2020/NĐ-CP", "2020-01-01", "2020-01-01")
	b := testDoc("b", "2/2021/NĐ-CP", "2021-01-01", "2021-02-01")
	c := testDoc("c", "3/2022/NĐ-CP", "2022-01-01", "2022-02-01")
	b.article = testArticle("1/2/2021", replacement(a.number, "1/1/2020"))
	c.article = testArticle("1/2/2022", replacement(b.number, "1/1/2021"))
	x := runTest(t, []validityDocument{a, b, c}, nil, nil)
	y := runTest(t, []validityDocument{c, b, a}, nil, nil)
	if !reflect.DeepEqual(x, y) {
		t.Fatal("result depends on order")
	}
	if dateValue(x.documents["a"].values.To) != "2021-02-01" || dateValue(x.documents["b"].values.To) != "2022-02-01" {
		t.Fatalf("chain: %+v", x.documents)
	}
}
func TestValidity_ExpiredActorStillEmitsEvent(t *testing.T) {
	a := testDoc("a", "1/2020/NĐ-CP", "2020-01-01", "2020-01-01")
	b := testDoc("b", "2/2021/NĐ-CP", "2021-01-01", "2021-02-01")
	b.sourceClass = "expired"
	b.article = testArticle("1/2/2021", replacement(a.number, "1/1/2020"))
	v := runTest(t, []validityDocument{a, b}, nil, nil)
	if dateValue(v.documents["a"].values.To) != "2021-02-01" {
		t.Fatal(v.documents)
	}
}
func TestValidity_FutureReplacementKeepsPredecessorCurrent(t *testing.T) {
	a := testDoc("a", "1/2020/NĐ-CP", "2020-01-01", "2020-01-01")
	b := testDoc("b", "2/2027/NĐ-CP", "2027-01-01", "2027-02-01")
	b.article = testArticle("1/2/2027", replacement(a.number, "1/1/2020"))
	v := runTest(t, []validityDocument{a, b}, nil, nil)
	if d := v.documents["a"].values; !d.Current || dateValue(d.To) != "2027-02-01" {
		t.Fatal(d)
	}
}
func TestValidity_DateConflictMakesNoEvents(t *testing.T) {
	a := testDoc("a", "1/2020/NĐ-CP", "2020-01-01", "2020-01-01")
	b := testDoc("b", "2/2021/NĐ-CP", "2021-01-01", "2021-03-01")
	b.article = testArticle("1/2/2021", replacement(a.number, "1/1/2020"))
	v := runTest(t, []validityDocument{a, b}, nil, nil)
	if v.documents["a"].values.To != nil || !hasReview(v, "date_conflict") {
		t.Fatal(v)
	}
}
func TestValidity_IssuerMustBeVerified(t *testing.T) {
	a := testDoc("a", "1/2020/NĐ-CP", "2020-01-01", "2020-01-01")
	a.issuer = "Bộ khác"
	b := testDoc("b", "2/2021/NĐ-CP", "2021-01-01", "2021-02-01")
	b.article = testArticle("1/2/2021", replacement(a.number, "1/1/2020"))
	v := runTest(t, []validityDocument{a, b}, nil, nil)
	if v.documents["a"].values.To != nil || !hasReview(v, "rule_b_skipped") {
		t.Fatal(v)
	}
}
func TestValidity_AdverseStatusNameNotCandidate(t *testing.T) {
	d := testDoc("a", "1/2020/NĐ-CP", "2020-01-01", "2020-02-01")
	d.statusCode = ""
	d.statusName = "Đã hết hiệu lực"
	d.sourceClass = "unknown"
	d.article = testArticle("1/2/2020", "")
	v := runTest(t, []validityDocument{d}, nil, nil)
	if v.documents["a"].values.Class != "unknown" || !hasReview(v, "rule_a_status_name") {
		t.Fatal(v)
	}
}
func TestValidity_ClearedIntervalBlocksRuleA(t *testing.T) {
	d := testDoc("a", "1/2020/NĐ-CP", "2020-01-01", "")
	d.statusCode = "CCHL"
	d.sourceClass = "not_yet"
	d.dateGap = true
	d.article = testArticle("1/2/2020", "")
	v := runTest(t, []validityDocument{d}, nil, nil)
	if v.documents["a"].values.Class != "not_yet" || !hasReview(v, "rule_a_blocked") {
		t.Fatal(v)
	}
}
func TestValidity_IncomingRelationBlocksRuleA(t *testing.T) {
	d := testDoc("a", "1/2020/NĐ-CP", "2020-01-01", "")
	d.statusCode = "CCHL"
	d.sourceClass = "not_yet"
	d.article = testArticle("1/2/2020", "")
	b := testDoc("b", "2/2021/NĐ-CP", "2021-01-01", "2021-01-01")
	v := runTest(t, []validityDocument{d, b}, []validityRelation{{source: "b", target: "a", kind: "amends_supplements", id: 1}}, nil)
	if v.documents["a"].values.Class != "not_yet" || !hasReview(v, "rule_a_blocked") {
		t.Fatal(v)
	}
}
func TestRuleC_OnlyInForceTargetsWithoutPartialMention(t *testing.T) {
	a := testDoc("a", "1/2020/NĐ-CP", "2020-01-01", "2020-01-01")
	b := testDoc("b", "2/2021/NĐ-CP", "2021-01-01", "2021-02-01")
	b.article = testArticle("1/2/2021", "")
	rel := []validityRelation{{source: "b", target: "a", kind: "replaces", id: 3}}
	v := runTest(t, []validityDocument{a, b}, rel, nil)
	if !v.documents["a"].values.Conflict {
		t.Fatal(v)
	}
	a.sourceClass = "partial"
	v = runTest(t, []validityDocument{a, b}, rel, nil)
	if v.documents["a"].values.Conflict || !hasReview(v, "partial_target") {
		t.Fatal(v)
	}
	a.sourceClass = "in_force"
	b.article = testArticle("1/2/2021", "Nghị định này bãi bỏ Điều 3 của Nghị định số 1/2020/NĐ-CP.")
	v = runTest(t, []validityDocument{a, b}, rel, nil)
	if v.documents["a"].values.Conflict {
		t.Fatal(v)
	}
}
func TestCorrections_HoldSuppressesRules(t *testing.T) {
	a := testDoc("a", "1/2020/NĐ-CP", "2020-01-01", "2020-01-01")
	b := testDoc("b", "2/2021/NĐ-CP", "2021-01-01", "2021-02-01")
	b.article = testArticle("1/2/2021", replacement(a.number, "1/1/2020"))
	v := runTest(t, []validityDocument{a, b}, []validityRelation{{source: "b", target: "a", kind: "replaces", id: 1}}, map[string]source.Correction{"a": {Hold: true}})
	if v.documents["a"].values.To != nil || v.documents["a"].values.Conflict {
		t.Fatal(v)
	}
}
func TestValidity_EligibleOverridesConflict(t *testing.T) {
	a := testDoc("a", "1/2020/NĐ-CP", "2020-01-01", "2020-01-01")
	b := testDoc("b", "2/2021/NĐ-CP", "2021-01-01", "2021-02-01")
	b.article = testArticle("1/2/2021", "")
	yes := true
	v := runTest(t, []validityDocument{a, b}, []validityRelation{{source: "b", target: "a", kind: "replaces", id: 1}}, map[string]source.Correction{"a": {Eligible: &yes}})
	if v.documents["a"].values.Conflict {
		t.Fatal(v)
	}
}
func TestDecisions_HistoryOrderAndCap(t *testing.T) {
	a := testDoc("a", "1/2020/NĐ-CP", "2020-01-01", "2020-01-01")
	b := testDoc("b", "2/2021/NĐ-CP", "2021-01-01", "2021-02-01")
	b.article = testArticle("1/2/2021", "")
	rels := []validityRelation{}
	for i := int64(1); i <= 8; i++ {
		rels = append(rels, validityRelation{source: "b", target: "a", kind: "replaces", id: i})
	}
	_, err := runValidity([]validityDocument{a, b}, rels, nil, "2026-11-10")
	if err == nil || !strings.Contains(err.Error(), "a") {
		t.Fatalf("error=%v", err)
	}
	v := runTest(t, []validityDocument{a, b}, rels[:1], nil)
	h := v.documents["a"].history
	if len(h) != 2 || h[0].layer != "source" || h[1].layer != "C" {
		t.Fatal(h)
	}
}
func TestReview_ListsEverySkippedItem(t *testing.T) {
	a := testDoc("a", "1/2020/NĐ-CP", "2020-01-01", "2020-01-01")
	b := testDoc("b", "2/2021/NĐ-CP", "2021-01-01", "2021-02-01")
	b.article = testArticle("1/2/2021", replacement("3/2020/NĐ-CP", "1/1/2020")+" "+replacement("4/2020/NĐ-CP", "1/1/2020"))
	v := runTest(t, []validityDocument{a, b}, nil, nil)
	n := 0
	for _, r := range v.reviews {
		if r.kind == "rule_b_skipped" {
			n++
		}
	}
	if n != 2 {
		t.Fatalf("reviews=%+v", v.reviews)
	}
}

func TestCorrections_UnrelatedAutomaticChangeDoesNotConflict(t *testing.T) {
	a := testDoc("a", "1/2020/NĐ-CP", "2020-01-01", "2020-01-01")
	b := testDoc("b", "2/2021/NĐ-CP", "2021-01-01", "2021-02-01")
	b.article = testArticle("1/2/2021", replacement(a.number, "1/1/2020"))
	status := "expired"
	v := runTest(t, []validityDocument{a, b}, nil, map[string]source.Correction{"a": {Set: source.CorrectionSet{StatusClass: &status}}})
	if hasReview(v, "correction_conflict") {
		t.Fatal(v.reviews)
	}
}

func TestReview_ItemsWithoutOperativeDate(t *testing.T) {
	a := testDoc("a", "1/2020/NĐ-CP", "2020-01-01", "2020-01-01")
	b := testDoc("b", "2/2021/NĐ-CP", "2021-01-01", "2021-03-01")
	b.article = testArticle("1/2/2021", replacement(a.number, "1/1/2020"))
	v := runTest(t, []validityDocument{a, b}, nil, nil)
	if !hasReview(v, "rule_b_skipped") {
		t.Fatal(v.reviews)
	}
}
