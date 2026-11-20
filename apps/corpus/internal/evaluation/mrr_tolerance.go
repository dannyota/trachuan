package evaluation

import (
	"fmt"
	"math"
	"reflect"
	"sort"

	"danny.vn/trachuan/internal/retrieval"
)

const mrrTolerance = .005

type MRRLoss struct {
	CaseID       string `json:"case_id"`
	PreviousRank int    `json:"previous_rank"`
	Rank         int    `json:"rank"`
}

type MRRComparison struct {
	Previous        *float64  `json:"previous"`
	HighWater       *float64  `json:"high_water"`
	Tolerance       float64   `json:"tolerance"`
	WithinTolerance bool      `json:"within_tolerance"`
	Losses          []MRRLoss `json:"losses"`
}

func comparableScorer(previous, current string) bool {
	return previous == current || (previous == "agent-eval/5" && current == "agent-eval/6") || (previous == "agent-eval/6" && current == "agent-eval/5")
}

func finite(v *float64) bool { return v != nil && !math.IsNaN(*v) && !math.IsInf(*v, 0) }

func applyMRRComparison(r, p *Report, cohort string, n *NonRegression) {
	n.MRRComparison = &MRRComparison{Previous: n.PreviousValue, HighWater: n.PreviousValue, Losses: []MRRLoss{}}
	if p == nil || !finite(n.PreviousValue) || !finite(n.Value) {
		return
	}
	sameRecipes := sameMRRRecipes(r, p)
	if p.Version >= 2 && sameRecipes {
		if old := previousMRRRecord(p, cohort, n); old != nil {
			v := math.Max(*n.PreviousValue, *old.HighWater)
			if old.Revision != nil {
				v = *n.PreviousValue + toleranceUsed(old)
			}
			n.HighWater = &v
		}
	}
	losses, expectations := mrrLosses(r, p, cohort, n.Mode, nil)
	n.Losses = losses
	if sameRecipes && comparableScorer(p.Scorer, r.Scorer) && sameMRRFixtures(r, p, cohort) && sameMRRCohort(r, p, cohort) && expectations {
		n.Tolerance = mrrTolerance
	}
	n.WithinTolerance = *n.Value < *n.PreviousValue
	n.Status = "fail"
	threshold := *n.PreviousValue
	if n.Tolerance > 0 {
		threshold = *n.HighWater - n.Tolerance
	}
	if *n.Value >= threshold {
		n.Status = "pass"
	}
}

func previousMRRRecord(p *Report, cohort string, n *NonRegression) *NonRegression {
	for i, old := range p.Cohorts[cohort].NonRegression {
		if old.Name == n.Name && old.Mode == n.Mode && old.Arms == n.Arms && old.Limit == n.Limit && old.MRRComparison != nil && finite(old.HighWater) {
			return &p.Cohorts[cohort].NonRegression[i]
		}
	}
	return nil
}

func toleranceUsed(old *NonRegression) float64 {
	judged := old.Value
	if old.Revision != nil {
		judged = old.Revision.Value
	}
	if !finite(judged) || *old.HighWater <= *judged {
		return 0
	}
	return *old.HighWater - *judged
}

func sameMRRFixtures(a, b *Report, cohort string) bool {
	if cohort == "development" {
		return a.FixtureHashes.Development != "" && a.FixtureHashes.Development == b.FixtureHashes.Development
	}
	return a.FixtureHashes.Legacy != "" && a.FixtureHashes.OldProduct != "" && a.FixtureHashes.Legacy == b.FixtureHashes.Legacy && a.FixtureHashes.OldProduct == b.FixtureHashes.OldProduct
}

type mrrCaseKey struct {
	ID    string
	Mode  retrieval.Mode
	Arms  retrieval.ArmMode
	Limit int
}

func cohortCases(r *Report, cohort string) (map[mrrCaseKey]CaseResult, bool) {
	out := map[mrrCaseKey]CaseResult{}
	for _, c := range r.Cases {
		if c.Cohort != cohort {
			continue
		}
		key := mrrCaseKey{c.CaseID, c.Mode, c.Arms, c.Limit}
		if _, ok := out[key]; ok {
			return nil, false
		}
		out[key] = c
	}
	return out, len(out) > 0
}

func sameMRRCohort(a, b *Report, cohort string) bool {
	ac, bc := a.Cohorts[cohort], b.Cohorts[cohort]
	if ac.CaseCount != bc.CaseCount || ac.PositiveCount != bc.PositiveCount || ac.NegativeCount != bc.NegativeCount || ac.RejectionCount != bc.RejectionCount || ac.ExpectedCitations != bc.ExpectedCitations {
		return false
	}
	x, ok := cohortCases(a, cohort)
	if !ok {
		return false
	}
	y, ok := cohortCases(b, cohort)
	if !ok || len(x) != len(y) {
		return false
	}
	for key, c := range x {
		old, exists := y[key]
		if !exists || c.Expected != old.Expected || len(c.Ranks) != len(old.Ranks) {
			return false
		}
	}
	type metricIdentity struct {
		Mode  retrieval.Mode
		Arms  retrieval.ArmMode
		Limit int
		Name  string
	}
	denoms := func(metrics []Metric) map[metricIdentity]int {
		out := map[metricIdentity]int{}
		for _, m := range metrics {
			out[metricIdentity{m.Mode, m.Arms, m.Limit, m.Name}] = m.Denominator
		}
		return out
	}
	if !reflect.DeepEqual(denoms(ac.Metrics), denoms(bc.Metrics)) {
		return false
	}
	rejections := func(r *Report) map[string]bool {
		out := map[string]bool{}
		for _, v := range r.Rejections {
			out[v.CaseID] = true
		}
		return out
	}
	return reflect.DeepEqual(rejections(a), rejections(b))
}

