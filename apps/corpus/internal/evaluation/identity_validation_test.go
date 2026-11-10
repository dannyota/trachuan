package evaluation

import (
	"strings"
	"testing"
)

func TestValidateInventory_PerCaseAndDocumentScoped(t *testing.T) {
	anchor := "dieu-1"
	cases := []Case{{ID: "one", ExpectedCitations: []Expectation{{DocNumber: "1/2024/TT"}}}, {ID: "two", ExpectedCitations: []Expectation{{DocNumber: "2/2024/TT"}}}}
	m := IdentityMap{Version: 2, LegacySHA256: "legacy", Entries: []MapEntry{
		{CaseID: "one", ExpectedNumber: "1/2024/TT", State: "resolved", Reason: "review", ReviewedAt: "2026-11-01", Evidence: []Evidence{{DocumentKey: "vn:1", DocumentNumber: "1/2024/TT", NodeID: "same", FetchSHA256: "hash", QuotedSpan: "text"}}, Alternatives: []Alternative{{Country: "vn", DocumentKey: "vn:1", DocumentNumber: "1/2024/TT", Kind: "provision", Anchor: &anchor}}},
		{CaseID: "two", ExpectedNumber: "2/2024/TT", State: "resolved", Reason: "review", ReviewedAt: "2026-11-01", Evidence: []Evidence{{DocumentKey: "vn:2", DocumentNumber: "2/2024/TT", NodeID: "same", FetchSHA256: "hash", QuotedSpan: "text"}}, Alternatives: []Alternative{{Country: "vn", DocumentKey: "vn:2", DocumentNumber: "2/2024/TT", Kind: "document"}}},
	}}
	inventory := IdentityInventory{Documents: map[string]IdentityDocument{"vn:1": {Number: "1/2024/TT", Anchors: map[string]bool{anchor: true}, Nodes: map[string]bool{"same": true}}, "vn:2": {Number: "2/2024/TT"}}}
	report, err := m.Validate(cases, "legacy", inventory)
	if err != nil {
		t.Fatal(err)
	}
	if len(report["one"]) != 0 || len(report["two"]) == 0 || report["two"][0].Field != "evidence[0].node_id" {
		t.Fatalf("report = %+v", report)
	}
}

func TestIdentity_ReplacedSecondaryNeedsReadjudication(t *testing.T) {
	cases := []Case{{ID: "one", ExpectedCitations: []Expectation{{DocNumber: "1/2024/TT"}}}}
	m := IdentityMap{Version: 2, LegacySHA256: "legacy", Entries: []MapEntry{{CaseID: "one", ExpectedNumber: "1/2024/TT", State: "resolved", Reason: "review", ReviewedAt: "2026-11-01", Alternatives: []Alternative{{Country: "vn", DocumentKey: "vn:chinhphu:1", DocumentNumber: "1/2024/TT", Kind: "document"}}}}}
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
	m := IdentityMap{Version: 2, LegacySHA256: "legacy", Entries: []MapEntry{{CaseID: "one", ExpectedNumber: "1/2024/TT", State: "resolved", Reason: "review", ReviewedAt: "2026-11-01", Alternatives: []Alternative{{Country: "vn", DocumentKey: "vn:vbpl:1", DocumentNumber: "1/2024/TT", Kind: "document"}}}}}
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
