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
	c := source.Correction{Key: "a", Number: "1/2020/NĐ-CP", Set: source.CorrectionSet{EffTo: &end}, Reason: "source error", Evidence: source.CorrectionEvidence{Anchor: "dieu-1", Quote: "evidence"}, Reviewed: "2026-11-10"}
	from, _ := time.Parse("2006-01-02", "2027-07-01")
	if _, err := validateCorrections([]source.Correction{c}, map[string]correctionDocument{"a": {number: c.Number, from: &from, provisions: map[string]string{"dieu-1": "evidence"}}}); err != nil {
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

func TestCorrections_EvidenceInOtherDocument(t *testing.T) {
	data := []byte("- {key: a, number: A, hold: true, reason: why, evidence: {key: b, anchor: dieu-37/khoan-1, quote: 'verbatim quote'}, reviewed: '2026-11-10'}\n")
	entries, err := source.ParseCorrections(data)
	if err != nil {
		t.Fatal(err)
	}
	docs := map[string]correctionDocument{
		"a": {number: "A", provisions: map[string]string{"dieu-37/khoan-1": "different text"}},
		"b": {number: "B", provisions: map[string]string{"dieu-37/khoan-1": "a verbatim quote here"}},
	}
	if _, err = validateCorrections(entries, docs); err != nil {
		t.Fatal(err)
	}
	docs["b"] = correctionDocument{number: "B", provisions: map[string]string{"dieu-37/khoan-1": "different text"}}
	if _, err = validateCorrections(entries, docs); err == nil {
		t.Fatal("accepted quote absent from evidence document")
	}
}

func TestCorrections_EvidenceKeyAbsentDocumentRejected(t *testing.T) {
	data := []byte("- {key: a, number: A, hold: true, reason: why, evidence: {key: missing, anchor: dieu-1, quote: text}, reviewed: '2026-11-10'}\n")
	entries, err := source.ParseCorrections(data)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = validateCorrections(entries, map[string]correctionDocument{"a": {number: "A"}}); err == nil {
		t.Fatal("accepted absent evidence document")
	}
}

func TestCorrections_QuoteWithoutAnchorOrURLRejected(t *testing.T) {
	c := source.Correction{Key: "a", Number: "A", Hold: true, Reason: "why", Evidence: source.CorrectionEvidence{Quote: "text"}, Reviewed: "2026-11-10"}
	if _, err := validateCorrections([]source.Correction{c}, map[string]correctionDocument{"a": {number: "A"}}); err == nil {
		t.Fatal("accepted quote without location")
	}
}

func TestCorrections_KeyWithoutAnchorRejected(t *testing.T) {
	for _, evidence := range []string{"{key: b, quote: text}", "{key: b, url: 'https://example.org/law', quote: text}"} {
		data := []byte("- {key: a, number: A, hold: true, reason: why, evidence: " + evidence + ", reviewed: '2026-11-10'}\n")
		entries, err := source.ParseCorrections(data)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := validateCorrections(entries, map[string]correctionDocument{"a": {number: "A"}, "b": {number: "B"}}); err == nil {
			t.Fatal("accepted key without anchor")
		}
	}
}

func TestCorrections_AnchorAndURLRejected(t *testing.T) {
	data := []byte("- {key: a, number: A, hold: true, reason: why, evidence: {anchor: dieu-1, url: 'https://example.org/law', quote: text}, reviewed: '2026-11-10'}\n")
	entries, err := source.ParseCorrections(data)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := validateCorrections(entries, map[string]correctionDocument{"a": {number: "A", provisions: map[string]string{"dieu-1": "text"}}}); err == nil {
		t.Fatal("accepted anchor and URL")
	}
}

func TestCorrections_ExternalEvidenceAcceptedAndListed(t *testing.T) {
	data := []byte("- {key: a, number: 1/2020/NĐ-CP, eligible: false, reason: why, evidence: {url: 'https://example.org/law', quote: 'external text'}, reviewed: '2026-11-10'}\n")
	entries, err := source.ParseCorrections(data)
	if err != nil {
		t.Fatal(err)
	}
	corrections, err := validateCorrections(entries, map[string]correctionDocument{"a": {number: "1/2020/NĐ-CP"}})
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := canonicalCorrections(entries)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(canonical), `"url":"https://example.org/law"`) {
		t.Fatalf("canonical = %s", canonical)
	}
	d := testDoc("a", "1/2020/NĐ-CP", "2020-01-01", "2020-01-01")
	d.extractHash = "target-hash"
	v := runTest(t, []validityDocument{d}, nil, corrections)
	h := v.documents["a"].history
	if len(h) != 2 || h[1].anchor != "external:https://example.org/law" || h[1].artifact != "" || h[1].before.Conflict || !h[1].after.Conflict {
		t.Fatalf("history=%+v", h)
	}
	data, err = encodeValidityReview(v.reviews)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"kind":"correction_external"`) || !strings.Contains(string(data), `https://example.org/law`) || !strings.Contains(string(data), `external text`) {
		t.Fatalf("reviews=%s", data)
	}
}

