// Package lookup defines evidence-bearing source lookup results.
package lookup

import (
	"encoding/json"
	"fmt"
)

// Completeness describes whether evidence covers the full query.
type Completeness string

// Completeness values.
const (
	Complete   Completeness = "complete"
	Incomplete Completeness = "incomplete"
)

// Item holds source metadata and a reference to its page.
type Item struct {
	Source    string   `json:"source"`
	SourceID  string   `json:"source_id"`
	RawNumber *string  `json:"raw_number"`
	Title     *string  `json:"title"`
	Issuer    *string  `json:"issuer"`
	IssueDate *string  `json:"issue_date"`
	PageRef   string   `json:"page_ref"`
	AgencyIDs []string `json:"agency_ids"`
}

// Page records the evidence for one response.
type Page struct {
	Ordinal   *int     `json:"ordinal"`
	Cursor    *string  `json:"cursor"`
	Total     *int     `json:"total"`
	ResultIDs []string `json:"result_ids"`
	SHA256    string   `json:"sha256"`
	Path      string   `json:"path"`
}

// Evidence records the query and the responses supporting a result.
type Evidence struct {
	QueryIdentity string          `json:"query_identity"`
	Query         json.RawMessage `json:"query"`
	PagesFetched  []Page          `json:"pages_fetched"`
	TotalReported *int            `json:"total_reported"`
	Completeness  Completeness    `json:"completeness"`
}

// Result is the versioned lookup envelope.
type Result struct {
	Version  int      `json:"version"`
	Items    []Item   `json:"items"`
	Evidence Evidence `json:"evidence"`
}

// UnmarshalJSON rejects unsupported envelope versions and completeness values.
func (r *Result) UnmarshalJSON(data []byte) error {
	type plain Result
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return fmt.Errorf("decode lookup result: %w", err)
	}
	if decoded.Version != 1 {
		return fmt.Errorf("unsupported lookup result version %d", decoded.Version)
	}
	if decoded.Evidence.Completeness != Complete && decoded.Evidence.Completeness != Incomplete {
		return fmt.Errorf("invalid completeness %q", decoded.Evidence.Completeness)
	}
	*r = Result(decoded)
	return nil
}

// ProvesAbsence reports whether recorded complete evidence proves zero results.
func (r Result) ProvesAbsence() bool {
	return r.Version == 1 && len(r.Evidence.PagesFetched) > 0 && r.Evidence.PagesFetched[0].SHA256 != "" && r.Evidence.PagesFetched[0].Path != "" && r.Evidence.Completeness == Complete && r.Evidence.TotalReported != nil && *r.Evidence.TotalReported == 0 && len(r.Items) == 0
}

// Builder collects query results and pagination outcomes.
type Builder struct {
	QueryIdentity string
	Query         json.RawMessage
	Items         []Item
	Pages         []Page
	TotalReported *int
	EndedNormally bool
	Failed        bool
	Uncertain     bool
}

// Result constructs an envelope, deriving completeness from the evidence.
func (b Builder) Result() Result {
	completeness := Incomplete
	if b.complete() {
		completeness = Complete
	}
	return Result{Version: 1, Items: b.Items, Evidence: Evidence{QueryIdentity: b.QueryIdentity, Query: b.Query, PagesFetched: b.Pages, TotalReported: b.TotalReported, Completeness: completeness}}
}

func (b Builder) complete() bool {
	if b.Failed || b.Uncertain || !b.EndedNormally || b.TotalReported == nil || *b.TotalReported < 0 || len(b.Pages) == 0 {
		return false
	}
	seen := make(map[string]bool)
	for _, page := range b.Pages {
		if page.Total != nil && *page.Total != *b.TotalReported {
			return false
		}
		added := 0
		for _, id := range page.ResultIDs {
			if id == "" {
				return false
			}
			if !seen[id] {
				seen[id] = true
				added++
			}
		}
		if len(page.ResultIDs) > 0 && added == 0 {
			return false
		}
	}
	return len(seen) == *b.TotalReported
}
