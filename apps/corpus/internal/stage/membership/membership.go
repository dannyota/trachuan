// Package membership stores and freezes source scope membership.
package membership

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
)

// EvidenceRef identifies the response supporting a membership rule.
type EvidenceRef struct {
	QueryIdentity  *string `json:"query_identity"`
	Page           *string `json:"page"`
	ResponseSHA256 string  `json:"response_sha256"`
	ArtifactHash   string  `json:"artifact_hash"`
	Path           string  `json:"path"`
}

// SupportTuple identifies an independent reason for membership.
type SupportTuple struct {
	RuleKind  string      `json:"rule_kind"`
	RuleValue string      `json:"rule_value"`
	Evidence  EvidenceRef `json:"evidence"`
}

// Entry describes one run membership record.
type Entry struct {
	DocumentKey      string         `json:"document_key"`
	Source           string         `json:"source"`
	SourceID         string         `json:"source_id"`
	Class            string         `json:"class"`
	Disposition      string         `json:"disposition"`
	RawNumber        *string        `json:"raw_number"`
	NormalizedNumber *string        `json:"normalized_number"`
	ReplacementKey   *string        `json:"replacement_key"`
	Supports         []SupportTuple `json:"supports"`
}

type snapshot struct {
	Version int     `json:"version"`
	Entries []Entry `json:"entries"`
}

// CanonicalSnapshot returns sorted compact JSON and its SHA-256 hex digest.
func CanonicalSnapshot(entries []Entry) ([]byte, string, error) {
	sorted := append([]Entry{}, entries...)
	for i := range sorted {
		sorted[i].Supports = MergeSupports(nil, sorted[i].Supports)
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].DocumentKey < sorted[j].DocumentKey })
	data, err := json.Marshal(snapshot{Version: 1, Entries: sorted})
	if err != nil {
		return nil, "", fmt.Errorf("encode snapshot: %w", err)
	}
	sum := sha256.Sum256(data)
	return data, hex.EncodeToString(sum[:]), nil
}

// MergeSupports combines independent support tuples in canonical order.
func MergeSupports(a, b []SupportTuple) []SupportTuple {
	all := append(append([]SupportTuple{}, a...), b...)
	sort.Slice(all, func(i, j int) bool {
		x, _ := json.Marshal(all[i])
		y, _ := json.Marshal(all[j])
		return bytes.Compare(x, y) < 0
	})
	out := make([]SupportTuple, 0, len(all))
	var last []byte
	for _, s := range all {
		data, _ := json.Marshal(s)
		if last == nil || !bytes.Equal(last, data) {
			out = append(out, s)
			last = data
		}
	}
	return out
}

// MergeClass applies precedence within the primary and relation classes.
func MergeClass(a, b string) (string, error) {
	valid := func(v string) bool { return v == "primary" || v == "relation_context" || v == "secondary" }
	if !valid(a) || !valid(b) || ((a == "secondary") != (b == "secondary")) {
		return "", fmt.Errorf("incompatible membership classes %q and %q", a, b)
	}
	if a == "primary" || b == "primary" {
		return "primary", nil
	}
	return a, nil
}

// LeavesScope reports whether neither current nor retained support remains.
func LeavesScope(current, retained int) bool { return current == 0 && retained == 0 }
