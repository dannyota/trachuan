package retrieval

import (
	"encoding/json"
	"testing"
	"time"
)

func TestStamp_EffectiveToIsFirstExcludedDay(t *testing.T) {
	from := time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2021, time.January, 1, 0, 0, 0, 0, time.UTC)
	doc := Document{StatusClass: "in_force", IsCurrent: true, EffectiveFrom: &from, EffectiveTo: &to}
	for _, tc := range []struct {
		date Date
		want bool
	}{{Date{2020, time.December, 31}, true}, {Date{2021, time.January, 1}, false}} {
		d := tc.date
		v, _ := stamp(doc, Arms{Date: d, AsOf: &d})
		if v.Current != tc.want {
			t.Errorf("as of %s: current %v, want %v", d.String(), v.Current, tc.want)
		}
	}
}

func TestValidity_StatusBasis(t *testing.T) {
	got, _ := stamp(Document{StatusClass: "unknown", StatusBasis: "derived_from_consolidated"}, Arms{})
	if got.StatusBasis != "derived_from_consolidated" {
		t.Fatalf("status basis = %q", got.StatusBasis)
	}
}

func TestShape_ConflictShownAsUnresolvedNonCurrent(t *testing.T) {
	a, recipe := shapeFixture("Nội dung")
	for i := range a.Primary.Dense {
		a.Primary.Dense[i].Document.ValidityConflict = true
	}
	for i := range a.Primary.Members {
		a.Primary.Members[i].Document.ValidityConflict = true
	}
	a.NonCurrent, a.Primary = a.Primary, ArmGroup{}
	a.IncludeNonCurrent = true
	got, err := Shape(a, recipe, 8, "hybrid")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Hits) != 0 || len(got.NonCurrent) != 1 || got.NonCurrent[0].Stamp.Current || !got.NonCurrent[0].Stamp.Unresolved {
		t.Fatalf("response: %+v", got)
	}
	d := Date{2025, time.January, 1}
	v, _ := stamp(Document{StatusClass: "in_force", IsCurrent: true, ValidityConflict: true}, Arms{AsOf: &d})
	if v.Unresolved {
		t.Fatalf("historical determination marked unresolved: %+v", v)
	}
}

func TestValidity_BothModesRoundTripDecisions(t *testing.T) {
	decision := ValidityDecision{Ordinal: 1, Layer: "source", RuleVersion: "1", Before: json.RawMessage(`{}`), After: json.RawMessage(`{}`)}
	for _, asOf := range []*Date{nil, {2025, time.January, 1}} {
		v, _ := stamp(Document{Decisions: []ValidityDecision{decision}}, Arms{AsOf: asOf})
		data, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		var got Validity
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatal(err)
		}
		if len(got.Decisions) != 1 || got.Decisions[0].Layer != "source" {
			t.Fatalf("decisions: %s", data)
		}
	}
}
