package loadrun

import "testing"

func TestReport_PhaseDErrors(t *testing.T) {
	r := recorder{records: []Record{
		{Phase: "A", Stream: "search", Status: 200, DurationMS: 10},
		{Phase: "D", Stream: "search", Status: 200, DurationMS: 10},
		{Phase: "D", Stream: "search", Status: 429, DurationMS: 20},
		{Phase: "D", Stream: "search", Status: 503, DurationMS: 30},
		{Phase: "D", Stream: "search", Status: 0, DurationMS: 40, Aborted: true},
	}}
	report := r.report()
	if report.Gates["D/search/errors"] != "fail" {
		t.Fatalf("D errors = %s", report.Gates["D/search/errors"])
	}
	if report.Phases["D"]["search"].P99MS != 40 {
		t.Fatalf("D p99 = %v", report.Phases["D"]["search"].P99MS)
	}
}
