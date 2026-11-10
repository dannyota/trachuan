package discover

import (
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/stage"
)

func TestPlan_PrecedenceSkipGapIsProvisional(t *testing.T) {
	number, issuer, date := "42", "Government", "2026-01-01"
	r := HandOverRecord{State: "seed_primary", ProbeState: "complete", Number: "42", Issuer: "Government", Date: "2026-01-01", PrimaryID: "1", Comparisons: []HandOverComparison{{ID: "1", State: "match", Number: &number, Issuer: &issuer, Date: &date}}}
	gap := precedenceGap("vn:chinhphu:5", []handOverCandidate{{record: r}})
	want := stage.Gap{Kind: "source_replaced", Detail: "vn:chinhphu:5 -> vn:vbpl:1"}
	if gap == nil || *gap != want {
		t.Fatalf("gap=%+v want=%+v", gap, want)
	}
}

func TestPlan_PrecedenceSkipRequiresVerifiedIdentity(t *testing.T) {
	record := HandOverRecord{State: "seed_primary", ProbeState: "complete", Number: "42", Issuer: "Government", Date: "2026-01-01", PrimaryID: "1", Comparisons: []HandOverComparison{{ID: "1", State: "match"}}}
	if gap := precedenceGap("vn:chinhphu:5", []handOverCandidate{{record: record}}); gap != nil {
		t.Fatalf("skipped with missing comparison identity: %+v", gap)
	}
}

func TestPlan_PrecedenceSkipRejectsConflictingCandidates(t *testing.T) {
	number, issuer, date1, date2 := "42", "Government", "2026-01-01", "2026-01-02"
	candidate := func(date *string) handOverCandidate {
		return handOverCandidate{record: HandOverRecord{State: "seed_primary", ProbeState: "complete", Number: number, Issuer: issuer, Date: *date, PrimaryID: "1", Comparisons: []HandOverComparison{{ID: "1", State: "match", Number: &number, Issuer: &issuer, Date: date}}}}
	}
	if gap := precedenceGap("vn:chinhphu:5", []handOverCandidate{candidate(&date1), candidate(&date2)}); gap != nil {
		t.Fatalf("skipped conflicting candidates: %+v", gap)
	}
}

func TestPlan_PrecedenceSkipRejectsUnverifiableSecondPair(t *testing.T) {
	number, issuer, date := "42", "Government", "2026-01-01"
	r := HandOverRecord{State: "seed_primary", ProbeState: "complete", Number: number, Issuer: issuer, Date: date, PrimaryID: "1", Comparisons: []HandOverComparison{{ID: "1", State: "match", Number: &number, Issuer: &issuer, Date: &date}, {ID: "2", State: "different_date"}}}
	if gap := precedenceGap("vn:chinhphu:5", []handOverCandidate{{record: r}}); gap != nil {
		t.Fatalf("skipped with unverifiable second pair: %+v", gap)
	}
}
