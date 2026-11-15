package evaluation

import (
	"strings"
	"testing"
)

func TestValidateInventory_PerCaseAndDocumentScoped(t *testing.T) {
	anchor := "dieu-1"
	cases := []Case{{ID: "one", ExpectedCitations: []Expectation{{DocNumber: "1/2024/TT"}}}, {ID: "two", ExpectedCitations: []Expectation{{DocNumber: "2/2024/TT"}}}}
	m := IdentityMap{Version: 3, LegacySHA256: "legacy", Entries: []MapEntry{
		{CaseID: "one", ExpectedNumber: "1/2024/TT", State: "resolved", Reason: "review", ReviewedAt: "2026-11-01", Evidence: []Evidence{{DocumentKey: "vn:1", DocumentNumber: "1/2024/TT", Anchor: &anchor, FetchSHA256: "hash", QuotedSpan: "text"}}, Alternatives: []Alternative{{Country: "vn", DocumentKey: "vn:1", DocumentNumber: "1/2024/TT", Kind: "provision", Anchor: &anchor}}},
		{CaseID: "two", ExpectedNumber: "2/2024/TT", State: "resolved", Reason: "review", ReviewedAt: "2026-11-01", Evidence: []Evidence{{DocumentKey: "vn:2", DocumentNumber: "2/2024/TT", Anchor: &anchor, FetchSHA256: "hash", QuotedSpan: "text"}}, Alternatives: []Alternative{{Country: "vn", DocumentKey: "vn:2", DocumentNumber: "2/2024/TT", Kind: "document"}}},
	}}
	inventory := IdentityInventory{Documents: map[string]IdentityDocument{"vn:1": {Number: "1/2024/TT", Anchors: map[string]bool{anchor: true}, Texts: map[string]string{anchor: "text"}}, "vn:2": {Number: "2/2024/TT"}}}
	report, err := m.Validate(cases, "legacy", inventory)
	if err != nil {
		t.Fatal(err)
	}
	if len(report["one"]) != 0 || len(report["two"]) == 0 || report["two"][0].Field != "evidence[0].anchor" {
		t.Fatalf("report = %+v", report)
	}
}

func TestIdentity_ReplacedSecondaryNeedsReadjudication(t *testing.T) {
	cases := []Case{{ID: "one", ExpectedCitations: []Expectation{{DocNumber: "1/2024/TT"}}}}
	m := IdentityMap{Version: 3, LegacySHA256: "legacy", Entries: []MapEntry{{CaseID: "one", ExpectedNumber: "1/2024/TT", State: "resolved", Reason: "review", ReviewedAt: "2026-11-01", Alternatives: []Alternative{{Country: "vn", DocumentKey: "vn:chinhphu:1", DocumentNumber: "1/2024/TT", Kind: "document"}}}}}
	inv := IdentityInventory{Documents: map[string]IdentityDocument{}, Replacements: map[string]string{"vn:chinhphu:1": "vn:vbpl:2"}}
	report, err := m.Validate(cases, "legacy", inv)
	if err != nil {
		t.Fatal(err)
	}
	for _, issue := range report["one"] {
		if strings.Contains(issue.Detail, "vn:vbpl:2") {
			return
		}
	}
	t.Fatalf("replacement not named: %+v", report)
}

func TestIdentity_ResolvedNoLongerPresentNeedsReadjudication(t *testing.T) {
	cases := []Case{{ID: "one", ExpectedCitations: []Expectation{{DocNumber: "1/2024/TT"}}}}
	m := IdentityMap{Version: 3, LegacySHA256: "legacy", Entries: []MapEntry{{CaseID: "one", ExpectedNumber: "1/2024/TT", State: "resolved", Reason: "review", ReviewedAt: "2026-11-01", Alternatives: []Alternative{{Country: "vn", DocumentKey: "vn:vbpl:1", DocumentNumber: "1/2024/TT", Kind: "document"}}}}}
	report, err := m.Validate(cases, "legacy", IdentityInventory{Documents: map[string]IdentityDocument{}})
	if err != nil {
		t.Fatal(err)
	}
	for _, issue := range report["one"] {
		if issue.Field == "alternatives[0].document_key" {
			return
		}
	}
	t.Fatalf("missing identity not reported: %+v", report)
}

