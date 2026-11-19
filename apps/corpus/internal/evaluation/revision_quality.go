package evaluation

import (
	"fmt"
	"reflect"

	"danny.vn/trachuan/internal/retrieval"
)

type comparableValue struct {
	value                  *float64
	numerator, denominator int
}

func comparableMetric(records map[mrrCaseKey]CaseResult, ids []string, mode retrieval.Mode, name string, limit int) comparableValue {
	matched, total, positive := 0, 0, 0
	mrr := 0.0
	for _, id := range ids {
		c, ok := records[mrrCaseKey{id, mode, "hybrid", limit}]
		if !ok || len(c.MappingIssues) > 0 {
			return comparableValue{}
		}
		if c.Expected == 0 {
			continue
		}
		positive++
		best := 0
		for _, rank := range c.Ranks {
			if rank > 0 && rank <= limit {
				matched++
			}
			if rank > 0 && rank <= 8 && (best == 0 || rank < best) {
				best = rank
			}
		}
		total += len(c.Ranks)
		if best > 0 {
			mrr += 1 / float64(best)
		}
	}
	out := comparableValue{numerator: matched, denominator: total}
	switch {
	case name == "mrr" && positive > 0:
		v := mrr / float64(positive)
		out.value = &v
	case name == "recall" && total > 0:
		v := float64(matched) / float64(total)
		out.value = &v
	}
	return out
}

func unresolvedRevision(r, previous *Report) error {
	if previous == nil || r.revision != nil || previous.FixtureHashes.Development == r.FixtureHashes.Development {
		return nil
	}
	if previous.Cohorts["development"].CaseCount > 0 || previous.Cohorts["development_period"].CaseCount > 0 {
		return fmt.Errorf("development fixture revision was not resolved against the previous report")
	}
	return nil
}

func applyRevision(r, p *Report, n *NonRegression) {
	rev := r.revision
	current, _ := cohortCases(r, "development")
	previous, _ := cohortCases(p, "development")
	candidate := comparableMetric(current, rev.comparable, n.Mode, n.Name, n.Limit)
	prior := comparableMetric(previous, rev.previousOrder, n.Mode, n.Name, n.Limit)
	n.PreviousValue = prior.value
	n.Revision = &Revision{Cases: len(rev.comparable), Value: candidate.value, Revised: append([]RevisedCase{}, rev.revised...)}
	if n.Name == "recall" {
		if candidate.value != nil {
			n.Numerator, n.Denominator = &candidate.numerator, &candidate.denominator
		}
		if prior.value != nil {
			n.PreviousNumerator, n.PreviousDenominator = &prior.numerator, &prior.denominator
		}
	} else {
		n.MRRComparison = &MRRComparison{Previous: prior.value, HighWater: prior.value, Losses: []MRRLoss{}}
	}
	switch {
	case !finite(n.Value) || !finite(candidate.value):
		n.Status = "unavailable"
		return
	case !finite(prior.value):
		n.Status = "no_previous"
		return
	}
	n.Status = "fail"
	if n.Name == "recall" {
		if candidate.denominator == prior.denominator && candidate.numerator >= prior.numerator {
			n.Status = "pass"
		}
		return
	}
	ids := map[string]bool{}
	for _, id := range rev.comparable {
		ids[id] = true
	}
	sameRecipes := sameMRRRecipes(r, p)
	if p.Version >= 2 && sameRecipes {
		v := *prior.value
		if old := previousMRRRecord(p, "development", n); old != nil {
			v += toleranceUsed(old)
		}
		n.HighWater = &v
	}
	losses, expectations := mrrLosses(r, p, "development", n.Mode, ids)
	n.Losses = losses
	if sameRecipes && comparableScorer(p.Scorer, r.Scorer) && sameRevisedCohort(r, p, ids) && expectations {
		n.Tolerance = mrrTolerance
	}
	n.WithinTolerance = *candidate.value < *prior.value
	threshold := *prior.value
	if n.Tolerance > 0 {
		threshold = *n.HighWater - n.Tolerance
	}
	if *candidate.value >= threshold {
		n.Status = "pass"
	}
}

func sameRevisedCohort(a, b *Report, ids map[string]bool) bool {
	x, ok := cohortCases(a, "development")
	if !ok {
		return false
	}
	y, ok := cohortCases(b, "development")
	if !ok {
		return false
	}
	count := 0
	for key, c := range x {
		if !ids[key.ID] {
			continue
		}
		count++
		old, exists := y[key]
		if !exists || c.Expected != old.Expected || len(c.Ranks) != len(old.Ranks) {
			return false
		}
	}
	for key := range y {
		if ids[key.ID] {
			count--
		}
	}
	if count != 0 {
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
