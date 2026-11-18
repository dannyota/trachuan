package retrieval

import (
	"testing"
	"time"
)

func TestNodeDocumentStatus_ServedAtDate(t *testing.T) {
	end := statusDay(2026, time.July, 1)
	start := statusDay(2026, time.December, 1)
	today := Date{2026, time.November, 18}
	for _, tc := range []struct {
		name    string
		source  Document
		class   string
		current bool
	}{
		{"open", Document{StatusClass: "in_force", IsCurrent: true}, "in_force", true},
		{"passed end", Document{StatusClass: "in_force", IsCurrent: true, EffectiveTo: end}, "expired", false},
		{"partial passed end", Document{StatusClass: "partial", IsCurrent: true, EffectiveTo: end}, "expired", false},
		{"future start", Document{StatusClass: "in_force", IsCurrent: true, EffectiveFrom: start}, "not_yet", false},
		{"conflict", Document{StatusClass: "in_force", IsCurrent: true, ValidityConflict: true}, "in_force", false},
		{"stored expired", Document{StatusClass: "expired"}, "expired", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got NodeDocument
			nodeDocumentStatus(&got, tc.source, today)
			if got.StatusClass != tc.class || got.IsCurrent != tc.current {
				t.Fatalf("%+v", got)
			}
		})
	}
}

func TestNodeDocumentStatus_AgreesWithStamp(t *testing.T) {
	dates := []*time.Time{nil, statusDay(2026, time.July, 1), statusDay(2026, time.November, 18), statusDay(2026, time.November, 19)}
	today := Date{2026, time.November, 18}
	for _, class := range []string{"in_force", "partial", "expired", "not_yet", "suspended", "unknown"} {
		for _, from := range dates {
			for _, to := range dates {
				for _, flags := range [][2]bool{{true, false}, {true, true}, {false, false}} {
					source := Document{StatusClass: class, IsCurrent: flags[0], ValidityConflict: flags[1], EffectiveFrom: from, EffectiveTo: to}
					want, _ := stamp(source, Arms{Date: today})
					var got NodeDocument
					nodeDocumentStatus(&got, source, today)
					if got.StatusClass != want.StatusClass || got.IsCurrent != want.Current {
						t.Fatalf("source %+v: node %+v, stamp %+v", source, got, want)
					}
				}
			}
		}
	}
}
