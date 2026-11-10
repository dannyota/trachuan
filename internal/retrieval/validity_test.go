package retrieval

import (
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