func TestValidate_VersionThreeEvidence(t *testing.T) {
	cases := []Case{{ID: "one", ExpectedCitations: []Expectation{{DocNumber: "12"}}}}
	data := `{"version":3,"legacy_sha256":"legacy","entries":[{"case_id":"one","expected_number":"12","state":"resolved","reason":"review","reviewed_at":"date","evidence":[{"document_key":"d","document_number":"12","fetch_sha256":"hash","anchor":null,"quoted_span":"title"}],"alternatives":[{"country":"vn","document_key":"d","document_number":"12","kind":"document","anchor":null}]}]}`
	m, err := LoadIdentityMap(strings.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	report, err := m.Validate(cases, "legacy", IdentityInventory{Documents: map[string]IdentityDocument{"d": {Number: "12"}}})
	if err != nil || len(report["one"]) != 0 {
		t.Fatalf("report = %+v, error = %v", report, err)
	}
}

func TestValidate_AnchoredEvidence(t *testing.T) {
	anchor, missing := "dieu-1", "dieu-2"
	cases := []Case{{ID: "one", ExpectedCitations: []Expectation{{DocNumber: "12"}}}}
	entry := MapEntry{CaseID: "one", ExpectedNumber: "12", State: "resolved", Reason: "review", ReviewedAt: "date", Evidence: []Evidence{{DocumentKey: "d", DocumentNumber: "12", FetchSHA256: "hash", Anchor: &anchor, QuotedSpan: "Café\ttext"}}, Alternatives: []Alternative{{Country: "vn", DocumentKey: "d", DocumentNumber: "12", Kind: "provision", Anchor: &anchor}}}
	inv := IdentityInventory{Documents: map[string]IdentityDocument{"d": {Number: "12", Anchors: map[string]bool{anchor: true}, Texts: map[string]string{anchor: "before Cafe\u0301 \n text after"}}}}
	for _, tc := range []struct {
		name   string
		anchor *string
		quote  string
		want   string
	}{
		{"normalized quote found", &anchor, "Café\ttext", ""},
		{"quote missing", &anchor, "other", "evidence[0].quoted_span"},
		{"empty quote", &anchor, " \n", "evidence[0].quoted_span"},
		{"anchor missing", &missing, "text", "evidence[0].anchor"},
		{"provision without anchored evidence", nil, "title", "evidence"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := entry
			e.Evidence = append([]Evidence(nil), entry.Evidence...)
			e.Evidence[0].Anchor, e.Evidence[0].QuotedSpan = tc.anchor, tc.quote
			m := IdentityMap{Version: 3, LegacySHA256: "legacy", Entries: []MapEntry{e}}
			report, err := m.Validate(cases, "legacy", inv)
			if err != nil {
				t.Fatal(err)
			}
			if tc.want == "" && len(report["one"]) == 0 {
				return
			}
			for _, issue := range report["one"] {
				if issue.Field == tc.want && issue.Kind == "mapping_incomplete" {
					return
				}
			}
			t.Fatalf("report = %+v, want %q", report, tc.want)
		})
	}
}

func TestValidate_OldVersionsIncomplete(t *testing.T) {
	cases := []Case{{ID: "one", ExpectedCitations: []Expectation{{DocNumber: "12"}}}}
	for _, version := range []int{1, 2} {
		m := IdentityMap{Version: version, LegacySHA256: "legacy", Entries: []MapEntry{{CaseID: "one", ExpectedNumber: "12", State: "absent", Reason: "searched 12", ReviewedAt: "date"}}}
		report, err := m.Validate(cases, "legacy", IdentityInventory{})
		if err != nil || len(report["one"]) == 0 || report["one"][0].Kind != "mapping_incomplete" {
			t.Fatalf("version %d: %+v, %v", version, report, err)
		}
	}
}

func TestValidate_QuoteOnlyInOtherProvisionOrDocument(t *testing.T) {
	anchor := "dieu-1"
	cases := []Case{{ID: "one", ExpectedCitations: []Expectation{{DocNumber: "12"}}}}
	m := IdentityMap{Version: 3, LegacySHA256: "legacy", Entries: []MapEntry{{CaseID: "one", ExpectedNumber: "12", State: "resolved", Reason: "review", ReviewedAt: "date", Evidence: []Evidence{{DocumentKey: "d", DocumentNumber: "12", FetchSHA256: "hash", Anchor: &anchor, QuotedSpan: "target quote"}}, Alternatives: []Alternative{{Country: "vn", DocumentKey: "d", DocumentNumber: "12", Kind: "provision", Anchor: &anchor}}}}}
	for _, tc := range []struct {
		name, document, otherAnchor string
	}{
		{"child", "d", "dieu-1/khoan-1"},
		{"sibling", "d", "dieu-2"},
		{"other document same anchor", "other", "dieu-1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			inv := IdentityInventory{Documents: map[string]IdentityDocument{
				"d":     {Number: "12", Anchors: map[string]bool{anchor: true}, Texts: map[string]string{anchor: "own text"}},
				"other": {Number: "13", Anchors: map[string]bool{}, Texts: map[string]string{}},
			}}
			doc := inv.Documents[tc.document]
			doc.Anchors[tc.otherAnchor] = true
			doc.Texts[tc.otherAnchor] = "target quote"
			report, err := m.Validate(cases, "legacy", inv)
			if err != nil {
				t.Fatal(err)
			}
			if len(report["one"]) != 1 || report["one"][0].Field != "evidence[0].quoted_span" || !strings.Contains(report["one"][0].Detail, "quote not found") {
				t.Fatalf("report = %+v", report)
			}
		})
	}
}