func TestCheckCorrections_ReportsEveryInvalidEntry(t *testing.T) {
	valid := source.Correction{Key: "a", Number: "A", Hold: true, Reason: "reviewed", Reviewed: "2026-11-15", Evidence: source.CorrectionEvidence{Anchor: "dieu-1", Quote: "exact text"}}
	badQuote := valid
	badQuote.Evidence.Quote = "exact  text"
	missing := valid
	missing.Key = "missing"
	docs := map[string]correctionDocument{"a": {number: "A", provisions: map[string]string{"dieu-1": "exact text"}}}
	failures := checkCorrections([]source.Correction{badQuote, missing, valid}, docs)
	if len(failures) != 3 || !strings.Contains(failures[0].Error(), "evidence not found") || !strings.Contains(failures[1].Error(), "document absent") || !strings.Contains(failures[2].Error(), "duplicate") {
		t.Fatalf("failures: %v", failures)
	}
}

func TestCheckCorrections_RejectsInvalidInterval(t *testing.T) {
	from, to := "2025-01-01", "2024-01-01"
	c := source.Correction{Key: "a", Number: "A", Set: source.CorrectionSet{EffFrom: &from, EffTo: &to}, Reason: "reviewed", Reviewed: "2026-11-15", Evidence: source.CorrectionEvidence{URL: "https://example.org", Quote: "text"}}
	failures := checkCorrections([]source.Correction{c}, map[string]correctionDocument{"a": {number: "A"}})
	if len(failures) != 1 || !strings.Contains(failures[0].Error(), "invalid interval") {
		t.Fatalf("failures: %v", failures)
	}
}

func TestCorrectionEvidence_ParentLabelsAndExactQuotes(t *testing.T) {
	docs := map[string]correctionDocument{"a": {number: "A", provisions: map[string]string{}}}
	labels := map[int64]string{}
	addCorrectionEvidence(docs, labels, "a", 1, nil, "Điều 1", "dieu-1", "article")
	parent := int64(1)
	addCorrectionEvidence(docs, labels, "a", 2, &parent, "Khoản 1", "dieu-1/khoan-1", "exact text")
	c := source.Correction{Key: "a", Number: "A", Hold: true, Reason: "reviewed", Reviewed: "2026-11-15", Evidence: source.CorrectionEvidence{Anchor: "Điều 1 khoản 1", Quote: "exact text"}}
	if _, err := validateCorrections([]source.Correction{c}, docs); err != nil {
		t.Fatal(err)
	}
	c.Evidence.Quote = "exact  text"
	if _, err := validateCorrections([]source.Correction{c}, docs); err == nil {
		t.Fatal("accepted nonmatching quote")
	}
}

func TestCheckCorrections_UsesUnchangedIntervalBound(t *testing.T) {
	from, err := time.Parse("2006-01-02", "2025-01-01")
	if err != nil {
		t.Fatal(err)
	}
	to := "2024-01-01"
	c := source.Correction{Key: "a", Number: "A", Set: source.CorrectionSet{EffTo: &to}, Reason: "reviewed", Reviewed: "2026-11-15", Evidence: source.CorrectionEvidence{URL: "https://example.org", Quote: "text"}}
	failures := checkCorrections([]source.Correction{c}, map[string]correctionDocument{"a": {number: "A", from: &from}})
	if len(failures) != 1 || !strings.Contains(failures[0].Error(), "invalid interval") {
		t.Fatalf("failures: %v", failures)
	}
}
