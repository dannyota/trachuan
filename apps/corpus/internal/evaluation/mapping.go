package evaluation

import (
	"encoding/json"
	"fmt"
	"io"
)

// Evidence records the source used to adjudicate an expectation.
type Evidence struct {
	DocumentKey    string `json:"document_key"`
	DocumentNumber string `json:"document_number"`
	SourceURL      string `json:"source_url"`
	FetchSHA256    string `json:"fetch_sha256"`
	NodeID         string `json:"node_id"`
	QuotedSpan     string `json:"quoted_span"`
}

// Alternative is an adjudicated identity or relation.
type Alternative struct {
	Country        string  `json:"country"`
	DocumentKey    string  `json:"document_key"`
	DocumentNumber string  `json:"document_number"`
	Anchor         *string `json:"anchor"`
	Kind           string  `json:"kind"`
	SourceKey      string  `json:"source_key,omitempty"`
	SourceNumber   string  `json:"source_number,omitempty"`
	TargetKey      string  `json:"target_key,omitempty"`
	TargetNumber   string  `json:"target_number,omitempty"`
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
	IdentityInventoryHash string     `json:"identity_inventory_hash,omitempty"`
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

func sameAlternative(a, b Alternative) bool {
	aa, bb := a.Anchor, b.Anchor
	a.Anchor = nil
	b.Anchor = nil
	if a != b {
		return false
	}
	return (aa == nil && bb == nil) || (aa != nil && bb != nil && *aa == *bb)
}
