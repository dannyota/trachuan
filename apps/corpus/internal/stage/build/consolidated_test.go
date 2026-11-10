package build

import (
	"testing"
	"time"
)

func TestConsolidatedStatus_Branches(t *testing.T) {
	date := func(s string) *time.Time {
		d, e := time.Parse("2006-01-02", s)
		if e != nil {
			t.Fatal(e)
		}
		return &d
	}
	for _, tc := range []struct {
		name          string
		own           *time.Time
		targetCurrent bool
		targetCount   int
		others        []*time.Time
		class, gap    string
	}{
		{"current", date("2024-01-01"), true, 1, nil, "in_force", ""},
		{"expired", date("2024-01-01"), false, 1, nil, "expired", ""},
		{"newer", date("2024-01-01"), true, 1, []*time.Time{date("2024-02-01")}, "expired", ""},
		{"unresolved", nil, false, 0, nil, "unknown", "consolidated_status_unresolved"},
		{"missing date", nil, true, 1, []*time.Time{date("2024-02-01")}, "unknown", "consolidated_status_unordered"},
		{"same date", date("2024-01-01"), true, 1, []*time.Time{date("2024-01-01")}, "unknown", "consolidated_status_unordered"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			class, gap := consolidatedStatus(tc.own, tc.targetCurrent, tc.targetCount, tc.others)
			if class != tc.class || gap != tc.gap {
				t.Fatalf("got %s %s", class, gap)
			}
		})
	}
}
