package evaluation

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// Evidence records the source used to adjudicate an expectation.
type Evidence struct {
	SourceURL   string `json:"source_url"`
	FetchSHA256 string `json:"fetch_sha256"`
	NodeID      string `json:"node_id"`
	QuotedSpan  string `json:"quoted_span"`
}

// Alternative is an adjudicated identity or relation.
type Alternative struct {
	Country        string  `json:"country"`
	DocumentKey    string  `json:"document_key"`
	Anchor         *string `json:"anchor"`
	Kind           string  `json:"kind"`
	SourceKey      string  `json:"source_key,omitempty"`
	TargetKey      string  `json:"target_key,omitempty"`
	RelationKind   string  `json:"relation_kind,omitempty"`
	EvidenceSHA256 string  `json:"evidence_sha256,omitempty"`
}

// MapEntry adjudicates one original citation.
type MapEntry struct {
	CaseID         string        `json:"case_id"`
	CitationIndex  int           `json:"citation_index"`
	State          string        `json:"state"`
	ExpectedNumber string        `json:"expected_number"`
	Reason         string        `json:"reason"`
	ReviewedAt     string        `json:"reviewed_at"`
	Evidence       []Evidence    `json:"evidence"`
	Alternatives   []Alternative `json:"alternatives"`
}

// IdentityMap binds adjudications to the fixture and inventory.
type IdentityMap struct {
	Version               int        `json:"version"`
	LegacySHA256          string     `json:"legacy_sha256"`
	IdentityInventoryHash string     `json:"identity_inventory_hash"`
	Entries               []MapEntry `json:"entries"`
}

// LoadIdentityMap decodes an identity map.
func LoadIdentityMap(r io.Reader) (IdentityMap, error) {
	var m IdentityMap
	d := json.NewDecoder(r)
	if err := d.Decode(&m); err != nil {
		return m, fmt.Errorf("decode identity map: %w", err)
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		if err == nil {
			return m, fmt.Errorf("decode identity map: trailing JSON")
		}
		return m, fmt.Errorf("decode identity map: %w", err)
	}
	return m, nil
}

// Validate checks that every expectation has one complete adjudication.
func (m IdentityMap) Validate(cases []Case, legacySHA256, inventoryHash string) error {
	fail := func(s string) error { return fmt.Errorf("mapping_incomplete: %s", s) }
	if m.Version != 1 || legacySHA256 == "" || inventoryHash == "" || m.LegacySHA256 != legacySHA256 || m.IdentityInventoryHash != inventoryHash {
		return fail("version or input hash mismatch")
	}
	expected := make(map[string][]Expectation)
	for _, c := range cases {
		if _, ok := expected[c.ID]; ok {
			return fail("duplicate case")
		}
		expected[c.ID] = c.ExpectedCitations
	}
	seen := make(map[string]bool)
	for _, e := range m.Entries {
		list, ok := expected[e.CaseID]
		if !ok || e.CitationIndex < 0 || e.CitationIndex >= len(list) {
			return fail("unknown citation")
		}
		key := fmt.Sprintf("%s:%d", e.CaseID, e.CitationIndex)
		if seen[key] {
			return fail("duplicate entry")
		}
		seen[key] = true
		if e.ExpectedNumber != list[e.CitationIndex].DocNumber || strings.TrimSpace(e.Reason) == "" || strings.TrimSpace(e.ReviewedAt) == "" {
			return fail("missing review or changed expectation")
		}
		if e.State == "absent" {
			numbers := append([]string{list[e.CitationIndex].DocNumber}, list[e.CitationIndex].AltDocNumbers...)
			for _, number := range numbers {
				if strings.TrimSpace(number) == "" || !strings.Contains(e.Reason, number) {
					return fail("absence does not name every searched number")
				}
			}
			if len(e.Alternatives) != 0 {
				return fail("absent entry has alternatives")
			}
			continue
		}
		if e.State != "resolved" || len(e.Alternatives) == 0 || len(e.Evidence) == 0 {
			return fail("unresolved entry")
		}
		// No verified source URL is stored, so source_url may be empty; a node
		// id is required whenever the entry credits a provision.
		needsNode := false
		for _, a := range e.Alternatives {
			needsNode = needsNode || a.Kind == "provision"
		}
		for _, v := range e.Evidence {
			if v.FetchSHA256 == "" || v.QuotedSpan == "" || (needsNode && v.NodeID == "") {
				return fail("incomplete evidence")
			}
		}
		alternatives := make(map[Alternative]bool)
		for _, a := range e.Alternatives {
			if a.Country == "" || a.DocumentKey == "" {
				return fail("incomplete identity")
			}
			switch a.Kind {
			case "document":
				if a.Anchor != nil {
					return fail("document anchor")
				}
			case "provision":
				if a.Anchor == nil || *a.Anchor == "" {
					return fail("missing anchor")
				}
			case "relation":
				if (a.DocumentKey != a.SourceKey && a.DocumentKey != a.TargetKey) || a.SourceKey == "" || a.TargetKey == "" || a.RelationKind == "" || a.EvidenceSHA256 == "" || (a.Anchor != nil && *a.Anchor == "") {
					return fail("incomplete relation")
				}
			default:
				return fail("unknown alternative")
			}
			for old := range alternatives {
				if sameAlternative(old, a) {
					return fail("duplicate alternative")
				}
			}
			alternatives[a] = true
		}
	}
	for id, list := range expected {
		for i := range list {
			if !seen[fmt.Sprintf("%s:%d", id, i)] {
				return fail("missing entry")
			}
		}
	}
	return nil
}
func sameAlternative(a, b Alternative) bool {
	aa, bb := a.Anchor, b.Anchor
	a.Anchor = nil
	b.Anchor = nil
	if a != b {
		return false
	}
	return (aa == nil && bb == nil) || (aa != nil && bb != nil && *aa == *bb)
}
