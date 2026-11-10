package evaluation

import (
	"fmt"
	"strings"
	"time"

	"danny.vn/trachuan/internal/retrieval"
)

// ValidityCase records an adjudicated document and its supporting text.
type ValidityCase struct {
	Number      string `json:"number"`
	IssueDate   string `json:"issue_date"`
	DocumentKey string `json:"-"`
	Resolution  string `json:"-"`
	Current     bool   `json:"current"`
	Anchor      string `json:"anchor"`
	Quote       string `json:"quote"`
}

// ValidityDisagreement identifies a returned stamp that differs from adjudication.
type ValidityDisagreement struct {
	CaseID   string `json:"case_id"`
	Number   string `json:"document_number"`
	Expected bool   `json:"expected_current"`
	Actual   bool   `json:"actual_current"`
}

// AdjudicatedValidity records agreement and coverage across primary hits.
type AdjudicatedValidity struct {
	Hits          int                    `json:"hits"`
	Agree         int                    `json:"agree"`
	Disagree      int                    `json:"disagree"`
	Coverage      *float64               `json:"coverage"`
	Disagreements []ValidityDisagreement `json:"disagreements"`
	NotEvaluated  []string               `json:"not_evaluated"`
	total         int
}

func validateValidityCases(cases []ValidityCase) error {
	if len(cases) == 0 {
		return fmt.Errorf("empty validity cases")
	}
	seen := map[string]bool{}
	for _, c := range cases {
		n := retrieval.NormalizeNumber(c.Number)
		_, dateErr := time.Parse("2006-01-02", c.IssueDate)
		identity := n + "\x00" + c.IssueDate
		if n == "" || dateErr != nil || seen[identity] || strings.TrimSpace(c.Anchor) == "" || strings.TrimSpace(c.Quote) == "" {
			return fmt.Errorf("invalid validity case %q", c.Number)
		}
		seen[identity] = true
	}
	return nil
}

func (r *Report) addHits(caseID string, hits []retrieval.Hit, cases []ValidityCase) {
	if r.HitsByClass == nil {
		r.HitsByClass = map[string]int{}
		r.HitsByStatusBasis = map[string]int{}
		r.HitsByConflictKind = map[string]int{}
	}
	for _, h := range hits {
		r.HitsByClass[h.Class]++
		r.HitsByStatusBasis[h.Stamp.StatusBasis]++
		if h.Stamp.Unresolved {
			r.HitsByConflictKind["unresolved"]++
		} else {
			r.HitsByConflictKind["none"]++
		}
		r.ValidityAdjudicated.total++
		for _, c := range cases {
			if c.DocumentKey == "" || c.DocumentKey != h.Identity.DocumentKey {
				continue
			}
			r.ValidityAdjudicated.Hits++
			if c.Current == h.Stamp.Current {
				r.ValidityAdjudicated.Agree++
			} else {
				r.ValidityAdjudicated.Disagree++
				r.ValidityAdjudicated.Disagreements = append(r.ValidityAdjudicated.Disagreements, ValidityDisagreement{caseID, c.Number, c.Current, h.Stamp.Current})
			}
			break
		}
	}
	r.ValidityAdjudicated.Coverage = ratio(r.ValidityAdjudicated.Hits, r.ValidityAdjudicated.total)
}

// ValidityDocument identifies one document in the evaluated corpus.
type ValidityDocument struct{ Key, Number, IssueDate string }

// ResolveValidityCases binds each case to a unique corpus document.
func ResolveValidityCases(cases []ValidityCase, documents []ValidityDocument) {
	for i := range cases {
		c := &cases[i]
		c.DocumentKey = ""
		c.Resolution = "absent"
		count := 0
		for _, d := range documents {
			if retrieval.NormalizeNumber(c.Number) == retrieval.NormalizeNumber(d.Number) && c.IssueDate == d.IssueDate {
				count++
				c.DocumentKey = d.Key
			}
		}
		if count == 1 {
			c.Resolution = ""
		} else if count > 1 {
			c.DocumentKey = ""
			c.Resolution = "ambiguous"
		}
	}
}
