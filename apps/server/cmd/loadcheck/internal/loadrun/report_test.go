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

func TestReport_SearchLimitsByPhase(t *testing.T) {
	r := recorder{records: []Record{
		{Phase: "A", Stream: "search", Status: 200, DurationMS: 1000},
		{Phase: "B", Stream: "search", Status: 200, DurationMS: 1250},
		{Phase: "C", Stream: "search", Status: 200, DurationMS: 1251},
	}}
	report := r.report()
	if report.Gates["A/search/latency"] != "pass" || report.Gates["B/search/latency"] != "pass" || report.Gates["C/search/latency"] != "fail" {
		t.Fatalf("gates %v", report.Gates)
	}
	r = recorder{records: []Record{{Phase: "A", Stream: "search", Status: 200, DurationMS: 1001}}}
	if r.report().Gates["A/search/latency"] != "fail" {
		t.Fatal("A over 1000 ms passed")
	}
}
