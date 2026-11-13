package evaluation

import "danny.vn/trachuan/internal/retrieval"

func productCaseDifferences(cases []CaseDifference) []CaseDifference {
	filtered := cases[:0]
	for _, c := range cases {
		if c.Cohort != "legacy" {
			filtered = append(filtered, c)
		}
	}
	return filtered
}

// CaseDifference lists changed citation matches and abstention decisions.
type CaseDifference struct {
	ValidityOverrides
	CapOverride        *retrieval.CapOverride        `json:"cap_override"`
	ArticleCapOverride *retrieval.ArticleCapOverride `json:"article_cap_override,omitempty"`
	CaseID             string                        `json:"case_id"`
	Cohort             string                        `json:"cohort"`
	Mode               retrieval.Mode                `json:"mode"`
	Arms               retrieval.ArmMode             `json:"arms"`
	Limit              int                           `json:"limit"`
	Gained             []int                         `json:"gained"`
	Lost               []int                         `json:"lost"`
	PreviousAbstain    bool                          `json:"previous_abstain"`
	Abstain            bool                          `json:"abstain"`
}

func compareCases(previous, current []CaseResult) []CaseDifference {
	type key struct {
		id, cohort string
		mode       retrieval.Mode
		arms       retrieval.ArmMode
		limit      int
	}
	old := map[key]CaseResult{}
	for _, c := range previous {
		old[key{c.CaseID, c.Cohort, c.Mode, c.Arms, c.Limit}] = c
	}
	changes := []CaseDifference{}
	for _, c := range current {
		p, ok := old[key{c.CaseID, c.Cohort, c.Mode, c.Arms, c.Limit}]
		if !ok {
			continue
		}
		d := CaseDifference{CapOverride: c.CapOverride, ArticleCapOverride: c.ArticleCapOverride, ValidityOverrides: c.ValidityOverrides, CaseID: c.CaseID, Cohort: c.Cohort, Mode: c.Mode, Arms: c.Arms, Limit: c.Limit, PreviousAbstain: p.Abstain, Abstain: c.Abstain}
		for i := 0; i < max(len(p.Ranks), len(c.Ranks)); i++ {
			before := i < len(p.Ranks) && p.Ranks[i] > 0
			after := i < len(c.Ranks) && c.Ranks[i] > 0
			if after && !before {
				d.Gained = append(d.Gained, i)
			}
			if before && !after {
				d.Lost = append(d.Lost, i)
			}
		}
		if len(d.Gained) > 0 || len(d.Lost) > 0 || d.PreviousAbstain != d.Abstain {
			changes = append(changes, d)
		}
	}
	return changes
}