func firstRank(ranks []int) int {
	best := 0
	for _, rank := range ranks {
		if rank > 0 && (best == 0 || rank < best) {
			best = rank
		}
	}
	return best
}

func reciprocal8(rank int) float64 {
	if rank < 1 || rank > 8 {
		return 0
	}
	return 1 / float64(rank)
}

func mrrLosses(r, p *Report, cohort string, mode retrieval.Mode, ids map[string]bool) ([]MRRLoss, bool) {
	losses := []MRRLoss{}
	old, _ := cohortCases(p, cohort)
	current, _ := cohortCases(r, cohort)
	preserved := true
	for key, c := range current {
		if key.Mode != mode || key.Arms != "hybrid" || key.Limit != 8 || (ids != nil && !ids[key.ID]) {
			continue
		}
		before, ok := old[key]
		if !ok {
			continue
		}
		previousRank, rank := firstRank(before.Ranks), firstRank(c.Ranks)
		if reciprocal8(rank) >= reciprocal8(previousRank) {
			continue
		}
		if !preservesAlternatives(before.Mapped, c.Mapped) {
			preserved = false
		}
		if rank == 0 {
			key.Limit = 20
			if later := firstRank(current[key].Ranks); later > 8 {
				rank = later
			}
		}
		losses = append(losses, MRRLoss{c.CaseID, previousRank, rank})
	}
	sort.Slice(losses, func(i, j int) bool { return losses[i].CaseID < losses[j].CaseID })
	return losses, preserved
}

func scoringAlternative(a Alternative) Alternative {
	a.DocumentNumber, a.SourceNumber, a.TargetNumber = "", "", ""
	if a.Kind == "document" {
		a.Anchor = nil
	}
	if a.Kind != "relation" {
		a.SourceKey, a.TargetKey, a.RelationKind, a.EvidenceSHA256 = "", "", "", ""
	}
	return a
}

func preservesAlternatives(previous, current [][]Alternative) bool {
	if len(previous) == 0 {
		return false
	}
	for _, group := range previous {
		if len(group) == 0 {
			return false
		}
		for _, a := range group {
			found := false
			for _, next := range current {
				for _, b := range next {
					if sameAlternative(scoringAlternative(a), scoringAlternative(b)) {
						found = true
					}
				}
			}
			if !found {
				return false
			}
		}
	}
	return true
}

func validatePreviousMRR(value any) error {
	switch fields := value.(type) {
	case map[string]any:
		if fields["name"] == "mrr" && fields["gating"] == true {
			for _, key := range []string{"previous", "high_water", "tolerance", "within_tolerance", "losses"} {
				if _, ok := fields[key]; !ok {
					return fmt.Errorf("previous MRR comparison missing %s", key)
				}
			}
			for _, key := range []string{"previous", "high_water", "tolerance"} {
				if fields[key] == nil && fields["status"] != "pass" && key != "tolerance" {
					continue
				}
				if _, ok := fields[key].(float64); !ok {
					return fmt.Errorf("invalid previous MRR %s", key)
				}
			}
			if _, ok := fields["within_tolerance"].(bool); !ok {
				return fmt.Errorf("invalid previous MRR within_tolerance")
			}
			if _, ok := fields["losses"].([]any); !ok {
				return fmt.Errorf("invalid previous MRR losses")
			}
		}
		if revision, present := fields["revision"]; present {
			if err := validatePreviousRevision(revision); err != nil {
				return err
			}
		}
		for _, child := range fields {
			if err := validatePreviousMRR(child); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range fields {
			if err := validatePreviousMRR(child); err != nil {
				return err
			}
		}
	}
	return nil
}

func validatePreviousRevision(value any) error {
	fields, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("invalid previous revision")
	}
	if _, ok = fields["cases"].(float64); !ok {
		return fmt.Errorf("invalid previous revision cases")
	}
	if v, present := fields["value"]; !present {
		return fmt.Errorf("previous revision missing value")
	} else if _, isNumber := v.(float64); v != nil && !isNumber {
		return fmt.Errorf("invalid previous revision value")
	}
	revised, ok := fields["revised"].([]any)
	if !ok {
		return fmt.Errorf("invalid previous revision revised")
	}
	for _, entry := range revised {
		item, isObject := entry.(map[string]any)
		if !isObject {
			return fmt.Errorf("invalid previous revised case")
		}
		id, _ := item["case_id"].(string)
		reason, _ := item["reason"].(string)
		if id == "" || (reason != "added" && reason != "changed" && reason != "cohort") {
			return fmt.Errorf("invalid previous revised case")
		}
	}
	return nil
}
