package evaluation

import (
	"fmt"
	"strings"

	"danny.vn/trachuan/internal/retrieval"
)

// IdentityDocument records identities verified from one document's artifacts.
type IdentityDocument struct {
	Number         string
	Anchors, Nodes map[string]bool
}

// IdentityInventory holds source document identities for one run.
type IdentityInventory struct{ Documents map[string]IdentityDocument }

// MappingIssue identifies a defect in one citation's mapping.
type MappingIssue struct {
	CitationIndex int    `json:"citation_index"`
	Kind          string `json:"kind"`
	Field         string `json:"field"`
	Detail        string `json:"detail"`
}

// ValidationReport groups mapping defects by case.
type ValidationReport map[string][]MappingIssue

// Validate checks mappings against the document-qualified inventory.
func (m IdentityMap) Validate(cases []Case, legacy string, inventory IdentityInventory) (ValidationReport, error) {
	if (m.Version != 1 && m.Version != 2) || legacy == "" || m.LegacySHA256 != legacy {
		return nil, fmt.Errorf("invalid mapping version or legacy hash")
	}
	report := ValidationReport{}
	entries := map[string]map[int][]MapEntry{}
	expectedCases := map[string]int{}
	for _, c := range cases {
		if _, exists := expectedCases[c.ID]; exists {
			return nil, fmt.Errorf("duplicate case ID")
		}
		expectedCases[c.ID] = len(c.ExpectedCitations)
	}
	for _, entry := range m.Entries {
		count, exists := expectedCases[entry.CaseID]
		if !exists || entry.CitationIndex < 0 || entry.CitationIndex >= count {
			return nil, fmt.Errorf("map contains unknown citation")
		}
		if entries[entry.CaseID] == nil {
			entries[entry.CaseID] = map[int][]MapEntry{}
		}
		entries[entry.CaseID][entry.CitationIndex] = append(entries[entry.CaseID][entry.CitationIndex], entry)
	}
	for _, c := range cases {
		for i, expected := range c.ExpectedCitations {
			issue := func(field, detail string) {
				report[c.ID] = append(report[c.ID], MappingIssue{i, "mapping_incomplete", field, field + ": " + detail})
			}
			found := entries[c.ID][i]
			if len(found) != 1 {
				issue("entry", "missing or duplicate entry")
				continue
			}
			e := found[0]
			if e.ExpectedNumber != expected.DocNumber {
				issue("expected_number", "changed expectation")
			}
			if strings.TrimSpace(e.Reason) == "" {
				issue("reason", "missing review reason")
			}
			if strings.TrimSpace(e.ReviewedAt) == "" {
				issue("reviewed_at", "missing review date")
			}
			if e.State == "absent" {
				if len(e.Alternatives) != 0 {
					issue("alternatives", "absent entry has alternatives")
				}
				for _, number := range append([]string{expected.DocNumber}, expected.AltDocNumbers...) {
					if !strings.Contains(e.Reason, number) {
						issue("reason", "searched number missing")
					}
					for _, doc := range inventory.Documents {
						if retrieval.NormalizeNumber(number) == doc.Number && len(doc.Anchors) > 0 {
							issue("state", "absent number now present")
							break
						}
					}
				}
				continue
			}
			if e.State != "resolved" {
				issue("state", "invalid mapping state")
				continue
			}
			if len(e.Alternatives) == 0 {
				issue("alternatives", "missing alternatives")
			}
			if len(e.Evidence) == 0 {
				issue("evidence", "missing evidence")
			}
			checkDoc := func(prefix, key, number string) IdentityDocument {
				doc, ok := inventory.Documents[key]
				if key == "" || !ok {
					issue(prefix+"_key", "missing document identity")
				}
				if number == "" || !ok || number != doc.Number || retrieval.NormalizeNumber(number) != number {
					issue(prefix+"_number", "missing or changed document number")
				}
				return doc
			}
			needsNode := false
			for _, alt := range e.Alternatives {
				if alt.Kind == "provision" {
					needsNode = true
				}
			}
			for n, evidence := range e.Evidence {
				p := fmt.Sprintf("evidence[%d].document", n)
				doc := checkDoc(p, evidence.DocumentKey, evidence.DocumentNumber)
				if (needsNode && evidence.NodeID == "") || (evidence.NodeID != "" && !doc.Nodes[evidence.NodeID]) {
					issue(fmt.Sprintf("evidence[%d].node_id", n), "missing document-qualified node")
				}
				if evidence.FetchSHA256 == "" {
					issue(fmt.Sprintf("evidence[%d].fetch_sha256", n), "missing review evidence")
				}
				if evidence.QuotedSpan == "" {
					issue(fmt.Sprintf("evidence[%d].quoted_span", n), "missing review evidence")
				}
			}
			for n, alt := range e.Alternatives {
				p := fmt.Sprintf("alternatives[%d]", n)
				doc := checkDoc(p+".document", alt.DocumentKey, alt.DocumentNumber)
				if alt.Country != "vn" {
					issue(p+".country", "missing country")
				}
				switch alt.Kind {
				case "document":
					if alt.Anchor != nil {
						issue(p+".anchor", "document anchor must be null")
					}
				case "provision":
					if alt.Anchor == nil || !doc.Anchors[*alt.Anchor] {
						issue(p+".anchor", "missing provision anchor")
					}
				case "relation":
					checkDoc(p+".source", alt.SourceKey, alt.SourceNumber)
					checkDoc(p+".target", alt.TargetKey, alt.TargetNumber)
					if alt.Anchor != nil && !doc.Anchors[*alt.Anchor] {
						issue(p+".anchor", "missing relation provision anchor")
					}
					if alt.RelationKind == "" {
						issue(p+".relation_kind", "missing relation kind")
					}
					if alt.EvidenceSHA256 == "" {
						issue(p+".evidence_sha256", "missing relation evidence hash")
					}
					if alt.DocumentKey != alt.SourceKey && alt.DocumentKey != alt.TargetKey {
						issue(p+".document_key", "not a relation endpoint")
					}
				default:
					issue(p+".kind", "invalid kind")
				}
				for j := 0; j < n; j++ {
					if sameAlternative(e.Alternatives[j], alt) {
						issue(p, "duplicate alternative")
					}
				}
			}
		}
	}
	return report, nil
}
