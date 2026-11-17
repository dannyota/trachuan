package abstention

import "sort"

type Candidate struct {
	Threshold float64 `json:"threshold"`
	TP        int     `json:"tp"`
	FP        int     `json:"fp"`
	TN        int     `json:"tn"`
	FN        int     `json:"fn"`
}

type Score struct {
	Value       float64
	Negative    bool
	Empty       bool
	FixedAnswer bool
}

func Threshold(scores []Score) (float64, []Candidate) {
	values := []float64{}
	for _, s := range scores {
		if !s.Empty && !s.FixedAnswer {
			values = append(values, s.Value)
		}
	}
	sort.Float64s(values)
	thresholds := []float64{-1}
	for i := 1; i < len(values); i++ {
		a, b := values[i-1], values[i]
		if a == b {
			continue
		}
		m := a + (b-a)/2
		if m <= a {
			m = b
		}
		thresholds = append(thresholds, m)
	}
	thresholds = append(thresholds, 2)
	best, bestScore := -1.0, -1
	candidates := make([]Candidate, 0, len(thresholds))
	for _, threshold := range thresholds {
		c := Candidate{Threshold: threshold}
		for _, s := range scores {
			abstain := !s.FixedAnswer && (s.Empty || s.Value < threshold)
			switch {
			case s.Negative && abstain:
				c.TP++
			case s.Negative:
				c.FN++
			case abstain:
				c.FP++
			default:
				c.TN++
			}
		}
		candidates = append(candidates, c)
		if c.TN+c.FP > 0 && c.TP+c.FN > 0 {
			score := c.TN*(c.TP+c.FN) + c.TP*(c.TN+c.FP)
			if score > bestScore {
				best, bestScore = threshold, score
			}
		}
	}
	return best, candidates
}
