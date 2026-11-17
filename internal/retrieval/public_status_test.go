package retrieval

import (
	"testing"
	"time"
)

func TestDocumentValidity_DateAndConflict(t *testing.T) {
	from := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 0, 1)
	for _, tc := range []struct {
		name              string
		date              Date
		conflict, current bool
	}{
		{"effective", Date{2099, 1, 1}, false, true},
		{"end excluded", Date{2099, 1, 2}, false, false},
		{"conflict", Date{2099, 1, 1}, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, gaps := DocumentValidity(Document{StatusClass: "in_force", StatusBasis: "manual_correction", IsCurrent: true, ValidityConflict: tc.conflict, EffectiveFrom: &from, EffectiveTo: &to}, tc.date)
			if got.Current != tc.current || got.Unresolved != tc.conflict || got.Basis != "source_status" || got.StatusBasis != "manual_correction" {
				t.Fatalf("status: %+v", got)
			}
			if tc.conflict && (len(gaps) != 1 || gaps[0].Kind != "validity_conflict") {
				t.Fatalf("gaps: %+v", gaps)
			}
		})
	}
}
