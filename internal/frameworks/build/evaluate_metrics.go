package build

import (
	"slices"
)

func metric(n float64, d int, failed bool) Metric {
	m := Metric{Status: "unavailable", Numerator: n, Denominator: d, Noise: "Small cohorts: one case can move a metric by several percentage points; abstention is biased by calibration on the scored public cohort."}
	if d > 0 && !failed {
		v := n / float64(d)
		m.Value = &v
		m.Status = "available"
	}
	return m
}

func cohortMember(c CaseResult, cohort string) bool {
	return c.Adjudication == "accepted" && (cohort == "licensed" || c.Negative || c.Access == "free")
}

func callerFor(cohort string) string {
	if cohort == "public" {
		return "free-only"
	}
	return "full"
}

func modeMetrics(cases []CaseResult, caller, mode string, versions map[string]int) ModeMetrics {
	positives, negatives, hits, answers, abstentions, current, frameworkHits := 0, 0, 0, 0, 0, 0, 0
	reciprocal := 0.0
	nearHits := map[string]int{}
	positiveError, negativeError := false, false
	multiple := false
	for _, n := range versions {
		multiple = multiple || n > 1
	}
	for _, c := range cases {
		o := c.Results[caller][mode]
		for _, near := range o.NearHits {
			nearHits[near.Kind]++
		}
		if c.Negative {
			if mode == "filtered" {
				continue
			}
			negatives++
			negativeError = negativeError || o.Error
			if o.Abstention == "abstained" {
				abstentions++
			}
		} else {
			positives++
			positiveError = positiveError || o.Error
			if o.Rank > 0 && o.Rank <= 8 {
				hits++
				reciprocal += 1 / float64(o.Rank)
			}
			if !o.Error && o.Abstention != "abstained" {
				answers++
			}
			if versions[c.Code] > 1 {
				current += o.CurrentHits
				frameworkHits += o.FrameworkHits
			}
		}
	}
	p, n := metric(float64(answers), positives, positiveError), metric(float64(abstentions), negatives, negativeError)
	b := metric(0, 2, positiveError || negativeError || p.Value == nil || n.Value == nil)
	if p.Value != nil && n.Value != nil {
		b = metric(*p.Value+*n.Value, 2, positiveError || negativeError)
	}
	cv := metric(float64(current), frameworkHits, positiveError)
	if !multiple {
		cv.Status = "not_applicable"
	}
	if mode == "filtered" {
		n.Status, b.Status = "not_applicable", "not_applicable"
	}
	return ModeMetrics{NearHits: nearHits, Recall: metric(float64(hits), positives, positiveError), MRR: metric(reciprocal, positives, positiveError), CurrentVersion: cv, PositiveAnswerRate: p, NegativeAbstentionRate: n, BalancedAccuracy: b}
}

func ComputeMetrics(cases []CaseResult, cohort string, versions map[string]int) CohortMetrics {
	caller := callerFor(cohort)
	result := CohortMetrics{Caller: caller, Modes: map[string]ModeMetrics{}, Versions: map[string]map[string]ModeMetrics{}, Filter: FilterGate{Pass: true, Losses: []string{}}}
	var members []CaseResult
	perVersion := map[string][]CaseResult{}
	for _, c := range cases {
		if !cohortMember(c, cohort) {
			continue
		}
		members = append(members, c)
		if !c.Negative {
			key := "fw:" + c.Code + ":" + c.Version
			perVersion[key] = append(perVersion[key], c)
		}
		if c.Negative {
			continue
		}
		open, filtered := c.Results[caller]["open"], c.Results[caller]["filtered"]
		if open.Error || filtered.Error || (open.Rank > 0 && filtered.Rank == 0) {
			result.Filter.Pass = false
			result.Filter.Losses = append(result.Filter.Losses, c.ID)
		}
		if open.Rank == 0 && filtered.Rank > 0 {
			result.Filter.Gains++
		}
	}
	for _, mode := range []string{"open", "filtered"} {
		result.Modes[mode] = modeMetrics(members, caller, mode, versions)
		for version, cs := range perVersion {
			if result.Versions[version] == nil {
				result.Versions[version] = map[string]ModeMetrics{}
			}
			result.Versions[version][mode] = modeMetrics(cs, caller, mode, versions)
		}
	}
	slices.Sort(result.Filter.Losses)
	return result
}
