package evaluation

import (
	"fmt"
	"strings"

	"danny.vn/trachuan/internal/retrieval"
)

// ValidityCase records an adjudicated document and its supporting text.
type ValidityCase struct {
	Number  string `json:"number"`
	Current bool   `json:"current"`
	Anchor  string `json:"anchor"`
	Quote   string `json:"quote"`
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
		if n == "" || seen[n] || strings.TrimSpace(c.Anchor) == "" || strings.TrimSpace(c.Quote) == "" {
			return fmt.Errorf("invalid validity case %q", c.Number)
		}
		seen[n] = true
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
			if retrieval.NormalizeNumber(c.Number) != retrieval.NormalizeNumber(h.Number) {
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
