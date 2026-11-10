package build

import (
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/source"
)

func TestCorrections_EveryRejection(t *testing.T) {
	from := "2024-01-01"
	to := "2025-01-01"
	status := "in_force"
	valid := source.Correction{Key: "vn:vbpl:1", Number: "1/2024/NĐ-CP", Set: source.CorrectionSet{StatusClass: &status, EffFrom: &from, EffTo: &to}, Reason: "source error", Evidence: source.CorrectionEvidence{Anchor: "Điều 1", Quote: "hiệu lực"}, Reviewed: "2026-11-10"}
	docs := map[string]correctionDocument{"vn:vbpl:1": {number: "1/2024/NĐ-CP", provisions: map[string]string{"Điều 1": "có hiệu lực"}}}
	cases := []struct {
		name   string
		modify func(*source.Correction)
	}{
		{"missing key", func(c *source.Correction) { c.Key = "missing" }},
		{"wrong number", func(c *source.Correction) { c.Number = "2/2024/NĐ-CP" }},
		{"missing anchor", func(c *source.Correction) { c.Evidence.Anchor = "Điều 2" }},
		{"missing quote", func(c *source.Correction) { c.Evidence.Quote = "other" }},
		{"empty correction", func(c *source.Correction) { c.Set = source.CorrectionSet{} }},
		{"invalid class", func(c *source.Correction) { s := "other"; c.Set.StatusClass = &s }},
		{"missing reason", func(c *source.Correction) { c.Reason = "" }},
		{"invalid reviewed", func(c *source.Correction) { c.Reviewed = "bad" }},
	}
	if _, err := validateCorrections([]source.Correction{valid}, docs); err != nil {
		t.Fatalf("valid correction: %v", err)
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := valid
			tc.modify(&c)
			if _, err := validateCorrections([]source.Correction{c}, docs); err == nil {
				t.Fatal("accepted invalid correction")
			}
		})
	}
}

func TestCorrections_OverrideWins(t *testing.T) {
	status := "expired"
	eligible := true
	got, _ := applyCorrection(validityValues{Class: "in_force", Current: true, Basis: "derived_from_text", Conflict: true}, source.Correction{Set: source.CorrectionSet{StatusClass: &status}, Eligible: &eligible}, true)
	if got.Class != "expired" || got.Current || got.Basis != "manual_correction" || got.Conflict {
		t.Fatalf("manual correction = %+v", got)
	}
}

func TestCorrections_RedundantAndConflictReports(t *testing.T) {
	status := "in_force"
	correction := source.Correction{Set: source.CorrectionSet{StatusClass: &status}}
	_, report := applyCorrection(validityValues{Class: "in_force", Current: true, Basis: "source_status"}, correction, false)
	if report != "correction_redundant" {
		t.Errorf("redundant report = %q", report)
	}
	_, report = applyCorrection(validityValues{Class: "expired", Basis: "derived_from_text"}, correction, true)
	if report != "correction_conflict" {
		t.Errorf("conflict report = %q", report)
	}
}

func TestCorrections_IntervalValidatedAfterRules(t *testing.T) {
	end := "2025-01-01"
	c := source.Correction{Key: "a", Number: "1/2020/NĐ-CP", Set: source.CorrectionSet{EffTo: &end}, Reason: "source error", Evidence: source.CorrectionEvidence{Quote: "evidence"}, Reviewed: "2026-11-10"}
	from, _ := time.Parse("2006-01-02", "2027-07-01")
	if _, err := validateCorrections([]source.Correction{c}, map[string]correctionDocument{"a": {number: c.Number, from: &from}}); err != nil {
		t.Fatal(err)
	}
	d := testDoc("a", c.Number, "2020-01-01", "2027-07-01")
	d.sourceClass = "not_yet"
	d.statusCode = "CCHL"
	d.article = testArticle("1/7/2024", "")
	if _, err := runValidity([]validityDocument{d}, nil, map[string]source.Correction{"a": c}, "2026-11-10"); err != nil {
		t.Fatal(err)
	}
	bad := "2023-01-01"
	c.Set.EffTo = &bad
	if _, err := runValidity([]validityDocument{d}, nil, map[string]source.Correction{"a": c}, "2026-11-10"); err == nil || !strings.Contains(err.Error(), "a") {
		t.Fatalf("expected named invalid interval: %v", err)
	}
}
