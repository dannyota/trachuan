package evaluation

import (
	"math"
	"unicode/utf8"

	"danny.vn/trachuan/internal/retrieval"
)

// CaseScore contains the results of one non-rejection case.
type CaseScore struct {
	Negative bool
	Ranks    []int
	Response retrieval.Response
}

// Metrics holds independently scored responses for one cohort, mode and limit.
type Metrics struct{ Cases []CaseScore }

func ratio(n, d int) *float64 {
	if d == 0 {
		return nil
	}
	v := float64(n) / float64(d)
	return &v
}

// RecallAt returns micro and macro recall at k for positive cases.
func (m Metrics) RecallAt(k int) (*float64, *float64) {
	matched, total, positive := 0, 0, 0
	macro := 0.0
	for _, c := range m.Cases {
		if c.Negative || len(c.Ranks) == 0 {
			continue
		}
		n := 0
		for _, r := range c.Ranks {
			if r > 0 && r <= k {
				n++
			}
		}
		matched += n
		total += len(c.Ranks)
		positive++
		macro += float64(n) / float64(len(c.Ranks))
	}
	if positive == 0 {
		return nil, nil
	}
	v := macro / float64(positive)
	return ratio(matched, total), &v
}

// MRRAt8 returns mean reciprocal first correct rank among positive cases.
func (m Metrics) MRRAt8() *float64 {
	sum := 0.0
	n := 0
	for _, c := range m.Cases {
		if c.Negative {
			continue
		}
		n++
		best := 0
		for _, r := range c.Ranks {
			if r > 0 && r <= 8 && (best == 0 || r < best) {
				best = r
			}
		}
		if best > 0 {
			sum += 1 / float64(best)
		}
	}
	if n == 0 {
		return nil
	}
	v := sum / float64(n)
	return &v
}

// CurrentLawPrecision measures stored-status consistency over the primary response group, any class.
// Legacy mode removes a trailing non-current run unless every hit is non-current.
func (m Metrics) CurrentLawPrecision(legacy bool) *float64 {
	current, total := 0, 0
	for _, c := range m.Cases {
		hits := c.Response.Hits
		if legacy {
			end := len(hits)
			for end > 0 && !isCurrent(hits[end-1]) {
				end--
			}
			if end > 0 {
				hits = hits[:end]
			}
		}
		for _, h := range hits {
			total++
			if isCurrent(h) {
				current++
			}
		}
	}
	return ratio(current, total)
}
func isCurrent(h retrieval.Hit) bool {
	return h.Stamp.Current && (h.Stamp.StatusClass == "in_force" || h.Stamp.StatusClass == "partial")
}

// PrecisionDetails counts primary validity and non-current group hits.
type PrecisionDetails struct{ PrimaryCurrent, PrimaryTotal, Unknown, Contradictory, NonCurrentCurrent, NonCurrentTotal int }

// PrecisionCounts returns validity counts for both response groups.
func (m Metrics) PrecisionCounts() PrecisionDetails {
	var d PrecisionDetails
	for _, c := range m.Cases {
		for _, h := range c.Response.Hits {
			d.PrimaryTotal++
			if isCurrent(h) {
				d.PrimaryCurrent++
			}
			if h.Stamp.StatusClass == "unknown" || h.Stamp.StatusClass == "" {
				d.Unknown++
			}
			expected := h.Stamp.StatusClass == "in_force" || h.Stamp.StatusClass == "partial"
			if h.Stamp.Current != expected {
				d.Contradictory++
			}
		}
		for _, h := range c.Response.NonCurrent {
			d.NonCurrentTotal++
			if isCurrent(h) {
				d.NonCurrentCurrent++
			}
		}
	}
	return d
}

// Confusion holds abstention counts and their derived rates.
type Confusion struct {
	TP                 int      `json:"tp"`
	FN                 int      `json:"fn"`
	FP                 int      `json:"fp"`
	TN                 int      `json:"tn"`
	Accuracy           *float64 `json:"accuracy"`
	NegativeRecall     *float64 `json:"negative_recall"`
	PositiveAnswerRate *float64 `json:"positive_answer_rate"`
	BalancedAccuracy   *float64 `json:"balanced_accuracy"`
}

// Abstention measures the abstention decision over all cases.
func (m Metrics) Abstention() Confusion {
	var a Confusion
	for _, c := range m.Cases {
		abstain := c.Response.Abstain || len(c.Response.Hits)+len(c.Response.NonCurrent) == 0
		switch {
		case c.Negative && abstain:
			a.TP++
		case c.Negative:
			a.FN++
		case abstain:
			a.FP++
		default:
			a.TN++
		}
	}
	a.Accuracy = ratio(a.TP+a.TN, a.TP+a.TN+a.FP+a.FN)
	a.NegativeRecall = ratio(a.TP, a.TP+a.FN)
	a.PositiveAnswerRate = ratio(a.TN, a.TN+a.FP)
	if a.NegativeRecall != nil && a.PositiveAnswerRate != nil {
		v := (*a.NegativeRecall + *a.PositiveAnswerRate) / 2
		a.BalancedAccuracy = &v
	}
	return a
}

// EvidenceCharacters returns the transmitted evidence character count per case.
func (m Metrics) EvidenceCharacters() []float64 {
	values := make([]float64, 0, len(m.Cases))
	for _, c := range m.Cases {
		n := 0
		for _, group := range [][]retrieval.Hit{c.Response.Hits, c.Response.NonCurrent} {
			for _, h := range group {
				n += utf8.RuneCountInString(h.Text)
				for _, r := range h.Relations {
					n += utf8.RuneCountInString(r.Evidence)
				}
			}
		}
		values = append(values, float64(n))
	}
	return values
}

// Quantile returns a nearest-rank quantile of sorted values, or nil for no values.
func Quantile(sorted []float64, p float64) *float64 {
	if len(sorted) == 0 || math.IsNaN(p) || p < 0 || p > 1 {
		return nil
	}
	i := int(math.Ceil(p*float64(len(sorted)))) - 1
	if i < 0 {
		i = 0
	}
	v := sorted[i]
	return &v
}
